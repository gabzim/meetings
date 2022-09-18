package main

import (
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"os"
	"strings"
	"time"
)

func getEnvOrDefault(envName, fallback string) string {
	v := os.Getenv(envName)
	if v == "" {
		return fallback
	}
	return v
}

type NotificationsQuery struct {
	Host       string        `url:"-"`
	Email      string        `url:"email"`
	Token      string        `url:"token"`
	Calendar   string        `url:"calendar"`
	TimeBefore time.Duration `url:"-"`
	timeBefore string        `url:"timeBefore"`
}

var token = flag.String("t", os.Getenv("MEETINGS_API_TOKEN"), "Meetings token to authenticate with the API")
var calendarName = flag.String("c", "primary", "The calendar you want to inspect, by default: \"primary\"")
var before = flag.String("b", "30s", "how long before the event starts to fire the notification")

func obtainConfig() (*NotificationsQuery, error) {
	host := getEnvOrDefault("MEETINGS_SERVER_HOST", "meetings-api.gabrielzim.com")
	tokenPath := getEnvOrDefault("MEETINGS_API_TOKEN_PATH", "./meetings-token.txt")
	q := NotificationsQuery{
		Token:    *token,
		Calendar: *calendarName,
		Host:     host,
	}
	if *token == "" {
		// TODO handle errors
		f, _ := os.OpenFile(tokenPath, os.O_RDWR|os.O_CREATE, 0655)
		defer f.Close()
		all, _ := io.ReadAll(f)
		contents := string(all)
		if len(contents) > 1 {
			tAndE := strings.Split(contents, "\n")
			q.Email = tAndE[0]
			q.Token = tAndE[1]
		} else {
			email, t, _ := initiateTokenRequestFlow(q.Host)
			if len(t) > 0 {
				f.WriteString(fmt.Sprintf("%s\n%s", email, t)) // write email in first line, token in second line
				log.Infof("Token written to: %v", tokenPath)
				q.Token = t
				q.Email = email
			}
		}

		//return &q, errNoToken
	}
	tBefore, err := time.ParseDuration(*before)
	if err != nil {
		return &q, errBadDuration
	}

	q.TimeBefore = tBefore
	q.timeBefore = q.TimeBefore.String()
	return &q, nil
}

func initiateTokenRequestFlow(host string) (string, string, error) {
	fmt.Printf("Please: go to this link: https://%v/auth/google and paste here the token you obtain after signing in and press enter.\n", host)
	var email string
	var token string
	_, err := fmt.Scanf("%s %s", &email, &token)
	return email, token, err
}
