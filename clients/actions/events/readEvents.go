package events

import (
	"context"
	"encoding/json"
	"google.golang.org/api/calendar/v3"
	"io"
)

func ReadEvents(ctx context.Context, i io.Reader) <-chan *calendar.Event {
	dec := json.NewDecoder(i)
	events := make(chan *calendar.Event, 5)
	go func() {
		defer close(events)
		for {
			var e calendar.Event
			err := dec.Decode(&e)
			if err != nil || ctx.Err() != nil {
				return
			}
			events <- &e
		}
	}()
	return events
}
