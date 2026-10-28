package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"log/slog"
  "github.com/bezbayana777/trip_go/internal/infra/telemetry"

	"github.com/jackc/pgx/v5/pgxpool"

	tripHttp "github.com/bezbayana777/trip_go/internal/http"
	createHandler "github.com/bezbayana777/trip_go/internal/http/trip/create"
	finishHandler "github.com/bezbayana777/trip_go/internal/http/trip/finish"
	getHandler "github.com/bezbayana777/trip_go/internal/http/trip/get"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
	repoTrip "github.com/bezbayana777/trip_go/internal/repository/trip"
	usecaseCreate "github.com/bezbayana777/trip_go/internal/usecase/trip/create"
	usecaseFinish "github.com/bezbayana777/trip_go/internal/usecase/trip/finish"
	usecaseGet "github.com/bezbayana777/trip_go/internal/usecase/trip/get"
)

func main() {
	ctx := context.Background()

	logger := telemetry.NewJSONLogger()
	slog.SetDefault(logger)

	collectorURL := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if collectorURL == "" {
		collectorURL = "localhost:4317" 
	}

	cleanup, err := telemetry.Init(ctx, collectorURL)
	if err != nil {
		logger.Error("failed to init telemetry", slog.String("error", err.Error()))
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = cleanup(shutdownCtx)
	}()

	logger.Info("service started", slog.String("service", "trip-service"))
	defer logger.Info("service stopped", slog.String("service", "trip-service"))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/trip_db?sslmode=disable"
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		log.Fatalf("Failed to parse database config: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("Database unreachable: %v", err)
	}
	log.Println("Successfully connected to PostgreSQL")

	txManager := postgres.NewTxManager(pool)
	tripRepo := repoTrip.New(pool)

	createUC := usecaseCreate.New(tripRepo, txManager)
	getUC := usecaseGet.New(tripRepo)
	finishUC := usecaseFinish.New(tripRepo, txManager)

	routerDeps := tripHttp.RouterDeps{
		CreateHandler: createHandler.New(createUC),
		GetHandler:    getHandler.New(getUC),
		FinishHandler: finishHandler.New(finishUC),
	}

	handler := tripHttp.NewRouter(routerDeps)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting HTTP server on port %s...", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	sig := <-shutdownChan
	log.Printf("Received shutdown signal (%v), gracefully shutting down...", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server forced to shutdown: %v", err)
	}

	log.Println("Server stopped successfully.")
}