package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	telemetry "github.com/bezbayana777/trip_go/internal/infra/telemetry"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	createHandler "github.com/bezbayana777/trip_go/internal/http/trip/create"
	finishHandler "github.com/bezbayana777/trip_go/internal/http/trip/finish"
	getHandler "github.com/bezbayana777/trip_go/internal/http/trip/get"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type RouterDeps struct {
	CreateHandler *createHandler.Handler
	GetHandler    *getHandler.Handler
	FinishHandler *finishHandler.Handler
	DB            Pinger 
}

func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx := r.Context()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			
			ctx, span := telemetry.Tracer().Start(ctx, r.Method+" "+r.URL.Path,
				trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()

			r = r.WithContext(ctx)
			next.ServeHTTP(ww, r)

			routePattern := chi.RouteContext(r.Context()).RoutePattern()
			if routePattern == "" {
				routePattern = r.URL.Path
			}

			span.SetName(r.Method + " " + routePattern)

			duration := time.Since(start).Seconds()

			telemetry.GlobalMetrics.HttpDuration.Record(ctx, duration,
				metric.WithAttributes(
					attribute.String("http.request.method", r.Method),
					attribute.String("http.route", routePattern),
					attribute.Int("http.response.status_code", ww.Status()),
				),
			)

			span.SetAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", routePattern),
				attribute.Int("http.status_code", ww.Status()),
			)

			if ww.Status() >= 500 {
				span.SetStatus(codes.Error, "Server Error")
			}
		})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/trips", func(r chi.Router) {
			r.Post("/", deps.CreateHandler.ServeHTTP)
			r.Get("/{id}", deps.GetHandler.ServeHTTP)
			r.Post("/{id}/finish", deps.FinishHandler.ServeHTTP)
		})
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if deps.DB == nil {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
			return
		}

		pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := deps.DB.Ping(pingCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "database unavailable"})
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	return r
}