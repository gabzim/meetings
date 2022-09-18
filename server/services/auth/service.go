package auth

import (
	"errors"
	"github.com/markbates/goth"
	"go.uber.org/zap"
)

var (
	ErrTokenInvalid = errors.New("TOKEN_INVALID_FOR_USER")
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
func (s *Service) AuthenticateUser(email string, token string) (*UserToken, error) {
	tokens, err := s.store.SelectByEmail(email)
	if err != nil {
		return nil, err
	}
	for _, t := range tokens {
		if t.MeetingsToken == token {
			return t, err
		}
	}
	return nil, ErrTokenInvalid
}
