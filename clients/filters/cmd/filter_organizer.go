package main

import (
	"google.golang.org/api/calendar/v3"
)

func FilterOrganizer(organizerEmail string) EventFilter {
	return func(es <-chan *calendar.Event) <-chan *calendar.Event {
		res := make(chan *calendar.Event)
		go func() {
			defer close(res)
			for e := range es {
				if e.Organizer.Email == organizerEmail {
					res <- e
				}
			}
		}()
		return res
	}
}
