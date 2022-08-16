package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gobwas/ws"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/google"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func main() {
    googleClientId = os.Getenv("GOOGLE_KEY")
	goth.UseProviders(google.New(googleClientId, os.Getenv("GOOGLE_SECRET"), "https://meetings.sa.ngrok.io/auth/google/callback", calendar.CalendarReadonlyScope, "email"))

	authCtrl := &GoogleAuthController{}
	notificationsCtrl := &CalendarNotificationsController{}

	http.HandleFunc("/auth/google", authCtrl.initiateAuth)
	http.HandleFunc("/auth/google/callback", authCtrl.completeAuth)
	http.HandleFunc("/notifications", notificationsCtrl.registerClient)

	http.ListenAndServe(":8080", nil)
}

type GoogleAuthController struct {
}

func (c *GoogleAuthController) initiateAuth(w http.ResponseWriter, r *http.Request) {
	req := r.WithContext(context.WithValue(r.Context(), "provider", "google"))
	user, err := gothic.CompleteUserAuth(w, req)
	if err == nil {
		fmt.Fprint(w, user.Email)
	} else {
		gothic.BeginAuthHandler(w, req)
	}
}

func (c *GoogleAuthController) completeAuth(w http.ResponseWriter, r *http.Request) {
	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		return
	}
    token := oauth2.Token{AccessToken: user.AccessToken, RefreshToken: user.RefreshToken, Expiry: user.ExpiresAt}
    config := &oauth2.Config{
ClientID: ,
    }

    calendar.NewService(context.TODO(), )
	fmt.Fprint(w, user.Email)
}

type CalendarNotificationsController struct {
	serv CalendarNotificationsService
}

func (c *CalendarNotificationsController) registerClient(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		fmt.Fprintf(w, "Error upgrading to ws")
		return
	}

	query := r.URL.Query()
	clientId := query.Get("clientId")
	calendar := query.Get("calendar")
	timeBeforeStr := query.Get("timeBefore")
	if clientId == "" || calendar == "" || timeBeforeStr == "" {
		fmt.Fprintf(w, "error, clientId, calendar and timeBefore are required query parameters")
		return
	}

	timeBefore, err := time.ParseDuration(timeBeforeStr)
	if err != nil {
		fmt.Fprintf(w, "error parsing timeBefore")
		return
	}

	c.serv.addClient(clientId, calendar, timeBefore, conn)
}

