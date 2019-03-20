package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/gabzim/meetings/gcal"
	"github.com/sfreiberg/gotwilio"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type AppConfig struct {
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

var config AppConfig

func main() {
	ReadConfig(&config)

	twilio := gotwilio.NewTwilioClient(config.TwilioAccountSid, config.TwilioAuthToken)
	calendarSrv, err := gcal.GetCalendarClient("credentials.json")
	if err != nil {
		panic(err)
	}

	pushNotificationsHook, h := gcal.NewManagedCalendarWebHook(calendarSrv, config.ApiEndpoint)
	err = pushNotificationsHook.Start()
	if err != nil {
		panic(err)
	}

	//take events coming in and, WHEN THEY'RE ABOUT TO START, push them to eventStartingNotifications
	ctx, cancelAlarms := context.WithCancel(context.Background())
	eventStartingNotifications := gcal.NotifyEventStarting(ctx, pushNotificationsHook.Events, 30 * time.Second)

	// when an event is about to start, call phone
	go func() {
		for startingEvent := range eventStartingNotifications {
			makePhoneCall(twilio, config.CallFromNumber, config.PhoneNumber, config.CallbackUrl)
			log.WithFields(log.Fields{"startingEvent": startingEvent.Summary}).Printf("Calling %v now...", config.PhoneNumber)
		}
	}()

	go func() {
		// Ask Google to stop POSTing notifications when we stop the process
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
		<-signals
		log.Info("Closing push notification channel before exit...")
		cancelAlarms()
		err = pushNotificationsHook.Stop()
		if err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()

	//set up web server that will listen for google calendar push notifications using the handler returned when creating the webhook
	if config.TLSCertPath != "" && config.TLSKeyPath != "" {
		err = http.ListenAndServeTLS(config.Addr, config.TLSCertPath, config.TLSKeyPath, h)
	} else {
		err = http.ListenAndServe(config.Addr, h)
	}
}

func makePhoneCall(twilio *gotwilio.Twilio, from, to, callbackUrl string) (*gotwilio.VoiceResponse, *gotwilio.Exception, error) {
	callbackParams := gotwilio.NewCallbackParameters(callbackUrl)
	callbackParams.Timeout = 15
	call, ex, err := twilio.CallWithUrlCallbacks(from, to, callbackParams)
	if err != nil {
		return nil, nil, fmt.Errorf("error making phone call %v", err)
	}
	return call, ex, nil
}

func ReadConfig(config *AppConfig) {
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

	if config.PhoneNumber == "" || config.ApiEndpoint == "" || config.CallbackUrl == "" || config.CallFromNumber == "" || config.TwilioAuthToken == "" || config.TwilioAccountSid == "" || config.GoogleCredentials == "" {
		log.Fatalf("Missing configs, all flags must be set %+v", *config)
	}
}
