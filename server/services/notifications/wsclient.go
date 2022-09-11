package notifications

import (
	"context"
	"fmt"
	"github.com/dchest/uniuri"
	"github.com/gabzim/meetings/server/services/auth"
	"github.com/gorilla/websocket"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"time"
)

func generateId() string {
	return uniuri.New()
}

func NewWsClient(s *Service, t *auth.UserToken, conn *websocket.Conn, calendarName string) *wsClient {
	ts := s.cfg.TokenSource(context.Background(), t.GetOauthToken())
	serv, _ := calendar.NewService(context.Background(), option.WithTokenSource(ts))
	c := wsClient{
		id:               generateId(),
		conn:             conn,
		calendarName:     calendarName,
		events:           make(chan *calendar.Event),
		t:                t,
		notificationServ: s,
		calendarServ:     serv,
	}

	c.conn.SetPongHandler(func(appData string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	go c.ReadPump()
	go c.WritePump()

	return &c
}

type wsClient struct {
	id               string
	conn             *websocket.Conn
	calendarName     string
	events           chan *calendar.Event
	t                *auth.UserToken
	notificationServ *Service
	calendarServ     *calendar.Service
}

func (c *wsClient) GetCalendarService() *calendar.Service {
	return c.calendarServ
}

func (c *wsClient) GetEmailAndCalendar() string {
	return c.t.Email + "_" + c.calendarName
}

// description
func (c *wsClient) SendEvent(e *calendar.Event) {
	c.events <- e
}

// ReadPump discards messages
func (c *wsClient) ReadPump() {
	for {
		_, _, err := c.conn.NextReader()
		if err != nil {
			c.Close()
			return
		}
	}
}
func (c *wsClient) WritePump() {
	ping := time.NewTicker(10 * time.Second)
	defer func() {
		ping.Stop()
		c.Close()
	}()
	for {
		select {
		case <-ping.C:
			if c.conn == nil {
				return
			}
			err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
			if err != nil {
				return
			}
		case e := <-c.events:
			if c.conn == nil {
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteJSON(e)
			if err != nil {
				return
			}
		}
	}
}

func (c *wsClient) Close() {
	if c.conn == nil {
		return
	}
	err := c.conn.Close()
	if err != nil {
		fmt.Println(err)
	}
	c.notificationServ.UnregisterClient(c.id)
	c.conn = nil
}
