package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	tripHttp "github.com/bezbayana777/trip_go/internal/http"
	createHandler "github.com/bezbayana777/trip_go/internal/http/trip/create"
	finishHandler "github.com/bezbayana777/trip_go/internal/http/trip/finish"
	getHandler "github.com/bezbayana777/trip_go/internal/http/trip/get"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
	"github.com/bezbayana777/trip_go/internal/infra/telemetry"
	repoTrip "github.com/bezbayana777/trip_go/internal/repository/trip"
	usecaseCreate "github.com/bezbayana777/trip_go/internal/usecase/trip/create"
	usecaseFinish "github.com/bezbayana777/trip_go/internal/usecase/trip/finish"
	usecaseGet "github.com/bezbayana777/trip_go/internal/usecase/trip/get"
)

func envDuration(key string, fallback time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return fallback
}

func main() {
	ctx := context.Background()
	logger := telemetry.NewJSONLogger(os.Getenv("LOG_LEVEL"))
	slog.SetDefault(logger)
	collector := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if collector == "" {
		collector = "http://localhost:4318"
	}
	cleanup, err := telemetry.Init(ctx, collector)
	if err != nil {
		logger.Error("failed to initialize telemetry", slog.String("error", err.Error()))
	} else {
		defer func() {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = cleanup(c)
		}()
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		logger.Error("invalid DATABASE_URL", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if v, e := strconv.ParseInt(os.Getenv("DATABASE_MAX_CONNS"), 10, 32); e == nil && v > 0 {
		config.MaxConns = int32(v)
	}
	if v, e := strconv.ParseInt(os.Getenv("DATABASE_MIN_CONNS"), 10, 32); e == nil && v >= 0 {
		config.MinConns = int32(v)
	}
	if d, e := time.ParseDuration(os.Getenv("DATABASE_MAX_CONN_LIFETIME")); e == nil && d > 0 {
		config.MaxConnLifetime = d
	}
	config.ConnConfig.Tracer = telemetry.NewDBTracer()
	if d := envDuration("DATABASE_QUERY_TIMEOUT", 3*time.Second); d > 0 {
		config.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(d.Milliseconds(), 10)
	}
	connectCtx, cancel := context.WithTimeout(ctx, envDuration("DATABASE_CONNECT_TIMEOUT", 5*time.Second))
	defer cancel()
	pool, err := pgxpool.NewWithConfig(connectCtx, config)
	if err != nil {
		logger.Error("failed to create database pool", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()
	if err = pool.Ping(connectCtx); err != nil {
		logger.Error("database unreachable", slog.String("error", err.Error()))
		os.Exit(1)
	}
	txManager := postgres.NewTxManager(pool)
	repository := repoTrip.New(pool)
	telemetry.SetActiveTripsProvider(repository.CountActiveTrips)
	createUC := usecaseCreate.New(repository, txManager, logger, telemetry.GlobalMetrics.TripsCreated)
	finishUC := usecaseFinish.New(repository, txManager, logger, telemetry.GlobalMetrics.TripsCompleted)
	getUC := usecaseGet.New(repository)
	handler := tripHttp.NewRouter(tripHttp.RouterDeps{CreateHandler: createHandler.New(createUC), GetHandler: getHandler.New(getUC), FinishHandler: finishHandler.New(finishUC), DB: pool, Logger: logger})
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{Addr: addr, Handler: handler, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		logger.Info("http server starting", slog.String("addr", addr))
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			logger.Error("http server failed", slog.String("error", e.Error()))
			os.Exit(1)
		}
	}()
	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), envDuration("SHUTDOWN_TIMEOUT", 10*time.Second))
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
	}
}
