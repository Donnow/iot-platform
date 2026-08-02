package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"iot-perform/internal/platform/domain"
)

// ensureUsersSchema is idempotent and runs at startup so deployments whose
// PostgreSQL volume predates the users table (docker-entrypoint-initdb.d
// only runs on first boot) still get the schema.
func ensureUsersSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			username VARCHAR(64) UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role VARCHAR(32) NOT NULL DEFAULT 'admin',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (domain.User, error) {
	if err := contextErr(ctx); err != nil {
		return domain.User{}, err
	}
	var user domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT id::text, username, password_hash, role, created_at
		FROM users WHERE username = $1`, username).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &user.CreatedAt)
	if err != nil {
		return domain.User{}, mapDBError(err)
	}
	return user, nil
}

func (s *Store) CreateUser(ctx context.Context, user domain.User) (domain.User, error) {
	if err := contextErr(ctx); err != nil {
		return domain.User{}, err
	}
	if user.Username == "" || user.PasswordHash == "" {
		return domain.User{}, errors.New("username and password hash are required")
	}
	if user.Role == "" {
		user.Role = "admin"
	}
	var createdAt time.Time
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id::text, created_at`, user.Username, user.PasswordHash, user.Role).
		Scan(&user.ID, &createdAt)
	if err != nil {
		return domain.User{}, mapDBError(err)
	}
	user.CreatedAt = createdAt
	return user, nil
}
