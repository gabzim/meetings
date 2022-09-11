package auth

import (
	"github.com/markbates/goth"
	"go.uber.org/zap"
)

type Service struct {
	logger *zap.SugaredLogger
	store  *TokenStore
}

func NewService(logger *zap.SugaredLogger, ts *TokenStore) *Service {
	l := logger.With("service", "AuthService")
	return &Service{store: ts, logger: l}
}

func (s *Service) RegisterUser(u *goth.User) (*UserToken, error) {
	t := &UserToken{
		AccessToken:  u.AccessToken,
		RefreshToken: u.RefreshToken,
		ExpiresAt:    &u.ExpiresAt,
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
	}
	t, err := s.store.UpsertToken(t)
	s.logger.Infow("User signed up", "email", t.Email)
	return t, err
}

// AuthenticateUser given a token will authenticate a user and return it, or an error
func (s *Service) AuthenticateUser(t string) (*UserToken, error) {
	tok, err := s.store.SelectToken(t)
	return tok, err
}
