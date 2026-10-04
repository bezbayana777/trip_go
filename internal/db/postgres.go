package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context) (*pgxpool.Pool, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DATABASE_URL: %w", err)
	}

	if maxConns := os.Getenv("DATABASE_MAX_CONNS"); maxConns != "" {
		if v, err := strconv.Atoi(maxConns); err == nil {
			config.MaxConns = int32(v)
		}
	}

	if minConns := os.Getenv("DATABASE_MIN_CONNS"); minConns != "" {
		if v, err := strconv.Atoi(minConns); err == nil {
			config.MinConns = int32(v)
		}
	}

	if maxLifetime := os.Getenv("DATABASE_MAX_CONN_LIFETIME"); maxLifetime != "" {
		if d, err := time.ParseDuration(maxLifetime); err == nil {
			config.MaxConnLifetime = d
		}
	}

	if connectTimeout := os.Getenv("DATABASE_CONNECT_TIMEOUT"); connectTimeout != "" {
		if d, err := time.ParseDuration(connectTimeout); err == nil {
			config.ConnConfig.ConnectTimeout = d
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5 * time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return pool, nil

}