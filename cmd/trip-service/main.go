package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bezbayana777/trip_go/internal/db"
	"github.com/bezbayana777/trip_go/internal/repository"
	generated "github.com/bezbayana777/trip_go/internal/generated"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpDelivery "github.com/bezbayana777/trip_go/internal/delivery/http"
)


func main() {

	ctx := context.Background()

	httpAddr := getFromEnv("HTTP_ADDR", ":8080")
	shutdownTimeoutString := getFromEnv("SHUTDOWN_TIMEOUT", "10s")	
	shutdownTimeout, err := time.ParseDuration(shutdownTimeoutString)
	if err != nil {
		log.Fatalf("Invalid SHUTDOWN_TIMEOUT value: %v", err)
	}

	dbPool, err := db.NewPool(ctx)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbPool.Close()
	log.Println("Successfully connected to PostgreSQL")

	tripRepo := repository.NewTripRepository(dbPool)
	txManager := repository.NewTxManager(dbPool)

	apiServer := httpDelivery.NewServer(tripRepo, txManager, dbPool)


	router := chi.NewRouter()
	router.Use(middleware.Recoverer)

	generated.HandlerFromMux(apiServer, router)
	
	server := &http.Server{
		Addr:    httpAddr,
		Handler: router,
		ReadTimeout:  5 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting server on %s", httpAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on %s: %v\n", httpAddr, err)
		}
	}()
	
	sig := <-stop
	log.Printf("Shutdown signal received: %v", sig)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("HTTP server stopped gracefully")
}


func getFromEnv(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	
	if exists {
		return value
	}
	
	return defaultValue
}