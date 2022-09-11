package main

import (
	"context"
	"encoding/json"
	"github.com/gabzim/meetings/clients/actions/alerter"
	"github.com/gabzim/meetings/clients/actions/events"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

//func main() {
//	n := alerter.Notification{
//		Title:   "Confirm it bitch",
//		Message: "Click to join",
//		AppIcon: "https://ssl.gstatic.com/calendar/images/dynamiclogo_2020q4/calendar_10_2x.png",
//	}
//	confirm, _ := alerter.Confirm(&n)
//	if confirm {
//		fmt.Println("confirmed")
//	}
//}

func main() {
	logger, _ := zap.NewDevelopment()
	log := logger.Sugar()
	ctx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, unix.SIGKILL)

	go func() {
		<-interrupt
		cancel()
		<-time.After(1 * time.Second)
		os.Exit(0)
	}()

	es := events.ReadEvents(ctx, os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for e := range es {
		enc.Encode(e)
		n := alerter.Notification{
			Title:   e.Summary,
			Message: "Click to join",
		}
		confirm, err := alerter.Confirm(&n)
		if err != nil {
			panic(err)
		}
		if confirm {
			log.Infow("Joining event", "event", e.Summary)
			exec.Command("open", e.HangoutLink).Run()
		} else {
			log.Infow("User decided not to join", "event", e.Summary)
		}
	}

}
