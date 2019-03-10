package gcal

import (
	"context"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/calendar/v3"
	"time"
)

func NewCalendarWebHookManaged(calendarService *calendar.Service, url string) *calendarWebHookManaged {
	w := &calendarWebHookManaged{
		calendarService: calendarService,
		url: url,
	}

	return w
}

type calendarWebHookManaged struct {
	url             string
	Webhook         *calendar.Channel
	calendarService *calendar.Service
	cancelRestart   context.CancelFunc
}

func (c *calendarWebHookManaged) Start() error {
	channel := &calendar.Channel{
		Id:      uuid.New().String(),
		Address: c.url,
		Type:    "web_hook",
	}

	webhook, err := c.calendarService.Events.Watch("primary", channel).Do()
	if err != nil {
		return err
	}
	c.Webhook = webhook
	log.
		WithField("expires", time.Unix(c.Webhook.Expiration/1000, 0)).
		WithField("url", c.url).
		Infof("Started channel %v \n", c.Webhook.Id)

	c.cancelRestart = c.restartAt(parseUnixTimeInSeconds(c.Webhook.Expiration))
	return nil
}

func (c calendarWebHookManaged) Stop() error {
	if c.cancelRestart != nil {
		c.cancelRestart()
	}
	err := c.calendarService.Channels.Stop(c.Webhook).Do()
	if err != nil {
		log.Errorf("Could not stop channel %v", c.Webhook.Id)
		return err
	} else {
		log.Infof("Stopped channel %v \n", c.Webhook.Id)
	}
	c.Webhook = nil
	return nil
}

func (c *calendarWebHookManaged) restart() error {
	err := c.Stop()
	if err != nil {
		return err
	}
	return c.Start()
}

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
