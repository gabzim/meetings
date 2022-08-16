package notificationserver

import "github.com/labstack/echo/v4"

type NotificationServer struct {
}

func (s *NotificationServer) Start(addr string) {
	e := echo.New()
	e.GET("hello")
	e.Start(addr)
}
