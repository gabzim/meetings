package calendarwh

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/api/calendar/v3"
	"net/http"
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

type pushNotificationHandler func(*CalendarPushNotification) error

var syncEvery30Minutes = 30 * time.Minute

func New(calendarService *calendar.Service, calendarName, endpoint string, log *zap.SugaredLogger) *CalendarWebHookManaged {
	w := &CalendarWebHookManaged{
		calendarSrv:  calendarService,
		endpoint:     endpoint,
		calendarName: calendarName,
		log:          log,
	}
	w.Handler = createPushHttpHandler(w.handleIncomingPushNotification, log)
	return w
}

// CalendarWebHookManaged instructs google to start pushing event updates to your webhook (called a Channel in the docs)
// and manages the lifecycle of this Channel (google will only post notifications to your endpoint up until some expiration date, and you need to recreate the Channel after it's expired).
// This service will handle that recreation for you automatically. Also, if any other channels are open on this endpoint, it will close them.
// When you .ReadPump() this service, you get a *calendar.Events chan that you can range over. Any new/updated/deleted events will be sent in there.
type CalendarWebHookManaged struct {
	// Handler is the http handler you must mount in your webserver that will handle the POSTs Google Calendar will do to your endpoint
	Handler http.HandlerFunc
	//events provides a channel that pushes a *calendar.Events when they are created/updated/deleted in your calendar. As your schedule changes
	//the updated events will be pushed in this channel. Sometimes, events will be pushed even though they haven't been updated. This happens so that you can sync/refresh your data
	//in case it's become stale.
	//for this reason, it is your responsibility to keep track of the events you have seen and the ones you haven't (and whether they have changed).
	//Consumers can't access this directly, they get a reference to it when they Start() the hook.
	events chan *calendar.Event
	//gcalChannel is the google calendar response when you ask to create the Webhook Channel, it is kept so that it can be closed when you stop the web hook.
	gcalChannel *calendar.Channel
	calendarSrv *calendar.Service
	//Your channel is only valid for a period of time. Before it expires, this ManagedCalendarHook
	cancelRestart context.CancelFunc
	ticker        *time.Ticker
	syncToken     string
	endpoint      string
	calendarName  string
	log           *zap.SugaredLogger
}

func (c *CalendarWebHookManaged) Start() (<-chan *calendar.Event, error) {
	err := c.startCalendarChannel(c.endpoint)
	if err != nil {
		return nil, err
	}
	c.events = make(chan *calendar.Event, 100)
	c.startEventSyncTicker(syncEvery30Minutes)
	return c.events, nil
}

func (c *CalendarWebHookManaged) Stop() error {
	c.stopEventSyncTicker()
	err := c.stopCalendarChannel()
	if err != nil {
		// we couldn't stop the channel, start sync ticker again so we do not leave the stop halfway through.
		c.startEventSyncTicker(syncEvery30Minutes)
		return err
	}
	close(c.events)
	c.events = nil
	return nil
}

func (c *CalendarWebHookManaged) IsRunning() bool {
	return c.events != nil
}

func (c *CalendarWebHookManaged) startCalendarChannel(endpoint string) error {
	channel := &calendar.Channel{
		Id:      uuid.New().String(),
		Address: endpoint,
		Type:    "web_hook",
	}
	webhook, err := c.calendarSrv.Events.Watch(c.calendarName, channel).Do()
	if err != nil {
		return err
	}
	c.gcalChannel = webhook
	c.log.
		Infow("Started channel "+c.gcalChannel.Id, "expires", time.Unix(c.gcalChannel.Expiration/1000, 0), "url", endpoint, "channel", c.gcalChannel.Id)

	c.cancelRestart = c.restartAt(parseUnixTimeInSeconds(c.gcalChannel.Expiration))
	return nil
}

