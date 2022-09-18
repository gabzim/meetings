package auth

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/dchest/uniuri"
	"github.com/jmoiron/sqlx"
	"golang.org/x/oauth2"
)

type UserToken struct {
	Id            int64      `db:"id"`
	AccessToken   string     `db:"access_token"`
	RefreshToken  string     `db:"refresh_token"`
	MeetingsToken string     `db:"meetings_token"`
	Email         string     `db:"email"`
	FirstName     string     `db:"first_name"`
	LastName      string     `db:"last_name"`
	ExpiresAt     *time.Time `db:"expires_at"`
	CreatedAt     *time.Time `db:"created_at"`
	UpdatedAt     *time.Time `db:"updated_at"`
}

func (t *UserToken) GenerateAuthToken() {
	t.MeetingsToken = uniuri.NewLen(64)
}

// GetOauthToken description
func (t *UserToken) GetOauthToken() *oauth2.Token {
	return &oauth2.Token{AccessToken: t.AccessToken, RefreshToken: t.RefreshToken, Expiry: *t.ExpiresAt}
}

type TokenStore struct {
	db *sqlx.DB
}

func NewTokenStore(db *sqlx.DB) *TokenStore {
	return &TokenStore{db}
}

var ErrUserNotFound = errors.New("USER_NOT_FOUND")

func (s *TokenStore) SelectToken(authToken string) (*UserToken, error) {
	user := UserToken{}
	err := s.db.Get(&user, "SELECT id, access_token, refresh_token, email, meetings_token, first_name, last_name, expires_at, created_at, updated_at from user_tokens where meetings_token = $1", authToken)
	if errors.Is(err, sql.ErrNoRows) {
		return &user, ErrUserNotFound
	}
	return &user, err
}

func (s *TokenStore) SelectByEmail(email string) ([]*UserToken, error) {
	email = strings.ToLower(strings.Trim(email, " "))
	tokens := make([]*UserToken, 0)
	err := s.db.Select(&tokens, "SELECT id, access_token, refresh_token, email, meetings_token, first_name, last_name, expires_at, created_at, updated_at from user_tokens where email = $1", email)
	if errors.Is(err, sql.ErrNoRows) {
		return tokens, ErrUserNotFound
	}
	// Todo wrap error
	return tokens, err
}

func (s *TokenStore) UpsertToken(t *UserToken) (*UserToken, error) {
	now := time.Now()
	if t.MeetingsToken == "" {
		t.GenerateAuthToken()
	}
	if t.CreatedAt == nil {
		t.CreatedAt = &now
	}
	t.UpdatedAt = &now
	_, error := s.db.Exec("INSERT INTO user_tokens (access_token, refresh_token, email, meetings_token, first_name, last_name, expires_at, created_at, updated_at) values ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (email) DO UPDATE SET access_token = $1, expires_at = $7, updated_at = $9, meetings_token = $4 RETURNING *", t.AccessToken, t.RefreshToken, t.Email, t.MeetingsToken, t.FirstName, t.LastName, t.ExpiresAt, t.CreatedAt, t.UpdatedAt)
	// todo wrap error
	return t, error
}
