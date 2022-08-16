package notifications

import (
	"net"
	"time"
)

type CalendarNotificationsService struct {
}

func (s *CalendarNotificationsService) addClient(token string, calendar string, timeBefore time.Duration, conn net.Conn) {

}
