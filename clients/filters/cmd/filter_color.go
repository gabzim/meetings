package main

import (
	"golang.org/x/exp/slices"
	"google.golang.org/api/calendar/v3"
)

type EventColor string

const (
	Lavender  EventColor = "1"
	Sage      EventColor = "2"
	Grape     EventColor = "3"
	Flamingo  EventColor = "4"
	Banana    EventColor = "5"
	Tangerine EventColor = "6"
	Peacock   EventColor = "7"
	Graphite  EventColor = "8"
	Blueberry EventColor = "9"
	Basil     EventColor = "10"
	Tomato    EventColor = "11"
)

var colorNameToEventColor = map[string]EventColor{
	"lavender":  Lavender,
	"sage":      Sage,
	"grape":     Grape,
	"flamingo":  Flamingo,
	"banana":    Banana,
	"tangerine": Tangerine,
	"peacock":   Peacock,
	"graphite":  Graphite,
	"blueberry": Blueberry,
	"basil":     Basil,
	"tomato":    Tomato,
}

func FilterColor(colors ...EventColor) EventFilter {
	return func(es <-chan *calendar.Event) <-chan *calendar.Event {
		res := make(chan *calendar.Event)
		go func() {
			defer close(res)
			for e := range es {
				if slices.Contains(colors, EventColor(e.ColorId)) {
					res <- e
				}
			}
		}()
		return res
	}
}
