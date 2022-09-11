package notifications

import (
	"fmt"
	"github.com/gabzim/meetings/server/calendarwh"
	"go.uber.org/zap"
)

// a webhook is an endpoint where google calendar pushes updates to us. Multiple web socket connections can subscribe to it.
// as soon as the first clients connects, we should start the webhook, if there's no more websocket clients listening to updates for it, we should close it
// this struct handles all those responsibilities.
type webhookWithClients struct {
	logger *zap.SugaredLogger
	base   string
	// the webhook where google will push updates
	wh *calendarwh.CalendarWebHookManaged
	// the web socket clients to whom we must forward the updates that come from google
	clients map[string]*wsClient
}

// AddClient Add a clients, if the webhook is not running, then run it.
func (w *webhookWithClients) AddClient(c *wsClient) {
	// if this is the first clients being added, initialize
	if w.clients == nil {
		w.clients = make(map[string]*wsClient)
	}
	if w.wh == nil {
		calServ := c.GetCalendarService()
		w.wh = calendarwh.New(calServ, c.calendarName, w.base+c.GetEmailAndCalendar(), w.logger)
	}

	w.clients[c.id] = c

	if !w.wh.IsRunning() {
		go w.StartWebhookAndForwardToAllClients()
	}
}

// RemoveClient remove a web socket clients, if it's the last one, tell google to stop updates to the webhook
// it returns true if the list of clients is empty and webhook was stopped, false if there are still clients connected to that wh
func (w *webhookWithClients) RemoveClient(c *wsClient) (bool, error) {
	// find which of the clients listening to that email & calendar disconnected (you may have more than one)
	_, found := w.clients[c.id]
	if !found {
		return false, fmt.Errorf("weird error unregistering a clients that could not be found among the entries")
	}

	delete(w.clients, c.id)
	noClientsLeft := len(w.clients) == 0
	// if you deleted the only clients left for this email + calendar, shut down the webhook and clean up
	if noClientsLeft {
		w.wh.Stop()
	}

	return noClientsLeft, nil
}

func (w *webhookWithClients) StartWebhookAndForwardToAllClients() error {
	events, err := w.wh.Start()
	if err != nil {
		w.logger.Errorf("error starting webhook for clients: %v", err)
		return err
	}
	// forward each one of the events received by the webhook to all the clients for that email + calendar
	for e := range events {
		for _, ws := range w.clients {
			ws.SendEvent(e)
		}
	}
	return nil
}
