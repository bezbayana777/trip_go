package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"

	"github.com/bezbayana777/trip_go/internal/db"
	httpDelivery "github.com/bezbayana777/trip_go/internal/delivery/http"
	"github.com/bezbayana777/trip_go/internal/gateway/push"
	generated "github.com/bezbayana777/trip_go/internal/generated"
	"github.com/bezbayana777/trip_go/internal/infra/postgres"
	"github.com/bezbayana777/trip_go/internal/infra/telemetry"
	"github.com/bezbayana777/trip_go/internal/repository"
	positionrepo "github.com/bezbayana777/trip_go/internal/repository/position"
	positionuc "github.com/bezbayana777/trip_go/internal/usecase/position"
	"github.com/bezbayana777/trip_go/internal/worker/positions"
)

func main() {
	ctx := context.Background()
	logger := telemetry.NewJSONLogger(required("LOG_LEVEL"))
	cleanupTelemetry, err := telemetry.Init(ctx, required("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		logger.Error("telemetry initialization failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	addr := required("HTTP_ADDR")
	shutdownTimeout := durationEnv("SHUTDOWN_TIMEOUT", 10*time.Second)
	checkInterval := durationEnv("POSITION_CHECK_INTERVAL", 30*time.Second)
	staleAfter := durationEnv("POSITION_STALE_AFTER", 60*time.Second)
	pushTimeout := durationEnv("PUSH_REQUEST_TIMEOUT", time.Second)
	workers := intEnv("POSITION_WORKERS", 4)
	queueSize := intEnv("POSITION_QUEUE_SIZE", 100)
	pushURL := required("PUSH_HTTP_URL")
	if workers < 1 || queueSize < 1 {
		logger.Error("POSITION_WORKERS and POSITION_QUEUE_SIZE must be positive")
		os.Exit(1)
	}

	poolDB, err := db.NewPool(ctx)
	if err != nil {
		logger.Error("database initialization failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	tripRepo := repository.NewTripRepository(poolDB)
	telemetry.SetActiveTripsProvider(tripRepo.CountActiveTrips)
	posRepo := positionrepo.New(poolDB)
	pushClient := push.New(pushURL, pushTimeout)
	m := telemetry.GlobalMetrics
	workerMetrics := positions.Metrics{StaleChecks: m.StaleChecks, QueueSize: m.PositionQueueSize, ActiveWorkers: m.PositionWorkersActive, PositionRequests: m.PositionRequests, RequestDuration: m.PositionRequestDuration}
	workPool, err := positions.New(ctx, workers, queueSize, pushTimeout, checkInterval, pushClient, logger, workerMetrics, otel.Tracer("trip-service/positions"))
	if err != nil {
		logger.Error("worker pool initialization failed", slog.String("error", err.Error()))
		poolDB.Close()
		os.Exit(1)
	}
	scanner, err := positions.NewScanner(posRepo, workPool, staleAfter, checkInterval, logger, nil)
	if err != nil {
		logger.Error("scanner initialization failed", slog.String("error", err.Error()))
		poolDB.Close()
		os.Exit(1)
	}
	scanCtx, stopScanner := context.WithCancel(context.Background())
	scannerDone := make(chan struct{})
	go func() { defer close(scannerDone); scanner.Run(scanCtx) }()
	txManager := postgres.NewTxManager(poolDB)
	apiServer := httpDelivery.NewServer(tripRepo, txManager, poolDB)
	apiServer.SetPositionUsecase(positionuc.New(posRepo, txManager, workPool, nil))
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)
	generated.HandlerFromMux(apiServer, router)
	server := &http.Server{Addr: addr, Handler: router, ReadTimeout: 5 * time.Second, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting", slog.String("addr", addr))
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case sig := <-signals:
		logger.Info("shutdown signal received", slog.String("signal", sig.String()))
	case err := <-serverErr:
		logger.Error("HTTP server failed", slog.String("error", err.Error()))
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			_ = server.Close()
		}
		stopScanner()
		<-scannerDone
		if stopErr := workPool.Stop(shutdownCtx); stopErr != nil {
			logger.Error("position workers shutdown failed", slog.String("error", stopErr.Error()))
		}
		pushClient.CloseIdleConnections()
		poolDB.Close()
		if telemetryErr := cleanupTelemetry(shutdownCtx); telemetryErr != nil {
			logger.Error("telemetry shutdown failed", slog.String("error", telemetryErr.Error()))
		}
		os.Exit(1)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP graceful shutdown timed out", slog.String("error", err.Error()))
		_ = server.Close()
	}
	stopScanner()
	<-scannerDone
	if err := workPool.Stop(shutdownCtx); err != nil {
		logger.Error("position workers shutdown failed", slog.String("error", err.Error()))
		workPool.Close()
	}
	pushClient.CloseIdleConnections()
	poolDB.Close()
	if err := cleanupTelemetry(shutdownCtx); err != nil {
		logger.Error("telemetry shutdown failed", slog.String("error", err.Error()))
	}
	logger.Info("service stopped")
}

func required(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		slog.Error("required environment variable is missing", slog.String("name", key))
		os.Exit(1)
	}
	return v
}

func durationEnv(key string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Error("invalid duration environment variable", slog.String("name", key), slog.String("value", raw))
		os.Exit(1)
	}
	return d
}

func intEnv(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		slog.Error("invalid integer environment variable", slog.String("name", key), slog.String("value", raw))
		os.Exit(1)
	}
	return v
}
