package auth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/google"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/plus/v1"
)

func NewController(cfg *oauth2.Config, authServ *Service, redirectUrl string) *GoogleAuthController {
	gProvider := google.New(cfg.ClientID, cfg.ClientSecret, redirectUrl, calendar.CalendarReadonlyScope, plus.UserinfoEmailScope, plus.UserinfoProfileScope)
	gProvider.SetPrompt("consent")

	goth.UseProviders(gProvider)
	return &GoogleAuthController{authServ: authServ, google: gProvider}
}

type GoogleAuthController struct {
	cfg      *oauth2.Config
	authServ *Service
	google   goth.Provider
}

func (c *GoogleAuthController) Redirect(w http.ResponseWriter, r *http.Request) {
	req := r.WithContext(context.WithValue(r.Context(), "provider", "google"))
	gothic.BeginAuthHandler(w, req)
}

func (c *GoogleAuthController) Callback(w http.ResponseWriter, r *http.Request) {
	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		return
	}

	t, err := c.authServ.RegisterUser(&user)
	if err != nil {
		fmt.Fprint(w, err)
		return
	}
	fmt.Fprint(w, t.MeetingsToken)
}
