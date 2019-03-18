package gcal

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/calendar/v3"
	"net/http"
	"sort"
	"time"
)

type CalendarPushNotification struct {
	ChannelExpiration *time.Time
	ChannelId         string
	MessageNumber     string
	ResourceId        string
	ResourceState     string
	ResourceUri       string
	Token             string
}

func NewManagedCalendarWebHook(calendarService *calendar.Service, endpoint string) (*calendarWebHookManaged, http.HandlerFunc) {
	w := &calendarWebHookManaged{
		endpoint:          endpoint,
		calendarSrv:       calendarService,
		pushNotifications: make(chan *CalendarPushNotification, 1),
	}
	handler := webhookHandler(w.pushNotifications)
	return w, handler
}

// calendarWebHookManaged instructs google to start pushing event updates via a webhook (called a Channel in the docs)
// and manages the lifecycle of this Channel (google will only post notifications to your endpoint up until some expiration date, and you need to recreate the Channel after it's expired).
// This service will handle that recreation for you automatically. Also, if any other channels are open on this endpoint, it will close them.
// After you .Start() this service, you can range over the .Events property. Any new/updated/deleted events will be sent in that channel.
type calendarWebHookManaged struct {
	//Events provides a channel that pushes *calendar.Events created/updated/deleted in your calendar. As your schedule changes
	//the events will be pushed in this channel. Sometimes, events will be pushed even though they haven't been updated. This happens whenever there's a need to "sync"
	//so that if you have a stale cache you can refresh it,
	//for this reason, it is your responsibility to keep track of the events you have seen and the ones you haven't (and whether they have changed)
	Events            chan *calendar.Event
	webhook           *calendar.Channel
	endpoint          string
	calendarSrv       *calendar.Service
	cancelRestart     context.CancelFunc
	pushNotifications chan *CalendarPushNotification
	syncToken         string
	ticker            *time.Ticker
}

func (c *calendarWebHookManaged) Start() error {
	err := c.startCalendarChannel()
	if err != nil {
		return err
	}
	c.startEventChannel()
	c.startNotificationTicker(time.Hour)
	return nil
}

func (c *calendarWebHookManaged) Stop() error {
	c.stopNotificationTicker()
	err := c.stopCalendarChannel()
	if err != nil {
		return err
	}
	close(c.pushNotifications)
	return nil
}

func (c *calendarWebHookManaged) startNotificationTicker(d time.Duration) {
	if c.pushNotifications == nil {
		return
	}
	ticker := time.NewTicker(d)
	c.ticker = ticker
	go func() {
		for range ticker.C {
			exp := parseUnixTimeInSeconds(c.webhook.Expiration)
			c.pushNotifications <- &CalendarPushNotification{
				ChannelId:         c.webhook.Id,
				ResourceId:        c.webhook.ResourceId,
				ChannelExpiration: &exp,
				ResourceState:     "sync",
				ResourceUri:       "",
				MessageNumber:     "1",
				Token:             c.webhook.Token,
			}
		}
	}()
}

func (c *calendarWebHookManaged) stopNotificationTicker() {
	if c.ticker == nil {
		return
	}
	c.ticker.Stop()
}

func (c *calendarWebHookManaged) startCalendarChannel() error {
	channel := &calendar.Channel{
		Id:      uuid.New().String(),
		Address: c.endpoint,
		Type:    "web_hook",
	}
	webhook, err := c.calendarSrv.Events.Watch("primary", channel).Do()
	if err != nil {
		return err
	}
	c.webhook = webhook
	log.
		WithField("expires", time.Unix(c.webhook.Expiration/1000, 0)).
		WithField("url", c.endpoint).
		Infof("Started channel %v \n", c.webhook.Id)

	c.cancelRestart = c.restartAt(parseUnixTimeInSeconds(c.webhook.Expiration))
	return nil
}

func (c *calendarWebHookManaged) stopCalendarChannel() error {
	if c.cancelRestart != nil {
		c.cancelRestart()
	}

	if c.webhook == nil {
		return fmt.Errorf("webhook already closed")
	}

	err := c.calendarSrv.Channels.Stop(c.webhook).Do()
	if err != nil {
		log.Errorf("Could not stop channel %v", c.webhook.Id)
		return err
	} else {
		log.Infof("Stopped channel %v \n", c.webhook.Id)
	}
	c.webhook = nil
	c.cancelRestart = nil
	return nil
}

