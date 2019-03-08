package gcal

import (
	"context"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/calendar/v3"
	"sync"
	"time"
)

// NotifyEventStarting receives a channel of events (new/updated/deleted) and returns a channel that fires every time an event is about to start
func NotifyEventStarting(parentCtx context.Context, events <-chan *calendar.Event) <-chan *calendar.Event {
	eventStarting := make(chan *calendar.Event)
	// maps eventIds to a cancelFn we can call to cancel their alarms if the events are deleted or changed.
	alarms := make(map[string]context.CancelFunc)
	lock := sync.Mutex{}

	go func() {
		for updatedEvent := range events {
			event := updatedEvent

			//if we have seen this event before, cancel the previous alarm and we'll recreate it
			lock.Lock()
			cancelPreviousAlarm, ok := alarms[event.Id]
			lock.Unlock()
			if ok {
				cancelPreviousAlarm()
				log.WithField("event", event.Summary).Info("Removed alarm for event")
			}
			// if the event is cancelled, do not set an alarm for it, it was handled above
			if event.Status == "cancelled" {
				continue
			}
			now := time.Now()
			startEvent, _ := time.Parse(time.RFC3339, event.Start.DateTime)
			timeUntilAlarm := startEvent.Sub(now)
			if timeUntilAlarm < 0 {
				log.WithFields(log.Fields{"at": startEvent.Format(time.RFC1123), "in": "   N/A   ", "name": event.Summary}).Info("Event is already in progress, skipping alarm")
				continue
			}

			log.WithFields(log.Fields{"at": startEvent.Format(time.RFC1123), "in": timeUntilAlarm, "name": event.Summary}).
				Info("Setting alarm for event")

			ctx, cancelAlarm := context.WithCancel(parentCtx)
			lock.Lock()
			alarms[event.Id] = cancelAlarm
			lock.Unlock()
			//a goroutine will fire when the event is about to start, unless this is cancelled
			go func() {
				timer := time.NewTimer(timeUntilAlarm)
				select {
				case <-timer.C:
					log.WithField("event", event.Summary).Info("Event is Starting")
					eventStarting <- event
				case <-ctx.Done():
					// this happened before the time so the alarm was cancelled
				}
			}()
		}
	}()

	return eventStarting
}
