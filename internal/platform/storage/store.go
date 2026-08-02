package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

	"iot-perform/internal/platform/repository"
)

const onlineStatusTTL = 10 * time.Minute

type Config struct {
	DatabaseURL      string
	RedisAddr        string
	RedisPassword    string
	RedisDB          int
	TDengineURL      string
	TDengineUser     string
	TDenginePassword string
}

type Store struct {
	db        *sql.DB
	redis     *redis.Client
	telemetry *TDengine
}

func New(ctx context.Context, config Config) (*Store, error) {
	if config.DatabaseURL == "" || config.RedisAddr == "" || config.TDengineURL == "" {
		return nil, errors.New("database, Redis, and TDengine configuration are required")
	}
	db, err := sql.Open("pgx", config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: config.RedisAddr, Password: config.RedisPassword, DB: config.RedisDB})
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = db.Close()
		_ = rdb.Close()
		return nil, fmt.Errorf("ping Redis: %w", err)
	}
	t := NewTDengine(config.TDengineURL, config.TDengineUser, config.TDenginePassword)
	if err := t.EnsureSchema(ctx); err != nil {
		_ = db.Close()
		_ = rdb.Close()
		return nil, fmt.Errorf("initialize TDengine: %w", err)
	}
	if err := ensureUsersSchema(ctx, db); err != nil {
		_ = db.Close()
		_ = rdb.Close()
		return nil, fmt.Errorf("initialize PostgreSQL users schema: %w", err)
	}
	return &Store{db: db, redis: rdb, telemetry: t}, nil
}

func NewWithDependencies(db *sql.DB, rdb *redis.Client, td *TDengine) *Store {
	return &Store{db: db, redis: rdb, telemetry: td}
}

func (s *Store) Close() error {
	var errs []error
	if s.db != nil {
		errs = append(errs, s.db.Close())
	}
	if s.redis != nil {
		errs = append(errs, s.redis.Close())
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Repositories() repository.Repositories {
	return repository.Repositories{
		Users:     s,
		Products:  s,
		Devices:   s,
		Telemetry: s,
		Rules:     s,
		Alarms:    s,
		Commands:  s,
		Shadows:   s,
		OTA:       s,
	}
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func mapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", repository.ErrConflict, pgErr.ConstraintName)
		case "23503":
			return fmt.Errorf("%w: %s", repository.ErrNotFound, pgErr.ConstraintName)
		}
	}
	return err
}

func pageValues(page, pageSize int) (int, int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize, (page - 1) * pageSize
}

func setOnlineCache(ctx context.Context, rdb *redis.Client, deviceID string, online bool) error {
	if rdb == nil {
		return nil
	}
	key := "device:online:" + deviceID
	if online {
		return rdb.Set(ctx, key, "1", onlineStatusTTL).Err()
	}
	return rdb.Del(ctx, key).Err()
}

func setShadowCache(ctx context.Context, rdb *redis.Client, deviceID string, value []byte) error {
	if rdb == nil {
		return nil
	}
	return rdb.Set(ctx, "device:shadow:"+deviceID, value, 0).Err()
}

func getShadowCache(ctx context.Context, rdb *redis.Client, deviceID string) ([]byte, error) {
	if rdb == nil {
		return nil, redis.Nil
	}
	return rdb.Get(ctx, "device:shadow:"+deviceID).Bytes()
}
