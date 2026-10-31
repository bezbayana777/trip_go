package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var queryTimeout time.Duration

func WithQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if queryTimeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, queryTimeout)
}

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
		v, err := strconv.Atoi(maxConns)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("invalid DATABASE_MAX_CONNS %q: must be a positive integer", maxConns)
		}
		config.MaxConns = int32(v)
	}

	if minConns := os.Getenv("DATABASE_MIN_CONNS"); minConns != "" {
		v, err := strconv.Atoi(minConns)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("invalid DATABASE_MIN_CONNS %q: must be a non-negative integer", minConns)
		}
		config.MinConns = int32(v)
	}

	if config.MinConns > config.MaxConns {
		return nil, fmt.Errorf("DATABASE_MIN_CONNS (%d) must not exceed DATABASE_MAX_CONNS (%d)", config.MinConns, config.MaxConns)
	}
	
	if maxLifetime := os.Getenv("DATABASE_MAX_CONN_LIFETIME"); maxLifetime != "" {
		d, err := time.ParseDuration(maxLifetime)
		if err != nil {
			return nil, fmt.Errorf("invalid DATABASE_MAX_CONN_LIFETIME %q: %w", maxLifetime, err)
		}
		config.MaxConnLifetime = d
	}

	if connectTimeout := os.Getenv("DATABASE_CONNECT_TIMEOUT"); connectTimeout != "" {
		d, err := time.ParseDuration(connectTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid DATABASE_CONNECT_TIMEOUT %q: %w", connectTimeout, err)
		}
		config.ConnConfig.ConnectTimeout = d
	}

	if qTimeout := os.Getenv("DATABASE_QUERY_TIMEOUT"); qTimeout != "" {
		d, err := time.ParseDuration(qTimeout)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("invalid DATABASE_QUERY_TIMEOUT %q: must be a positive duration", qTimeout)
		}
		queryTimeout = d
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgxpool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return pool, nil
}