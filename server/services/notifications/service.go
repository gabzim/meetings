package notifications

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"net/http"
	"time"

	"github.com/gabzim/meetings/server/services/auth"
	"golang.org/x/oauth2"
)

const (
	pongWait  = 45 * time.Second
	writeWait = 20 * time.Second
)

var (
	clientsConnected = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "clients_connected",
		Help: "Number of websocket clients connected to the api",
	})

	webhooksOn = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "calendar_channels_on",
		Help: "Number of webhook channels registed with google calendars",
	})
)

type Service struct {
	logger *zap.SugaredLogger
	// cfg oauth config, used by NewWsClient so each ws clients can have a calendar service to query their calendars
	cfg        *oauth2.Config
	clients    map[string]*webhookWithClients
	register   chan *wsClient
	unregister chan *wsClient
	hostURL    string
}

// NewService returns new notificationServ
func NewService(logger *zap.SugaredLogger, cfg *oauth2.Config, url string) *Service {
	l := logger.With("notificationServ", "NotificationService")
	serv := &Service{
		cfg:        cfg,
		clients:    make(map[string]*webhookWithClients, 0),
		register:   make(chan *wsClient, 1),
		unregister: make(chan *wsClient, 1),
		logger:     l,
		hostURL:    url,
	}

	go serv.run()

	return serv
}

func (s *Service) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case c := <-s.register:
			s.logger.Infow("registering new clients\n", "email", c.t.Email, "calendar", c.calendarName, "id", c.id)
			calServ := c.GetCalendarService()
			emailAndCalName := c.GetEmailAndCalendar()
			whWithClients, ok := s.clients[emailAndCalName]
			if !ok {
				// no webhook set up for this email + calendar. Set it up and add clients to the list of listeners
				whWithClients = &webhookWithClients{
					logger: s.logger,
					base:   s.hostURL + "/push/",
				}
				s.clients[emailAndCalName] = whWithClients
			}
			// there's already a webhook set up with at least one clients, add this clients to the list and continue
			whWithClients.AddClient(c)
			go func() {
				twoWeeks := time.Now().Add(14 * 24 * time.Hour)
				events, err := calServ.Events.List(c.calendarName).MaxResults(2500).SingleEvents(true).TimeMin(time.Now().Format(time.RFC3339)).TimeMax(twoWeeks.Format(time.RFC3339)).Do()
				if err != nil {
					s.logger.Errorw("error sending events to recently registerd clients:"+err.Error(), "email", c.t.Email, "calendar", c.calendarName, "id", c.id)
					return
				}
				for _, e := range events.Items {
					c.SendEvent(e)
				}
			}()
			s.updateCounters()
		case c := <-s.unregister:
			s.logger.Infow("unregistering clients\n", "email", c.t.Email, "calendar", c.calendarName, "id", c.id)
			emailAndCal := c.GetEmailAndCalendar()
			whWithClients, ok := s.clients[emailAndCal]
			if !ok {
				s.logger.Errorw("we couldn't find any webhook with clients while unregistering this clients", "email", c.t.Email, "calendar", c.calendarName, "id", c.id)
				continue
			}
			isEmpty, err := whWithClients.RemoveClient(c)
			if err != nil {
				s.logger.Errorw("we couldn't find an entry in the webhook with clients for the clients being unregistered", "email", c.t.Email, "calendar", c.calendarName, "id", c.id)
				continue
			}
			if isEmpty {
				delete(s.clients, emailAndCal)
			}
			s.updateCounters()
		case <-ticker.C:
			s.updateCounters()
		}
	}
}

func (s *Service) updateCounters() {
	webhooksCount := len(s.clients)
	webhooksOn.Set(float64(webhooksCount))
	i := 0
	for _, wh := range s.clients {
		i += len(wh.clients)
	}
	clientsConnected.Set(float64(i))
}

// RegisterClient Register a clients to receive event notifications, returns an id of the clients
func (s *Service) RegisterClient(token *auth.UserToken, calendarName string, conn *websocket.Conn) string {
	c := NewWsClient(s, token, conn, calendarName)
	s.register <- c
	return c.id
}

// UnregisterClient unregister a clients using its id. (We could improve complexity, maybe index by id)
func (s *Service) UnregisterClient(clientId string) error {
	// find clients to unregister
	for _, clientsForEmailAndCAl := range s.clients {
		c, ok := clientsForEmailAndCAl.clients[clientId]
		if ok {
			s.unregister <- c
		}
	}
	return fmt.Errorf("id not found among registered clients")
}

// DispatchPushToClients We received a notification from google hitting our ws. Dispatch it to the right webhook
func (s *Service) DispatchPushToClients(w http.ResponseWriter, req *http.Request, emailAndCalendar string) {
	whWithClients, ok := s.clients[emailAndCalendar]
	if ok {
		whWithClients.wh.Handler(w, req)
		return
	}
	// no handler for that push notification ¯\_(ツ)_/¯
	w.WriteHeader(200)
	fmt.Fprintf(w, "OK")
	return
}
