package notifications

import (
	"fmt"
	"github.com/gabzim/meetings/server/services/auth"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"net/http"
	"strings"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func NewController(notifServ *Service, authServ *auth.Service, log *zap.SugaredLogger) *Controller {
	l := log.With("controller", "NotificationsController")
	return &Controller{serv: notifServ, auth: authServ, log: l}
}

type Controller struct {
	log  *zap.SugaredLogger
	serv *Service
	auth *auth.Service
}

func (c *Controller) RegisterClient(w http.ResponseWriter, r *http.Request) {
	t := r.URL.Query().Get("token")
	calendarName := r.URL.Query().Get("calendar")

	user, err := c.auth.AuthenticateUser(t)
	if err != nil {
		c.log.Errorf("could not authenticate user: %v", err)
		w.WriteHeader(404)
		fmt.Fprintf(w, "User not found")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		c.log.Errorf("could not upgrade connection: %v", err)
		w.WriteHeader(400)
		fmt.Fprintf(w, "Error upgrading: %v", err)
		return
	}

	c.serv.RegisterClient(user, calendarName, conn)
}

func (c *Controller) ReceivePushFromGoogle(w http.ResponseWriter, req *http.Request) {
	clientId := strings.TrimPrefix(req.URL.Path, "/push/") // what follows the /push is the clientId, eg: /push/:clientId
	c.serv.DispatchPushToClients(w, req, clientId)
}
