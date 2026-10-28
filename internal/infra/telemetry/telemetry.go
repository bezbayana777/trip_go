package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
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

var activeTripsProvider func(ctx context.Context) (int64, error)

func SetActiveTripsProvider(provider func(ctx context.Context) (int64, error)) {
	activeTripsProvider = provider
}

func Init(ctx context.Context, collectorURL string) (func(context.Context) error, error) {
	cleanURL := strings.TrimPrefix(collectorURL, "http://")
	cleanURL = strings.TrimPrefix(cleanURL, "https://")

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("trip-service"),
			semconv.ServiceVersionKey.String("1.0.0"),
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
	if GlobalMetrics.HttpActiveReq, err = meter.Int64UpDownCounter("http.server.active_requests"); err != nil {
		return nil, fmt.Errorf("failed to create http.server.active_requests: %w", err)
	}
	if GlobalMetrics.DbDuration, err = meter.Float64Histogram("db.client.operation.duration", metric.WithUnit("s")); err != nil {
		return nil, fmt.Errorf("failed to create db.client.operation.duration: %w", err)
	}
	if GlobalMetrics.TripsCreated, err = meter.Int64Counter("tripgo.trips.created"); err != nil {
		return nil, fmt.Errorf("failed to create tripgo.trips.created: %w", err)
	}
	if GlobalMetrics.TripsCompleted, err = meter.Int64Counter("tripgo.trips.completed"); err != nil {
		return nil, fmt.Errorf("failed to create tripgo.trips.completed: %w", err)
	}

	// Gauge для tripgo.trips.active
	_, err = meter.Int64ObservableGauge("tripgo.trips.active",
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			if activeTripsProvider != nil {
				val, err := activeTripsProvider(ctx)
				if err == nil {
					o.Observe(val)
				}
			}
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tripgo.trips.active gauge: %w", err)
	}

	cleanup := func(ctx context.Context) error {
		if err := tp.Shutdown(ctx); err != nil {
			return err
		}
		return mp.Shutdown(ctx)
	}

	return cleanup, nil
}

func Tracer() traceapi.Tracer {
	return otel.Tracer("trip-service")
}

func NewJSONLogger() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
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