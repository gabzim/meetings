package web

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/http"
	"net/http/httputil"
	"time"
)

const (
	greet = `<?xml version="1.0" encoding="UTF-8"?>
			<Response>
				<Say voice="woman">Hello thanks for picking up</Say>
				<Hangup />
			</Response>`
)

func PhonePickedUpHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	reqBytes, err := httputil.DumpRequest(r, true)
	if err != nil {
		fmt.Println(string(reqBytes))
	}

	w.Header().Set("Content-Type", "text/xml")

	fmt.Fprint(w, greet)
}

type CalendarPushNotification struct {
	ChannelExpiration *time.Time
	ChannelId         string
	MessageNumber     string
	ResourceId        string
	ResourceState     string
	ResourceUri       string
}

func CalendarNotifications(notifications chan<- *CalendarPushNotification) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info("Push notification received")
		defer r.Body.Close()

		channelExpiration := r.Header.Get("X-Goog-webhook-Expiration")
		var channelExpirationTime *time.Time
		if channelExpiration != "" {
			t, err := time.Parse(time.RFC1123, channelExpiration)
			if err != nil {
				log.Errorf("Could not parse channel expiration time %v", err)
			}
			channelExpirationTime = &t
		}
		pushNotification := &CalendarPushNotification{
			ResourceId:        r.Header.Get("X-Goog-Resource-ID"),
			ChannelExpiration: channelExpirationTime,
			ChannelId:         r.Header.Get("X-Goog-Channel-ID"),
			MessageNumber:     r.Header.Get("X-Goog-Message-Number"),
			ResourceState:     r.Header.Get("X-Goog-Resource-State"),
			ResourceUri:       r.Header.Get("X-Goog-Resource-URI"),
		}

		w.Write([]byte("OK"))

		notifications <- pushNotification
	})
}
