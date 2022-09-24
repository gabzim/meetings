package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/amimof/huego"
	"github.com/gabzim/meetings/clients/actions/events"
	lights2 "github.com/gabzim/meetings/clients/actions/lights"
	"github.com/jessevdk/go-flags"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
	"image/color"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

//func signUp() error {
//	bridge, err := huego.Discover()
//	if err != nil {
//		return err
//	}
//	user, _ := bridge.CreateUser("meetings-lights") // Link button needs to be pressed
//	bridge = bridge.Login(user)
//	return nil
//}

var (
	errNoLights     = errors.New("no lights provided, you must choose either lights or rooms or all")
	errInvalidColor = errors.New("invalid format")
)

type Opts struct {
	Lights     []string `short:"l" long:"lights" description:"Lights you want to change when std in is received"`
	Rooms      []string `short:"r" long:"rooms" description:"Rooms you want to change when std in is received"`
	All        bool     `short:"a" long:"all" description:"Use all lights instead of specific ones or specific rooms"`
	ColorStr   string   `short:"c" long:"color" description:"ColorStr the light/room should transition too"`
	UserToken  string   `short:"t" long:"token" description:"Token used to sign in to hue hub"`
	BridgeAddr string   `short:"h" long:"host" description:"Host addr of the hue bridge, eg: 192.168.50.20"`
}

func (o *Opts) Color() color.Color {
	c, err := ParseHexColor(o.ColorStr)
	if err != nil {
		panic(err)
	}
	return c
}

func (o *Opts) Validate() error {
	if !o.All && len(o.Rooms) == 0 && len(o.Lights) == 0 {
		return errNoLights
	}
	if len(o.ColorStr) < 7 {
		return errInvalidColor
	}
	_, err := ParseHexColor(o.ColorStr)
	if err != nil {
		return err
	}
	return nil
}

func getConfig() (*Opts, error) {
	var opts Opts
	_, err := flags.Parse(&opts)
	if opts.UserToken == "" {
		opts.UserToken = os.Getenv("MEETINGS_LIGHTS_USER")
	}

	if opts.BridgeAddr == "" {
		opts.BridgeAddr = os.Getenv("MEETINGS_LIGHTS_BRIDGE_ADDR")
	}

	if opts.UserToken == "" || opts.BridgeAddr == "" {
		// 1. read from f system
		var path string
		if os.Getenv("MEETINGS_LIGHTS_CFG_PATH") != "" {
			path = os.Getenv("MEETINGS_LIGHTS_CFG_PATH")
		} else {
			path = "/tmp/hue.config"
		}
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0655)
		defer f.Close()
		if err != nil {
			return nil, err
		}

		// 2. attempt to read cfg from file
		cfg, err := io.ReadAll(f)
		if err != nil {
			return nil, err
		}
		// 3a. We found a cfg file, try to use it
		if len(string(cfg)) > 0 {
			hAndUser := strings.Split(string(cfg), "\n")
			if len(hAndUser) > 1 {
				opts.BridgeAddr = hAndUser[0]
				opts.UserToken = hAndUser[1]
				return &opts, nil
			}
		}

		// 3b. We didn't find a file, create one
		bridge, err := huego.Discover()
		if err != nil {
			return nil, err
		}

		var confirm string
		fmt.Println("We will attempt to create a user for first time use. Go press the button in the hub and then press enter...")
		fmt.Scanln(&confirm)

		user, err := bridge.CreateUser("meetings-lights")
		if err != nil {
			return nil, err
		}
		// 4. Store info in file for next use
		_, err = f.WriteString(fmt.Sprintf("%v\n%v", bridge.Host, user))
		if err != nil {
			return nil, err
		}
		opts.UserToken = user
		opts.BridgeAddr = bridge.Host
	}

	return &opts, err
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, unix.SIGKILL)
	go func() {
		<-signals
		cancel()
		select {
		case <-signals:
		case <-time.After(5 * time.Second):
		}
		os.Exit(0)
	}()

	logger, _ := zap.NewDevelopment()
	log := logger.Sugar()
	opts, err := getConfig()
	if err != nil {
		log.Fatalf("error getting config: %v", err)
	}
	bridge := huego.New(opts.BridgeAddr, opts.UserToken)

	lights, err := getLightsAndRoomsFromOpts(bridge, opts)
	if err != nil {
		log.Fatalf("error retrieving lights from options: %v", err)
	} else if len(lights) == 0 {
		log.Fatalf("No lights or rooms passed in, lights passed it: %v. groups passed in: %v", strings.Join(opts.Lights, ","), strings.Join(opts.Rooms, ","))
	}

	enc := json.NewEncoder(os.Stdout)
	es := events.ReadEvents(ctx, os.Stdin)

	for e := range es {
		enc.Encode(e)
		log.Infow("Flashing lights for events", "event", e.Summary)
		TriggerLights(ctx, bridge, lights, opts.Color(), 2*time.Minute, log)
		log.Infow("Ending Flashing lights, restoring to previous state", "event", e.Summary)
		if err != nil {
			log.Errorf("Error triggering lights: %v", err)
		}
	}
}