func (c *CalendarWebHookManaged) startEventSyncTicker(d time.Duration) {
	ticker := time.NewTicker(d)
	c.ticker = ticker
	go func() {
		for range ticker.C {
			if c.gcalChannel == nil {
				c.stopEventSyncTicker()
				return
			}
			exp := parseUnixTimeInSeconds(c.gcalChannel.Expiration)
			pushNotification := &CalendarPushNotification{
				ChannelId:         c.gcalChannel.Id,
				ResourceId:        c.gcalChannel.ResourceId,
				ChannelExpiration: &exp,
				ResourceState:     "sync",
				ResourceUri:       "",
				MessageNumber:     "1",
				Token:             c.gcalChannel.Token,
			}
			c.handleIncomingPushNotification(pushNotification)
		}
	}()
}

func (c *CalendarWebHookManaged) stopEventSyncTicker() {
	if c.ticker == nil {
		return
	}
	c.ticker.Stop()
	c.ticker = nil
}

func (c *CalendarWebHookManaged) stopCalendarChannel() error {
	if c.cancelRestart != nil {
		c.cancelRestart()
	}

	if c.gcalChannel == nil {
		return fmt.Errorf("webhook already closed")
	}

	err := c.calendarSrv.Channels.Stop(c.gcalChannel).Do()
	if err != nil {
		c.log.Errorf("Could not stop channel %v", c.gcalChannel.Id)
		return err
	} else {
		c.log.Infof("Stopped channel %v \n", c.gcalChannel.Id)
	}
	c.gcalChannel = nil
	c.cancelRestart = nil
	return nil
}

func (c *CalendarWebHookManaged) handleIncomingPushNotification(pushNotification *CalendarPushNotification) error {
	if c.events == nil {
		return fmt.Errorf("the google calendar Channel is stopped but we're still receiving notifications")
	}
	// if we receive a push from a channel we haven't created, close it.
	if pushNotification.ChannelId != c.gcalChannel.Id {
		err := c.calendarSrv.Channels.Stop(&calendar.Channel{Id: pushNotification.ChannelId, ResourceId: pushNotification.ResourceId}).Do()
		if err != nil {
			c.log.Errorw("could not close lingering hook", "channel", pushNotification.ChannelId)
		} else {
			c.log.Errorw("closed lingering hook", "channel", pushNotification.ChannelId)
		}
		return nil
	}
	// if it's a sync push notification, forget our sync token and start fresh
	if pushNotification.ResourceState == "sync" {
		c.syncToken = ""
	}
	// passing in syncToken means that the query will only retrieve deltas since the last query
	// if it's empty we retrieve all events for the coming week
	twoWeeks := time.Now().Add(14 * 24 * time.Hour)
	events, nextSyncToken, err := c.fetchEventsDelta(twoWeeks, c.syncToken)

	if err != nil {
		c.log.Errorf("unable to retrieve events delta")
	}
	c.syncToken = nextSyncToken
	for _, event := range events {
		c.events <- event
	}
	return nil
}

// fetchEventsDelta retrieves events from the calendar, if syncToken is passed, only deltas from the last query will be retrieved
// it then sets the value of syncToken to what the google calendar returned
func (c *CalendarWebHookManaged) fetchEventsDelta(to time.Time, syncToken string) ([]*calendar.Event, string, error) {
	eventQuery := c.calendarSrv.Events.
		List(c.calendarName).
		MaxResults(2500).
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

	return events.Items, events.NextSyncToken, nil
}

func (c *CalendarWebHookManaged) restart() error {
	err := c.stopCalendarChannel()
	if err != nil {
		return err
	}
	return c.startCalendarChannel(c.endpoint)
}

// restartAt launches a goroutine that restarts the calendar.Channel at the given time. It returns a function that you can call if you wish to cancel this Restart.
func (c *CalendarWebHookManaged) restartAt(t time.Time) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-time.After(time.Until(t)):
			c.restart()
		case <-ctx.Done():

		}
	}()

	return cancel
}

// utility functions

func parseUnixTimeInSeconds(secs int64) time.Time {
	return time.Unix(secs/1000, 0)
}

func createPushHttpHandler(callback pushNotificationHandler, log *zap.SugaredLogger) http.HandlerFunc {
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

		err := callback(pushNotification)
		if err != nil {
			log.Error(err)
		}
	}
}
