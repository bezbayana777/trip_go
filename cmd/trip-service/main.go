package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bezbayana777/trip_go/internal/db"
	generated "github.com/bezbayana777/trip_go/internal/generated"
	"github.com/bezbayana777/trip_go/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	httpDelivery "github.com/bezbayana777/trip_go/internal/delivery/http"
)

func main() {
	httpAddr := requiredEnv("HTTP_ADDR")
	shutdownTimeout := requiredDuration("SHUTDOWN_TIMEOUT", true)
	logLevel := strings.ToLower(requiredEnv("LOG_LEVEL"))
	if logLevel != "debug" && logLevel != "info" && logLevel != "warn" && logLevel != "error" {
		log.Fatalf("invalid LOG_LEVEL %q: expected debug, info, warn, or error", logLevel)
	}

	ctx := context.Background()
	dbPool, err := db.NewPool(ctx)
	if err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	defer dbPool.Close()

	tripRepo := repository.NewTripRepository(dbPool)
	txManager := repository.NewTxManager(dbPool)
	apiServer := httpDelivery.NewServer(tripRepo, txManager, dbPool)
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)
	generated.HandlerWithOptions(apiServer, generated.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			instance := r.URL.Path
			detail := "Invalid path or parameter"
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(generated.Problem{
				Type: "https://tripgo.example/problems/invalid-request", Title: "Invalid request",
				Status: http.StatusBadRequest, Detail: &detail, Instance: &instance, Code: "invalid_request",
			})
		},
	})

	server := &http.Server{
		Addr: httpAddr, Handler: router,
		ReadTimeout: 5 * time.Second, ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("starting HTTP server on %s (log level: %s)", httpAddr, logLevel)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	select {
	case sig := <-stop:
		log.Printf("shutdown signal received: %v", sig)
	case err := <-serverErr:
		if err != nil {
			log.Fatalf("HTTP server failed: %v", err)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	log.Print("HTTP server stopped gracefully")
}

func requiredEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		log.Fatalf("%s is required", key)
	}
	return value
}

func requiredDuration(key string, positive bool) time.Duration {
	raw := requiredEnv(key)
	d, err := time.ParseDuration(raw)
	if err != nil || (positive && d <= 0) || (!positive && d < 0) {
		log.Fatalf("invalid %s %q: must be a valid positive duration", key, raw)
	}
	return d
}
