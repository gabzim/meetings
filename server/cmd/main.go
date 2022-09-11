package main

import (
	"go.uber.org/zap"
	"net/http"
	"os"

	"github.com/gabzim/meetings/server/postgres"
	"github.com/gabzim/meetings/server/services/auth"
	"github.com/gabzim/meetings/server/services/notifications"
	"github.com/markbates/goth/providers/google"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
)

func getEnvOrDefault(envName, fallback string) string {
	v := os.Getenv(envName)
	if v == "" {
		return fallback
	}
	return v
}

type ServerConfig struct {
	Port     string
	DbURL    string
	hostURL  string
	OauthCfg *oauth2.Config
}

func getServerConfig() *ServerConfig {
	port := getEnvOrDefault("MEETINGS_PORT", "8080")
	dbUrl := os.Getenv("MEETINGS_DB_URL")
	googleClientId := os.Getenv("MEETINGS_GOOGLE_KEY")
	googleClientSecret := os.Getenv("MEETINGS_GOOGLE_SECRET")
	hostUrl := os.Getenv("MEETINGS_HOST_URL")
	redirectUrl := hostUrl + "/auth/google/callback"
	cfg := &oauth2.Config{ClientID: googleClientId, ClientSecret: googleClientSecret, Endpoint: google.Endpoint, RedirectURL: redirectUrl, Scopes: []string{calendar.CalendarReadonlyScope}}
	return &ServerConfig{
		Port:     port,
		DbURL:    dbUrl,
		hostURL:  hostUrl,
		OauthCfg: cfg,
	}
}

func main() {
	cfg := getServerConfig()
	log, _ := zap.NewProduction()
	logger := log.Sugar()

	// init data layer
	db, err := postgres.CreateDB(cfg.DbURL)
	if err != nil {
		logger.Fatalf("error connecting to db: %v", err)
	}

	// init services
	tokenStore := auth.NewTokenStore(db)
	authServ := auth.NewService(logger, tokenStore)
	notifServ := notifications.NewService(logger, cfg.OauthCfg, cfg.hostURL)

	// init controllers
	authCtrl := auth.NewController(cfg.OauthCfg, authServ, cfg.OauthCfg.RedirectURL)
	notificationsCtrl := notifications.NewController(notifServ, authServ, logger)

	// init api
	http.HandleFunc("/auth/google", authCtrl.Redirect)
	http.HandleFunc("/auth/google/callback", authCtrl.Callback)
	http.HandleFunc("/notifications", notificationsCtrl.RegisterClient)
	http.HandleFunc("/push/", notificationsCtrl.ReceivePushFromGoogle)

	logger.Infof("Listening in %v...", cfg.Port)
	err = http.ListenAndServe(":"+cfg.Port, nil)
	logger.Fatalf("Error attaching to port %v: %v", cfg.Port, err)
}