func (c *calendarWebHookManaged) startEventChannel() {
	c.Events = make(chan *calendar.Event)
	go func() {
		for pushNotification := range c.pushNotifications {
			if pushNotification.ResourceState == "sync" {
				c.syncToken = ""
			}
			// if we receive a push from a channel we haven't created, close it.
			if pushNotification.ChannelId != c.webhook.Id {
				err := c.calendarSrv.Channels.Stop(&calendar.Channel{Id: pushNotification.ChannelId, ResourceId: pushNotification.ResourceId}).Do()
				if err != nil {
					log.WithField("channel", pushNotification.ChannelId).Error("could not close lingering hook")
				} else {
					log.WithField("channel", pushNotification.ChannelId).Warn("closed lingering hook")
				}
				continue
			}
			// passing in syncToken means that the query will only retrieve deltas since the last query
			// if it's empty so we retrieve all events for the coming week
			nextWeek := time.Now().Add(7 * 24 * time.Hour)
			events, nextSyncToken, err := c.fetchEventsDelta(nextWeek, c.syncToken)
			if err != nil {
				log.Errorf("unable to retrieve events delta")
			}
			c.syncToken = nextSyncToken
			for _, event := range events {
				c.Events <- event
			}
		}
		close(c.Events)
	}()
}

// fetchEventsDelta retrieves events from the calendar, if syncToken is passed, only deltas from the last query will be retrieved
// it then sets the value of syncToken to what the google calendar returned
func (c *calendarWebHookManaged) fetchEventsDelta(to time.Time, syncToken string) ([]*calendar.Event, string, error) {
	eventQuery := c.calendarSrv.Events.
		List("primary").
		MaxResults(20).
		SingleEvents(true)

	if syncToken != "" {
		eventQuery.SyncToken(syncToken)
	} else {
		eventQuery.TimeMin(time.Now().Format(time.RFC3339)).TimeMax(to.Format(time.RFC3339))
	}

	events, err := eventQuery.Do()

	if err != nil {
		return nil, "", err
	}

	sort.Slice(events.Items, func(i, j int) bool {
		if events.Items[i].Start == nil || events.Items[j].Start == nil {
			return false
		}
		return events.Items[i].Start.DateTime < events.Items[j].Start.DateTime
	})
	return events.Items, events.NextSyncToken, nil
}

func (c *calendarWebHookManaged) restart() error {
	err := c.stopCalendarChannel()
	if err != nil {
		return err
	}
	return c.startCalendarChannel()
}

//restartAt launches a goroutine that restarts the calendar.Channel at the given time. It returns a function that you can call if you wish to cancel this Restart.
func (c *calendarWebHookManaged) restartAt(t time.Time) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-time.After(t.Sub(time.Now())):
			c.restart()
		case <-ctx.Done():

		}
	}()

	return cancel
}

func parseUnixTimeInSeconds(secs int64) time.Time {
	return time.Unix(secs/1000, 0)
}

func webhookHandler(notifications chan<- *CalendarPushNotification) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		channelExpiration := r.Header.Get("X-Goog-webhook-Expiration")
		var channelExpirationTime *time.Time
		if channelExpiration != "" {
			t, err := time.Parse(time.RFC1123, channelExpiration)
			if err != nil {
				log.Errorf("Could not parse channel expiration time %v", err)
			}
			channelExpirationTime = &t
		}
		pushNotification := &CalendarPushNotification{
			ResourceId:        r.Header.Get("X-Goog-Resource-ID"),
			ChannelExpiration: channelExpirationTime,
			ChannelId:         r.Header.Get("X-Goog-Channel-ID"),
			MessageNumber:     r.Header.Get("X-Goog-Message-Number"),
			ResourceState:     r.Header.Get("X-Goog-Resource-State"),
			ResourceUri:       r.Header.Get("X-Goog-Resource-URI"),
			Token:             r.Header.Get("X-Goog-Channel-Token"),
		}

		w.Write([]byte("OK"))

		notifications <- pushNotification
	}
}
