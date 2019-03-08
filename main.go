package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/sfreiberg/gotwilio"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/calendar/v3"
	"meetings/gcal"
	"meetings/web"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"
)

type Config struct {
	ApiEndpoint       string
	GoogleCredentials string
	PhoneNumber       string
	CallFromNumber    string
	TwilioAccountSid  string
	TwilioAuthToken   string
	CallbackUrl       string
}

func main() {
	var config Config
	ReadConfig(&config)

	twilio := gotwilio.NewTwilioClient(config.TwilioAccountSid, config.TwilioAuthToken)
	service, err := gcal.GetCalendarClient("credentials.json")
	if err != nil {
		panic(err)
	}

	pushNotificationsHook, err := createCalendarPushNotificationHook(service, config.ApiEndpoint)
	if err != nil {
		panic(err)
	}
	log.WithField("expires", time.Unix(pushNotificationsHook.Expiration/1000, 0)).Infof("Started channel %v \n", pushNotificationsHook.Id)

	pushNotifications := make(chan *web.CalendarPushNotification)
	events := make(chan *calendar.Event)
	eventStartingNotifications := make(chan *calendar.Event)

	// every time a push notification arrives, fetch calendar events and push them to the events channel
	go handlePushNotifications(service, pushNotifications, events)

	//take events coming in and, WHEN THEY'RE ABOUT TO START, push them to eventStartingNotifications
	go gcal.NotifyEventStarting(context.Background(), events, eventStartingNotifications)

	// handles making the phone calls when an event is about to start
	go func() {
		for event := range eventStartingNotifications {
			makePhoneCall(twilio, config.CallFromNumber, config.PhoneNumber, config.CallbackUrl)
			log.WithFields(log.Fields{"event": event.Summary}).Printf("Calling %v now...", config.PhoneNumber)
		}
	}()

	// Ask Google to stop POSTing notifications when we stop the process
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		log.Warn("Closing push notification channel before exit...")
		err := service.Channels.Stop(pushNotificationsHook).Do()
		if err != nil {
			log.Errorf("Could not stop channel %v", pushNotificationsHook.Id)
		} else {
			log.Infof("Stopped channel %v \n", pushNotificationsHook.Id)
		}
		os.Exit(0)
	}()

	http.HandleFunc("/twilio", web.PhonePickedUpHandler)
	http.HandleFunc("/", web.CalendarNotifications(pushNotifications))

	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}

func handlePushNotifications(service *calendar.Service, notifications <-chan *web.CalendarPushNotification, events chan<- *calendar.Event) {
	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	var syncToken string
	for pushNotification := range notifications {
		if pushNotification.ResourceState == "sync" {
			syncToken = ""
		}
		// passing in syncToken means that the query will only retrieve deltas since the last query,
		// the first time it's empty so we retrieve all events for the coming week
		upcomingEvents, err := getCalendarEvents(service, nextWeek, syncToken)
		if err != nil {
			panic(err)
		}
		syncToken = upcomingEvents.NextSyncToken
		for _, event := range upcomingEvents.Items {
			events <- event
		}
	}
}

func getCalendarEvents(service *calendar.Service, to time.Time, syncToken string) (*calendar.Events, error) {
	eventQuery := service.Events.
		List("primary").
		MaxResults(20).
		SingleEvents(true)

	if syncToken != "" {
		eventQuery.SyncToken(syncToken)
	} else {
		eventQuery.TimeMin(time.Now().Format(time.RFC3339)).TimeMax(to.Format(time.RFC3339))
	}

	events, err := eventQuery.Do()

	if err != nil {
		return nil, err
	}

	sort.Slice(events.Items, func(i, j int) bool {
		return events.Items[i].Start.DateTime < events.Items[j].Start.DateTime
	})

	return events, nil
}

func makePhoneCall(twilio *gotwilio.Twilio, from, to, callbackUrl string) (*gotwilio.VoiceResponse, *gotwilio.Exception, error) {
	callbackParams := gotwilio.NewCallbackParameters(callbackUrl)
	callbackParams.Timeout = 20
	call, ex, err := twilio.CallWithUrlCallbacks(from, to, callbackParams)
	if err != nil {
		return nil, nil, fmt.Errorf("error making phone call %v", err)
	}
	return call, ex, nil
}

// createCalendarPushNotificationHook tells Google Calendar to post calendar update notifications to the URL you pass in
func createCalendarPushNotificationHook(service *calendar.Service, url string) (*calendar.Channel, error) {
	channel := &calendar.Channel{
		Id:      uuid.New().String(),
		Address: url,
		Type:    "web_hook",
	}

	createdChannel, err := service.Events.Watch("primary", channel).Do()
	if err != nil {
		return nil, err
	}

	return createdChannel, nil
}

func ReadConfig(config *Config) {
	var (
		defaultCredsPath  = os.Getenv("MEETINGS_GOOGLE_CREDENTIALS_PATH")
		apiEndpoint       = os.Getenv("MEETINGS_API_ENDPOINT")
		myPhoneNumber     = os.Getenv("MEETINGS_MY_NUMBER")
		twilioFromNumber  = os.Getenv("MEETINGS_TWILIO_FROM_NUMBER")
		twilioAccountSid  = os.Getenv("MEETINGS_TWILIO_ACCOUNT_SID")
		twilioAuthToken   = os.Getenv("MEETINGS_TWILIO_ACCOUNT_TOKEN")
		twilioCallbackUrl = os.Getenv("MEETINGS_TWILIO_CALLBACK_URL")
	)

	if defaultCredsPath == "" {
		defaultCredsPath = "credentials.json"
	}
	flag.StringVar(&config.ApiEndpoint, "endpoint", apiEndpoint, "URL that google calendar will hit with push notifications")
	flag.StringVar(&config.GoogleCredentials, "credentialsPath", defaultCredsPath, "Path to the file you user for your credentials")
	flag.StringVar(&config.PhoneNumber, "phone", myPhoneNumber, "Phone number that will receive calls when a meeting is about to start")
	flag.StringVar(&config.CallFromNumber, "callFrom", twilioFromNumber, "Phone number that will be used to make the phone calls")
	flag.StringVar(&config.TwilioAccountSid, "twilioSid", twilioAccountSid, "Your Twilio Account SID")
	flag.StringVar(&config.TwilioAuthToken, "twilioToken", twilioAuthToken, "Your Twilio Account Auth Token")
	flag.StringVar(&config.CallbackUrl, "callbackUrl", twilioCallbackUrl, "URL you want Twilio to POST to when someone interacts with your phone call")
	flag.Parse()

	if config.PhoneNumber == "" || config.ApiEndpoint == "" || config.CallbackUrl == "" || config.CallFromNumber == "" || config.TwilioAuthToken == "" || config.TwilioAccountSid == "" || config.GoogleCredentials == "" {
		log.Fatalf("Missing configs, all flags must be set %+v", *config)
	}
}