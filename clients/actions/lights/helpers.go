package lights

import (
	"github.com/amimof/huego"
	"go.uber.org/zap"
	"image/color"
)

var prevStateLights = make(map[int]huego.State)
var prevStateRooms = make(map[int]huego.State)

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
			log.Errorf("could not restore light state: %v", err)
		}
	}
}

func RestoreStateForRooms(rooms []huego.Group, log *zap.SugaredLogger) {
	var err error
	for _, room := range rooms {
		err = room.Alert("none")
		if err != nil {
			log.Errorf("could not unset alert mode for room: %v", err)
		}
		prevState, ok := prevStateRooms[room.ID]
		if !ok {
			log.Errorf("previous state not found for room: %v", room.Name)
		}
		room.State.Alert = "none"
		if !prevState.On {
			err = room.Off()
			if err != nil {
				log.Errorf("error turning room off: %v", err)
			}
			continue
		}
		err = room.Bri(prevState.Bri)
		if err != nil {
			log.Errorf("error setting room brightness: %v", err)
		}
		switch prevState.ColorMode {
		case "ct":
			err = room.Ct(prevState.Ct)
		case "xy":
			err = room.Xy(prevState.Xy)
		case "hs":
			err = room.Hue(prevState.Hue)
		}
		if err != nil {
			log.Errorf("could not restore room color: %v", err)
		}
	}
}

func HoldStateForRooms(rooms []huego.Group) {
	for _, r := range rooms {
		prevStateRooms[r.ID] = *r.State
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

func AlertRooms(rooms []huego.Group, color color.Color, pulse bool, log *zap.SugaredLogger) {
	var err error
	for _, light := range rooms {
		err = light.Col(color)
		if err != nil {
			log.Errorf("Error setting light color: %v", err)
			continue
		}
		err = light.Bri(255)
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
		err = light.Alert(alert)
		if err != nil {
			log.Errorf("Error alerting light: %v", err)
		}
	}
}
