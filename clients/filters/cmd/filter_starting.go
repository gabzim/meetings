package main

import (
	"context"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/calendar/v3"
	"time"
)

type eventAlarm struct {
	event       *calendar.Event
	cancelAlarm context.CancelFunc
}

func FilterStarting(parentCtx context.Context, timeBeforeStart time.Duration, skipAlreadyStarted bool) EventFilter {
	return func(e <-chan *calendar.Event) <-chan *calendar.Event {
		return NotifyEventStarting(parentCtx, e, timeBeforeStart, skipAlreadyStarted)
	}
}

// NotifyEventStarting receives a channel of events (new/updated/deleted) and returns a channel that fires every time an event is about to start (or also if they've already started, depending on the value of the last argument).
func NotifyEventStarting(parentCtx context.Context, incomingEvents <-chan *calendar.Event, timeBeforeStart time.Duration, skipAlreadyStarted bool) <-chan *calendar.Event {
	// maps eventIds to a cancelFn we can call to cancel their alarms if the events are deleted or changed.
	eventAlarms := make(map[string]eventAlarm)

	eventStarting := make(chan *calendar.Event)

	go func() {
		for e := range incomingEvents {
			e := e
			logMsg := "Event received, setting alarm"

			eventAlarm, alarmIsSet := eventAlarms[e.Id]

			if alarmIsSet {
				// remove alarm if e has been cancelled or rescheduled
				if e.Status == "cancelled" {
					eventAlarm.cancelAlarm()
					delete(eventAlarms, e.Id)
					log.WithField("name", eventAlarm.event.Summary).Info("Event cancelled, alarm removed")
					continue
				} else if e.Start.DateTime == eventAlarm.event.Start.DateTime {
					//event start time hasn't changed, skip it
					continue
				} else if e.Start.DateTime != eventAlarm.event.Start.DateTime {
					// event start time has changed, cancel alarm and proceed to set a new one
					logMsg = "Event updated, resetting alarm"
					delete(eventAlarms, e.Id)
					eventAlarm.cancelAlarm()
				}
			}

			// if an event is cancelled but we didn't have an alarm set for it, just skip
			if e.Status == "cancelled" {
				continue
			}

			now := time.Now()
			eventEndTime, _ := time.Parse(time.RFC3339, e.End.DateTime)

			if eventEndTime.Before(now) {
				log.WithFields(log.Fields{"name": e.Summary}).Info("Event that already ended, skipping alarm...")
				// this event already ended, just ignore it
				continue
			}

			eventStartTime, _ := time.Parse(time.RFC3339, e.Start.DateTime)
			alarmTime := eventStartTime.Add(-timeBeforeStart)

			// if the event has already started
			if eventStartTime.Before(now) && skipAlreadyStarted {
				log.WithFields(log.Fields{"at": eventStartTime.Format(time.RFC1123), "in": "   N/A   ", "name": e.Summary}).Info("Event is already in progress, skipping alarm")
				continue
			}

			log.WithFields(log.Fields{"at": eventStartTime.Format(time.RFC1123), "in": alarmTime.Sub(now), "name": e.Summary}).
				Info(logMsg)

			eventAlarms[e.Id] = setAlarmForEvent(parentCtx, e, alarmTime, eventStarting)

			// We need to cleanup the map every now and then so it doesn't grow endlessly
			cleanupEventsAlarms(eventAlarms)

		}
		// if the event updates channel is closed, close the e starting channel as well
		close(eventStarting)
	}()
	return eventStarting
}

// setAlarmForEvent creates a goroutine that pushes the event to the passed alarmFired channel when alarmTime is reached.
// It returns an eventAlarm struct that has a cancelAlarm function in case you want to cancel this alarm (eg. if event is cancelled)
func setAlarmForEvent(parentCtx context.Context, event *calendar.Event, alarmTime time.Time, alarmFired chan *calendar.Event) eventAlarm {
	ctx, cancelAlarm := context.WithCancel(parentCtx)
	now := time.Now()
	durationUntilAlarm := alarmTime.Sub(now)
	alarm := eventAlarm{event: event, cancelAlarm: cancelAlarm}

	//event already started, fire now
	if durationUntilAlarm <= 0 {
		log.WithFields(log.Fields{"event": event.Summary, "startTime": event.Start.DateTime, "timeUntilAlarm": durationUntilAlarm}).Info("Event already started, notifying")
		alarmFired <- event
		return alarm
	}

	//a goroutine will fire when the event is about to start, unless this is cancelled
	go func() {
		// TODO maybe reuse timers with sync.Pool
		timer := time.NewTimer(durationUntilAlarm)
		select {
		case <-timer.C:
			log.WithField("event", event.Summary).Info("Firing alarm for event")
			alarmFired <- event
		case <-ctx.Done():
			// alarm cancelled
			timer.Stop()
		}
	}()

	return alarm
}

// removes from the map any event that has already ended
func cleanupEventsAlarms(eventAlarms map[string]eventAlarm) {
	now := time.Now()
	for id, e := range eventAlarms {
		endsAt, _ := time.Parse(time.RFC3339, e.event.End.DateTime)
		if endsAt.Before(now) {
			delete(eventAlarms, id)
		}
	}
}
