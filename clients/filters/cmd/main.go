package main

import (
	"context"
	"encoding/json"
	"github.com/jessevdk/go-flags"
	"go.uber.org/zap"
	"google.golang.org/api/calendar/v3"
	"io"
	"os"
	"strings"
	"time"
)

type EventFilter func(e <-chan *calendar.Event) <-chan *calendar.Event

type FilterOptions struct {
	ColorsStr      []string `short:"c" long:"colors" description:"if you want to filter events by color"`
	AttendeeEmail  string   `short:"e" long:"attendee" description:"If you want to filter by attendee status, this is the attendee email"`
	AttendeeStatus []string `short:"r" long:"attendee-status" description:"Attendee status you want to filter by, eg: confirmed, needsAction"`
	TimeBeforeStr  string   `short:"b" long:"time-before" description:"How long before event starts should we notify if the starting option was passed in"`
	SkipStarted    bool     `short:"k" long:"skip-started" description:"Skip events that have already started"`
}

func (o FilterOptions) Colors() ([]EventColor, error) {
	if len(o.ColorsStr) == 0 {
		return []EventColor{}, nil
	}
	res := make([]EventColor, len(o.ColorsStr), len(o.ColorsStr))
	for i, cStr := range o.ColorsStr {
		c := colorNameToEventColor[strings.ToLower(cStr)]
		res[i] = c
	}
	return res, nil
}

func main() {
	opts := FilterOptions{}
	_, err := flags.ParseArgs(&opts, os.Args)

	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewDevelopment()
	log := logger.Sugar()
	dec := json.NewDecoder(os.Stdin)
	events := make(chan *calendar.Event)
	go processEvents(events, opts, os.Stdout)
	for {
		var e calendar.Event
		err := dec.Decode(&e)
		if err != nil {
			log.Errorf("error decoding json: %v", err)
			os.Exit(1)
		}
		events <- &e
	}
}

func processEvents(e <-chan *calendar.Event, opts FilterOptions, w io.Writer) error {
	enc := json.NewEncoder(w)
	colors, err := opts.Colors()
	if err != nil {
		return err
	}
	if len(colors) > 0 {
		e = FilterColor(colors...)(e)
	}
	if opts.AttendeeEmail != "" && len(opts.AttendeeStatus) > 0 {
		e = FilterAttendeeStatus(opts.AttendeeEmail, opts.AttendeeStatus...)(e)
	}
	if len(opts.TimeBeforeStr) > 0 {
		timeBefore, err := time.ParseDuration(opts.TimeBeforeStr)
		if err != nil {
			return err
		}
		e = FilterStarting(context.Background(), timeBefore, opts.SkipStarted)(e)
	}
	for e := range e {
		enc.Encode(e)
	}
	return nil
}
