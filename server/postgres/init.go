package postgres

import (
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var schema = `
CREATE TABLE IF NOT EXISTS user_tokens(
    id SERIAL PRIMARY KEY,
    access_token CHAR(255) NOT NULL,
    refresh_token CHAR(255) NOT NULL,
    meetings_token CHAR(256) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    first_name VARCHAR(50),
    last_name VARCHAR(50),
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
)`

func CreateDB(url string) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", url)
	if err != nil {
		return nil, err
	}
	db.MustExec(schema)
	return db, err
}
