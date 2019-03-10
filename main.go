package main

import (
	"context"
	"flag"
	"fmt"
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
	Addr              string
	TLSCertPath       string
	TLSKeyPath        string
	ApiEndpoint       string
	GoogleCredentials string
	PhoneNumber       string
	CallFromNumber    string
	TwilioAccountSid  string
	TwilioAuthToken   string
	CallbackUrl       string
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
}

var config Config

func main() {
	ReadConfig(&config)

	twilio := gotwilio.NewTwilioClient(config.TwilioAccountSid, config.TwilioAuthToken)
	service, err := gcal.GetCalendarClient("credentials.json")
	if err != nil {
		panic(err)
	}

	// Ask gCalendar to start pushing calendar updates to our endpoint
	pushNotificationsHook := gcal.NewCalendarWebHookManaged(service, config.ApiEndpoint)

	err = pushNotificationsHook.Start()
	if err != nil {
		panic(err)
	}

	pushNotifications := make(chan *web.CalendarPushNotification)
	events := make(chan *calendar.Event, 250)
	eventStartingNotifications := make(chan *calendar.Event)

	// every time a push notification arrives: retrieve updated calendar events and push them to the events channel
	go func() {
		var syncToken string
		for pushNotification := range pushNotifications {
			// if we receive a push from a channel we haven't created, close it.
			if pushNotification.ChannelId != pushNotificationsHook.Webhook.Id {
				err := service.Channels.Stop(&calendar.Channel{Id: pushNotification.ChannelId, ResourceId: pushNotification.ResourceId}).Do()
				if err != nil {
					log.WithField("channel", pushNotification.ChannelId).Error("could not close lingering hook")
				} else {
					log.WithField("channel", pushNotification.ChannelId).Warn("closed lingering hook")
				}
			}
			if pushNotification.ResourceState == "sync" {
				syncToken = ""
			}
			// passing in syncToken means that the query will only retrieve deltas since the last query
			// if it's empty so we retrieve all events for the coming week
			fetchUpdatedEvents(service, events, &syncToken)
		}
	}()

	//take events coming in and, WHEN THEY'RE ABOUT TO START, push them to eventStartingNotifications
	gcal.NotifyEventStarting(context.Background(), events, eventStartingNotifications)

	// when an event is about to start, call phone
	go func() {
		for startingEvent := range eventStartingNotifications {
			makePhoneCall(twilio, config.CallFromNumber, config.PhoneNumber, config.CallbackUrl)
			log.WithFields(log.Fields{"startingEvent": startingEvent.Summary}).Printf("Calling %v now...", config.PhoneNumber)
		}
	}()

	// Ask Google to stop POSTing notifications when we stop the process
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-signals
		log.Warn("Closing push notification channel before exit...")
		pushNotificationsHook.Stop()
		os.Exit(0)
	}()

	http.HandleFunc("/twilio", web.PhonePickedUpHandler)
	http.HandleFunc("/", web.CalendarNotifications(pushNotifications))

	if err := http.ListenAndServeTLS(config.Addr, config.TLSCertPath, config.TLSKeyPath, nil); err != nil {
		pushNotificationsHook.Stop()
		panic(err)
	}
}

// fetchUpdatedEvents retrieves events from the calendar, if syncToken is passed, only deltas from the last query will be retrieved
// it then sets the value of syncToken to what the google calendar returned
func fetchUpdatedEvents(service *calendar.Service, events chan<- *calendar.Event, syncToken *string) {
	nextWeek := time.Now().Add(7 * 24 * time.Hour)
	upcomingEvents, err := getCalendarEvents(service, nextWeek, *syncToken)
	if err != nil {
		log.Errorf("Could not retrieve upcoming events: %v", err)
	}

	*syncToken = upcomingEvents.NextSyncToken

	for _, event := range upcomingEvents.Items {
		events <- event
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
		if events.Items[i].Start == nil || events.Items[j].Start == nil {
			return false
		}
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

func ReadConfig(config *Config) {
	var (
		defaultAddr       = os.Getenv("MEETINGS_ADDR")
		defaultCertPath   = os.Getenv("MEETINGS_CERT_PATH")
		defaultKeyPath    = os.Getenv("MEETINGS_KEY_PATH")
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
	if defaultAddr == "" {
		defaultAddr = ":8080"
	}
	flag.StringVar(&config.Addr, "a", defaultAddr, "Address to bind to for the http server that will receive notifications")
	flag.StringVar(&config.TLSCertPath, "cert", defaultCertPath, "Path to TLS Cert file")
	flag.StringVar(&config.TLSKeyPath, "key", defaultKeyPath, "Path to TLS Key file")
	flag.StringVar(&config.ApiEndpoint, "endpoint", apiEndpoint, "URL that google calendar will hit with push notifications")
	flag.StringVar(&config.GoogleCredentials, "credentialsPath", defaultCredsPath, "Path to the file you user for your credentials")
	flag.StringVar(&config.PhoneNumber, "phone", myPhoneNumber, "Phone number that will receive calls when a meeting is about to start")
	flag.StringVar(&config.CallFromNumber, "callFrom", twilioFromNumber, "Phone number that will be used to make the phone calls")
	flag.StringVar(&config.TwilioAccountSid, "twilioSid", twilioAccountSid, "Your Twilio Account SID")
	flag.StringVar(&config.TwilioAuthToken, "twilioToken", twilioAuthToken, "Your Twilio Account Auth Token")
	flag.StringVar(&config.CallbackUrl, "callbackUrl", twilioCallbackUrl, "URL you want Twilio to POST to when someone interacts with your phone call")
	flag.Parse()

	if config.PhoneNumber == "" || config.ApiEndpoint == "" || config.CallbackUrl == "" || config.CallFromNumber == "" || config.TwilioAuthToken == "" || config.TwilioAccountSid == "" || config.GoogleCredentials == "" || config.TLSCertPath == "" || config.TLSKeyPath == "" {
		log.Fatalf("Missing configs, all flags must be set %+v", *config)
	}
}
