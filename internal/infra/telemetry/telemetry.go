package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	traceapi "go.opentelemetry.io/otel/trace"
)

type Metrics struct {
	HttpDuration   metric.Float64Histogram
	HttpActiveReq  metric.Int64UpDownCounter
	DbDuration     metric.Float64Histogram
	TripsCreated   metric.Int64Counter
	TripsCompleted metric.Int64Counter
}

var GlobalMetrics Metrics

var (
	activeTripsProvider func(ctx context.Context) (int64, error)
	activeTripsMu       sync.RWMutex
)

func SetActiveTripsProvider(provider func(ctx context.Context) (int64, error)) {
	activeTripsMu.Lock()
	defer activeTripsMu.Unlock()
	activeTripsProvider = provider
}

func Init(ctx context.Context, collectorURL string) (func(context.Context) error, error) {
	cleanURL := strings.TrimPrefix(collectorURL, "http://")
	cleanURL = strings.TrimPrefix(cleanURL, "https://")

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("trip-service"),
			semconv.ServiceVersionKey.String(buildVersion()),
			semconv.ServiceNamespaceKey.String("tripgo"),
			semconv.DeploymentEnvironmentKey.String("local"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(cleanURL), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)
	otel.SetTracerProvider(tp)

	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpoint(cleanURL), otlpmetrichttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
	)
	otel.SetMeterProvider(mp)

	meter := otel.Meter("trip-service")

	if GlobalMetrics.HttpDuration, err = meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s")); err != nil {
		return nil, fmt.Errorf("failed to create http.server.request.duration: %w", err)
	}
	if GlobalMetrics.HttpActiveReq, err = meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}")); err != nil {
		return nil, fmt.Errorf("failed to create http.server.active_requests: %w", err)
	}
	if GlobalMetrics.DbDuration, err = meter.Float64Histogram("db.client.operation.duration", metric.WithUnit("s")); err != nil {
		return nil, fmt.Errorf("failed to create db.client.operation.duration: %w", err)
	}
	if GlobalMetrics.TripsCreated, err = meter.Int64Counter("tripgo.trips.created", metric.WithUnit("{trip}")); err != nil {
		return nil, fmt.Errorf("failed to create tripgo.trips.created: %w", err)
	}
	if GlobalMetrics.TripsCompleted, err = meter.Int64Counter("tripgo.trips.completed", metric.WithUnit("{trip}")); err != nil {
		return nil, fmt.Errorf("failed to create tripgo.trips.completed: %w", err)
	}

	if err := registerActiveTripsGauge(meter); err != nil {
		return nil, err
	}

	cleanup := func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		return mp.Shutdown(ctx)
	}

	return cleanup, nil
}

func registerActiveTripsGauge(meter metric.Meter) error {
	_, err := meter.Int64ObservableGauge("tripgo.trips.active", metric.WithUnit("{trip}"), metric.WithInt64Callback(func(ctx context.Context, observer metric.Int64Observer) error {
		activeTripsMu.RLock()
		provider := activeTripsProvider
		activeTripsMu.RUnlock()
		if provider == nil {
			return errors.New("active trips provider is not configured")
		}
		value, err := provider(ctx)
		if err != nil {
			return err
		}
		observer.Observe(value)
		return nil
	}))
	if err != nil {
		return fmt.Errorf("failed to create tripgo.trips.active gauge: %w", err)
	}
	return nil
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			return setting.Value
		}
	}
	return "dev"
}

type (
	queryTelemetryContextKey struct{}
	queryTelemetryData       struct {
		startedAt  time.Time
		operation  string
		collection string
	}
)

type dbQueryTracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}

// NewDBTracer retains otelpgx spans and records the database histogram with the
// exact attributes used by the service dashboards.
func NewDBTracer() pgx.QueryTracer {
	base := otelpgx.NewTracer(otelpgx.WithMeterProvider(metricnoop.NewMeterProvider()))
	return &dbQueryTracer{Tracer: base, duration: GlobalMetrics.DbDuration}
}

func (t *dbQueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	return context.WithValue(ctx, queryTelemetryContextKey{}, queryTelemetryData{startedAt: time.Now(), operation: sqlOperation(data.SQL), collection: sqlCollection(data.SQL)})
}

func (t *dbQueryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	t.Tracer.TraceQueryEnd(ctx, conn, data)
	query, ok := ctx.Value(queryTelemetryContextKey{}).(queryTelemetryData)
	if !ok || t.duration == nil {
		return
	}
	errorType := ""
	if data.Err != nil && !errors.Is(data.Err, pgx.ErrNoRows) {
		errorType = fmt.Sprintf("%T", data.Err)
	}
	t.duration.Record(ctx, time.Since(query.startedAt).Seconds(), metric.WithAttributes(
		attribute.String("db.operation.name", query.operation),
		attribute.String("db.collection.name", query.collection),
		attribute.String("error.type", errorType),
	))
}

func sqlOperation(sql string) string {
	fields := strings.Fields(sql)
	if len(fields) == 0 {
		return "UNKNOWN"
	}
	return strings.ToUpper(strings.Trim(fields[0], ";"))
}

func sqlCollection(sql string) string {
	fields := strings.Fields(strings.ReplaceAll(sql, "\n", " "))
	for i, field := range fields {
		switch strings.ToUpper(strings.Trim(field, "`\";")) {
		case "FROM", "INTO", "UPDATE", "JOIN":
			if i+1 < len(fields) {
				collection := strings.Trim(fields[i+1], "`\";,()")
				if dot := strings.LastIndexByte(collection, '.'); dot >= 0 {
					collection = collection[dot+1:]
				}
				if collection != "" {
					return collection
				}
			}
		}
	}
	return "unknown"
}

func Tracer() traceapi.Tracer {
	return otel.Tracer("trip-service")
}

func NewJSONLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	return slog.New(handler)
}

func LogWithContext(ctx context.Context) []any {
	spanCtx := traceapi.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}
	return []any{
		slog.String("trace_id", spanCtx.TraceID().String()),
		slog.String("span_id", spanCtx.SpanID().String()),
	}
}
