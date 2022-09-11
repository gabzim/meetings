# go-calendar-twilio-alerts

Ever since I started having more distractions at home (babies, emergencies, etc) I find myself being late for meetings. 
This is an initial implementation that checks my google calendar and places a phone call to my cell whenever a meeting is about to start in case I miss the notifications (it can be extended to fire philips Hue animations, trigger alarms, send you push notifications, etc).

## Google Calendar Client
Steps to receive event starting notifications:
1. You need to obtain a google calendar service:
`calendarSrv, err := gcalnotifications.GetCalendarClient(config.GoogleCredentials)`
 If the file doesn't exist, you'll be taken to a google page to grant access to your calendar.
2. Now that you have a Google calendar client, you need to tell Gcal to start pushing event updates to you:
`pushNotificationsHook := gcalnotifications.New(calendarSrv, config.ApiEndpoint)`, the second argument is the URL to which google will POST updates, **this should be your app**.
The result of this call has 2 important methods and a field: `Start()`, `Stop()` and `Handler`. The `Handler` is the HTTP handler you need to mount on your webserver to handle Google's POSTS.

3. Instruct Google to start pumping event changes, you get a channel of event updates:
`events, err := pushNotificationsHook.Start()`

The events channel will contain new events/event updates/event deletions. If you're interested in setting alarms for when an event starts:
`eventStartingNotifications := gcalnotifications.NotifyEventStarting(ctx, events, 30 * time.Second)`

This takes a `context` object, so you can cancel all alarms, the events channel you created above, and how long before an event starts you want to be notified.
It returns a channel that emits every time an event is about to start. 

You can then range over this channel and do whatever you want:
```go
go func() {
		for startingEvent := range eventStartingNotifications {
			// do whatever you want, make phone calls/turn on lights/fire some fireworks...
		}
}()
```
4. So far you've told google to start pushing notifications to `config.GoogleCredentials`, but you need to handle this POSTs from google, so you need to start a web server
and mount the handler: 
`err = http.ListenAndServeTLS(config.Addr, config.TLSCertPath, config.TLSKeyPath, pushNotificationsHook.Handler)`

## Usage

Usage of the binary if you don't edit `main.go`:
    
      -a string
            Address to bind to for the http server that will receive notifications
    
      -callFrom string
            Twilio Phone number that will be used to make the phone calls
    
      -callbackUrl string
            URL you want Twilio to POST to when someone interacts with your phone call
    
      -cert string
            Path to TLS Cert file
      
      -credentialsPath string
            Path to the file you use for your gcloud credentials (you can set the path and the first time you run the app you'll be asked to grant permissions and this file will be generated for you)
      
      -endpoint string
            URL that google calendar will hit with push notifications (if you're in a local environment you can use serveo.net as a reverse proxy)
      
      -key string
            Path to TLS Key file (in case you need to start a TLS server, you might not need it if you're behind a Load Balancer)
      
      -phone string
            Phone number that will receive calls when a meeting is about to start (This is your phone number)
      
      -twilioSid string
            Your Twilio Account SID
      
      -twilioToken string
            Your Twilio Account Auth Token
            
    
