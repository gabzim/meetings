package main

import (
	"flag"
	"fmt"
	"github.com/sfreiberg/gotwilio"
	"google.golang.org/api/calendar/v3"
	"meeting-alert/gcal"
	"meeting-alert/web"
	"net/http"
	"os"
	"sort"
	"time"
)

type Config struct {
	PhoneNumber      string
	CallFromNumber   string
	TwilioAccountSid string
	TwilioAuthToken  string
	CallbackUrl      string
}

func main() {
	var config Config
	flag.StringVar(&config.PhoneNumber, "phone", os.Getenv("TWILIO_TO_NUMBER"), "Phone number that will receive calls when a meeting is about to start")
	flag.StringVar(&config.CallFromNumber, "callFrom", os.Getenv("TWILIO_FROM_NUMBER"), "Phone number that will be used to make the phone calls")
	flag.StringVar(&config.TwilioAccountSid, "twilioSid", os.Getenv("TWILIO_ACCOUNT_SID"), "Your Twilio Account SID")
	flag.StringVar(&config.TwilioAuthToken, "twilioToken", os.Getenv("TWILIO_ACCOUNT_TOKEN"), "Your Twilio Account Auth Token")
	flag.StringVar(&config.CallbackUrl, "callbackUrl", os.Getenv("TWILIO_CALLBACK_URL"), "URL you want Twilio to POST to when someone interacts with your phone call")
	flag.Parse()

	twilio := gotwilio.NewTwilioClient(config.TwilioAccountSid, config.TwilioAuthToken)

	service, err := gcal.GetCalendarClient()

	if err != nil {
		panic(err)
	}

	go func() {

		// start a ticker at a round minute in the hour, eg: if it's 04:03:20, wait until it's 04:05:00
		now := time.Now()
		timeUntilStart := getNextStartTime(now).Sub(now)
		time.Sleep(timeUntilStart - 30*time.Second)
		ticks := time.Tick(5 * time.Minute)
		for now := range ticks {
			events, _ := getUpcomingEvents(service, now, now.Add(time.Minute))
			for _, event := range events {
				eventStart, _ := time.Parse(time.RFC3339, event.Start.DateTime)
				if eventStart.Sub(now).Minutes() < 5 {
					fmt.Printf("Making a phone call: for meeting: %v, starting at: %v", event.Summary, event.Start.DateTime)
					makePhoneCall(twilio, config.CallFromNumber, config.PhoneNumber, config.CallbackUrl)
				}
			}
		}
	}()

	http.HandleFunc("/twilio", web.PhonePickedUpHandler)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}

// getNextStartTime we want to start checking for events at :00, :05, :10, quarter past, etc. From the given date,
// get the next time
func getNextStartTime(from time.Time) time.Time {
	nanoSecondsUntilRoundTime := time.Duration(1000000000 - from.Nanosecond()%1000000000)
	startTime := from.Add(nanoSecondsUntilRoundTime)
	secondsUntilMin := time.Duration(60-startTime.Second()%60) * time.Second
	startTime = startTime.Add(secondsUntilMin)
	minutesUntilRoundTime := time.Duration(5-startTime.Minute()%5) * time.Minute
	startTime = startTime.Add(minutesUntilRoundTime)
	return startTime
}

func getUpcomingEvents(service *calendar.Service, from, to time.Time) ([]*calendar.Event, error) {
	events, err := service.Events.
		List("primary").
		MaxResults(20).
		TimeMin(from.Format(time.RFC3339)).
		TimeMax(to.Format(time.RFC3339)).
		SingleEvents(true).
		Do()

	if err != nil {
		return nil, err
	}

	sort.Slice(events.Items, func(i, j int) bool {
		return events.Items[i].Start.DateTime < events.Items[j].Start.DateTime
	})

	result := make([]*calendar.Event, 0, len(events.Items))

	for _, event := range events.Items {
		if event.Start == nil && event.End == nil {
			continue
		}
		result = append(result, event)
	}

	return result, nil
}

func makePhoneCall(twilio *gotwilio.Twilio, from, to, callbackUrl string) {
	callbackParams := gotwilio.NewCallbackParameters(callbackUrl)
	callbackParams.Timeout = 20
	res, ex, err := twilio.CallWithUrlCallbacks(from, to, callbackParams)

	fmt.Printf("%+v\n", res)
	fmt.Printf("%+v\n", ex)
	fmt.Printf("%+v\n", err)

}