func getLightsAndRoomsFromOpts(bridge *huego.Bridge, opts *Opts) ([]huego.Light, error) {
	// user wants to trigger all lights
	lights, err := bridge.GetLights()
	if opts.All {
		return lights, err
	}

	lightsById := make(map[int]huego.Light)
	for _, light := range lights {
		lightsById[light.ID] = light
	}

	// user wants to trigger some (not all) lights
	resLights := make([]huego.Light, 0)

	if len(opts.Lights) > 0 {
		ixLightName := make(map[string]struct{})
		for _, lName := range opts.Lights {
			ixLightName[strings.ToLower(lName)] = struct{}{}
		}
		lights, err := bridge.GetLights()
		if err != nil {
			return resLights, err
		}
		for _, l := range lights {
			l := l
			_, ok := ixLightName[strings.ToLower(l.Name)]
			if ok {
				resLights = append(resLights, l)
			}
		}
	}

	if len(opts.Rooms) > 0 {
		groups, err := bridge.GetGroups()
		if err != nil {
			return resLights, err
		}
		for _, rname := range opts.Rooms {
			roomName := strings.ToLower(strings.Trim(rname, " "))
		optLoop:
			for _, g := range groups {
				g := g
				groupName := strings.ToLower(g.Name)
				if strings.HasPrefix(groupName, roomName) {
					for _, idStr := range g.Lights {
						id, _ := strconv.Atoi(idStr)
						light := lightsById[id]
						resLights = append(resLights, light)
					}
					break optLoop
				}
			}
		}
	}

	return resLights, nil
}

func RefreshLights(bridge *huego.Bridge, lights []huego.Light) ([]huego.Light, error) {
	refreshed := make([]huego.Light, 0)
	allLights, err := bridge.GetLights()
	if err != nil {
		return refreshed, err
	}

	allLightsById := make(map[int]*huego.Light)
	for _, l := range allLights {
		l := l
		allLightsById[l.ID] = &l
	}

	for _, l := range lights {
		refreshedLight, ok := allLightsById[l.ID]
		if ok {
			refreshed = append(refreshed, *refreshedLight)
		}
	}
	return refreshed, nil
}

func TriggerLights(ctx context.Context, bridge *huego.Bridge, lightList []huego.Light, color color.Color, dur time.Duration, log *zap.SugaredLogger) error {
	lights, err := RefreshLights(bridge, lightList)
	if err != nil {
		return err
	}
	// 1. remember state of lights and rooms
	lights2.HoldStateForLights(lights)

	// 2. pulse light and rooms (the true means pulse the light)
	lights2.AlertLights(lights, color, true, log)

	select {
	case <-time.After(15 * time.Second):
	case <-ctx.Done():
	}

	// 3. ensure light and rooms remain at full brightness with the alarm color after pulsing (pulsing can leave lights with low brightness sometimes)
	lights2.AlertLights(lights, color, false, log)

	select {
	case <-time.After(dur):
	case <-ctx.Done():
	}

	// 4. restore lights
	lights2.RestoreStateForLights(lights, log)

	return nil
}

// Living room
// Main Bedroom
// Entertainment area
// Balcony
// Kitchen
// Dressing room
// Custom group for $lights
// Dining room
// Backyard
// Office
// Downstairs
// Stairs
// Meli's Room

// ParseHexColor https://stackoverflow.com/questions/54197913/parse-hex-string-to-image-color
func ParseHexColor(s string) (c color.RGBA, err error) {
	c.A = 0xff

	if s[0] != '#' {
		return c, errInvalidColor
	}

	hexToByte := func(b byte) byte {
		switch {
		case b >= '0' && b <= '9':
			return b - '0'
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10
		}
		err = errInvalidColor
		return 0
	}

	switch len(s) {
	case 7:
		c.R = hexToByte(s[1])<<4 + hexToByte(s[2])
		c.G = hexToByte(s[3])<<4 + hexToByte(s[4])
		c.B = hexToByte(s[5])<<4 + hexToByte(s[6])
	case 4:
		c.R = hexToByte(s[1]) * 17
		c.G = hexToByte(s[2]) * 17
		c.B = hexToByte(s[3]) * 17
	default:
		err = errInvalidColor
	}
	return
}
