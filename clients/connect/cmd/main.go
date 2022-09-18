package main

import (
	"encoding/json"
	"errors"
	"github.com/google/go-querystring/query"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"google.golang.org/api/calendar/v3"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	pongWait = 45 * time.Second
)

var (
	errNoToken      = errors.New("NO_API_TOKEN")
	errUserNotFound = errors.New("EMAIl_NOT_FOUND_IN_DB")
	errTokenInvalid = errors.New("INVALID_TOKEN_FOR_EMAIL")
	errBadDuration  = errors.New("BAD_DURATION_PASSED_IN")
)

func connectToWs(q *NotificationsQuery) (*websocket.Conn, error) {
	qs, _ := query.Values(q)

	u := url.URL{Scheme: "wss", Host: q.Host, Path: "/notifications", RawQuery: qs.Encode()}

	c, res, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if res != nil && res.StatusCode > 300 {
		switch res.StatusCode {
		case 404:
			return c, errUserNotFound
		case 401:
			return c, errTokenInvalid
		}
	}
	return c, err
}

func main() {
	logger, _ := zap.NewProduction()
	log := logger.Sugar()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGKILL)

	q, err := obtainConfig()
	if err != nil {
		HandleConfigErrors(err, log, q)
	}

	conn, err := connectToWs(q)
	if err != nil {
		log.Fatalf("Error connecting to ws: %v", err)
	}
	defer conn.Close()

	events := ReadEventsFromWs(log, conn)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	conn.SetPongHandler(func(appData string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	enc := json.NewEncoder(os.Stdout)

	disconnect := func() {
		err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			log.Fatalf("error closing websocket connection: %v", err)
		}
	}

	for {
		select {
		case e, ok := <-events:
			if !ok {
				disconnect()
				return
			}
			err := enc.Encode(e)
			if err != nil {
				disconnect()
				return
			}
		case <-ticker.C:
			conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(pongWait))
		case <-interrupt:
			log.Info("interrupt received")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			disconnect()
			select {
			case <-events:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

func HandleConfigErrors(err error, log *zap.SugaredLogger, q *NotificationsQuery) {
	if errors.Is(err, errNoToken) {
		log.Fatalf(`You don't have a token set. If you have one, you can set it on MEETINGS_API_TOKEN or if you have more than one, pass it using -t.
If you don't have one, you can obtain one from %v/auth/google.`, q.Host)
	} else if errors.Is(err, errBadDuration) {
		log.Fatalf(`The duration you passed in is wrong. You passed in: %v. Valid examples are: 30s, 1m, 0s.`, *before)
	}
	panic(err)
}

// ReadEventsFromWs takes in a websocket connections and a duration `d`, receives events from the ws and returns a channel that emits events `d` moments before they start
func ReadEventsFromWs(log *zap.SugaredLogger, c *websocket.Conn) <-chan *calendar.Event {
	es := make(chan *calendar.Event, 100)

	go func() {
		// read from ws
		defer close(es)
		for {
			var e calendar.Event
			err := c.ReadJSON(&e)
			if err != nil {
				log.Errorf("read:", err)
				return
			}
			es <- &e
		}
	}()

	return es
}
