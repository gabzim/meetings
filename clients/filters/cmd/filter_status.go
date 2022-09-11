package main

import (
	"google.golang.org/api/calendar/v3"
)

func FilterAttendeeStatus(attendeeEmail string, status ...string) EventFilter {
	indexedStatus := make(map[string]struct{})
	for _, s := range status {
		indexedStatus[s] = struct{}{}
	}
	return func(es <-chan *calendar.Event) <-chan *calendar.Event {
		res := make(chan *calendar.Event)
		go func() {
			defer close(res)
			for e := range es {
				for _, a := range e.Attendees {
					if a.Email == attendeeEmail {
						_, ok := indexedStatus[e.Status]
						if ok {
							res <- e
						}
					}
				}
			}
		}()
		return res
	}
}
