package lights

import (
	"github.com/amimof/huego"
	"go.uber.org/zap"
	"image/color"
	"time"
)

var prevStateLights = make(map[int]huego.State)

func HoldStateForLights(lights []huego.Light) {
	for _, l := range lights {
		prevStateLights[l.ID] = *l.State
	}
}

func RestoreStateForLights(lights []huego.Light, log *zap.SugaredLogger) {
	var err error
	for _, light := range lights {
		err = light.Alert("none")
		if err != nil {
			log.Errorf("could not unset alert mode for light: %v", err)
		}
		err = light.SetState(prevStateLights[light.ID])
		if err != nil {
			log.Errorf("could not restore light state: %v, retrying...", err)
			time.Sleep(300 * time.Millisecond)
			light.SetState(prevStateLights[light.ID])
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func AlertLights(lights []huego.Light, color color.Color, pulse bool, log *zap.SugaredLogger) {
	var err error
	for _, l := range lights {
		err = l.Col(color)
		if err != nil {
			log.Errorf("Error setting light color: %v", err)
			continue
		}
		err = l.Bri(255)
		if err != nil {
			log.Errorf("Error setting light brightness: %v", err)
			continue
		}
		var alert string
		if pulse {
			alert = "lselect"
		} else {
			alert = "none"
		}
		err = l.Alert(alert)
		if err != nil {
			log.Errorf("Error alerting light: %v", err)
		}
	}
}
