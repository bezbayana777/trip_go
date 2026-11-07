package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/bezbayana777/trip_go/internal/http/domainerror"
	createHandler "github.com/bezbayana777/trip_go/internal/http/trip/create"
	finishHandler "github.com/bezbayana777/trip_go/internal/http/trip/finish"
	getHandler "github.com/bezbayana777/trip_go/internal/http/trip/get"
	"github.com/bezbayana777/trip_go/internal/infra/telemetry"
)

type (
	Pinger     interface{ Ping(context.Context) error }
	RouterDeps struct {
		CreateHandler *createHandler.Handler
		GetHandler    *getHandler.Handler
		FinishHandler *finishHandler.Handler
		DB            Pinger
		Logger        *slog.Logger
	}
)

func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(tracing)
	r.Use(requestLogging(deps.Logger))
	r.Use(requestMetrics)
	r.Use(recoverProblem(deps.Logger))
	r.Route("/api/v1", func(r chi.Router) {
		r.Route("/trips", func(r chi.Router) {
			r.Post("/", deps.CreateHandler.ServeHTTP)
			r.Get("/{tripId}", deps.GetHandler.ServeHTTP)
			r.Post("/{tripId}/finish", deps.FinishHandler.ServeHTTP)
		})
	})
	r.Get("/health", healthHandler)
	r.Get("/ready", readinessHandler(deps.DB))
	return r
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeHealthResponse(w, http.StatusOK, "ok")
}

func readinessHandler(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if db == nil {
			writeHealthResponse(w, http.StatusServiceUnavailable, "unavailable")
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
		defer cancel()
		if db.Ping(ctx) != nil {
			writeHealthResponse(w, http.StatusServiceUnavailable, "unavailable")
			return
		}

		writeHealthResponse(w, http.StatusOK, "ok")
	}
}

func writeHealthResponse(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}

func routeName(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	if route := routeFromPath(r.Method, r.URL.Path); route != "" {
		return route
	}
	return "<unmatched>"
}

func routeFromPath(method, path string) string {
	switch path {
	case "/health":
		if method == http.MethodGet {
			return "/health"
		}
	case "/ready":
		if method == http.MethodGet {
			return "/ready"
		}
	case "/api/v1/trips/":
		if method == http.MethodPost {
			return "/api/v1/trips/"
		}
	}
	const prefix = "/api/v1/trips/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) == 1 && parts[0] != "" {
		if method == http.MethodGet {
			return "/api/v1/trips/{tripId}"
		}
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "finish" && method == http.MethodPost {
		return "/api/v1/trips/{tripId}/finish"
	}
	return ""
}

func tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := telemetry.Tracer().Start(r.Context(), r.Method+" "+r.URL.Path, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		rw := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			span.SetName(r.Method + " " + routeName(r))
			span.SetAttributes(attribute.Int("http.response.status_code", rw.Status()), attribute.String("http.route", routeName(r)))
			if rw.Status() >= 500 {
				span.SetStatus(codes.Error, "HTTP server error")
			}
		}()
		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

func requestLogging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				code := rw.Status()
				attrs := []any{slog.String("method", r.Method), slog.String("route", routeName(r)), slog.Int("status", code), slog.Duration("duration", time.Since(start))}
				sc := trace.SpanContextFromContext(r.Context())
				if sc.IsValid() {
					attrs = append(attrs, slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
				}
				if logger != nil {
					if code >= 500 {
						logger.ErrorContext(r.Context(), "http request", attrs...)
					} else {
						logger.InfoContext(r.Context(), "http request", attrs...)
					}
				}
			}()
			next.ServeHTTP(rw, r)
		})
	}
}

func requestMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rw := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		route := routeName(r)
		attrs := metric.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("http.route", route))
		if telemetry.GlobalMetrics.HttpActiveReq != nil {
			telemetry.GlobalMetrics.HttpActiveReq.Add(r.Context(), 1, attrs)
		}
		defer func() {
			if telemetry.GlobalMetrics.HttpActiveReq != nil {
				telemetry.GlobalMetrics.HttpActiveReq.Add(r.Context(), -1, attrs)
			}
			if telemetry.GlobalMetrics.HttpDuration != nil {
				telemetry.GlobalMetrics.HttpDuration.Record(r.Context(), time.Since(started).Seconds(), metric.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("http.route", route), attribute.Int("http.response.status_code", rw.Status())))
			}
			if rw.Status() >= 500 {
				trace.SpanFromContext(r.Context()).SetStatus(codes.Error, "HTTP server error")
			}
		}()
		next.ServeHTTP(rw, r)
	})
}

func recoverProblem(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if logger != nil {
						logger.ErrorContext(r.Context(), "panic recovered", slog.Any("panic", recovered), slog.String("stack", string(debug.Stack())))
					}
					p := domainerror.ProblemForInternal()
					p.Instance = stringPtr(r.URL.Path)
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(p)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
func stringPtr(s string) *string { return &s }
