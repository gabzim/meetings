package gcal

import (
	"context"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/calendar/v3"
	"time"
)

type EventAlarm struct {
	event       *calendar.Event
	cancelAlarm context.CancelFunc
}

// NotifyEventStarting receives a channel of events (new/updated/deleted) and returns a channel that fires every time an event is about to start
func NotifyEventStarting(parentCtx context.Context, events <-chan *calendar.Event) <-chan *calendar.Event {
	// maps eventIds to a cancelFn we can call to cancel their alarms if the events are deleted or changed.
	alarms := make(map[string]EventAlarm)
	eventStarting := make(chan *calendar.Event)
	go func() {
		for updatedEvent := range events {
			event := updatedEvent

			//if we have seen this event before, cancel the previous alarm and we'll recreate it
			eventAlarm, ok := alarms[event.Id]
			logMsg := "Event received, setting alarm"
			if ok {
				//cancel alarm if event has been cancelled or rescheduled
				if event.Status == "cancelled" {
					eventAlarm.cancelAlarm()
					log.WithField("name", eventAlarm.event.Summary).Info("Event cancelled, alarm removed")
					continue
				} else if event.Start.DateTime == eventAlarm.event.Start.DateTime {
					//evente hasn't changed, skip processing it
					continue
				} else if event.Start.DateTime != eventAlarm.event.Start.DateTime {
					// event has changed, cancel alarm and set a new one below
					logMsg = "Event updated, resetting alarm"
					eventAlarm.cancelAlarm()
				}
			}

			now := time.Now()
			eventStartTime, _ := time.Parse(time.RFC3339, event.Start.DateTime)
			timeUntilAlarm := eventStartTime.Sub(now) - 30*time.Second // trigger alarm 30 seconds before event starts
			if timeUntilAlarm < 0 {
				log.WithFields(log.Fields{"at": eventStartTime.Format(time.RFC1123), "in": "   N/A   ", "name": event.Summary}).Info("Event is already in progress, skipping alarm")
				continue
			}

			log.WithFields(log.Fields{"at": eventStartTime.Format(time.RFC1123), "in": timeUntilAlarm, "name": event.Summary}).
				Info(logMsg)

			ctx, cancelAlarm := context.WithCancel(parentCtx)
			alarms[event.Id] = EventAlarm{event: event, cancelAlarm: cancelAlarm}

			//a goroutine will fire when the event is about to start, unless this is cancelled
			go func() {
				timer := time.NewTimer(timeUntilAlarm)
				select {
				case <-timer.C:
					log.WithField("event", event.Summary).Info("Event is Starting")
					eventStarting <- event
				case <-ctx.Done():
					// alarm cancelled
				}
			}()
		}
	}()
	return eventStarting
}