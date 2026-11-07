package telemetry

import (
	"context"
	"errors"
	"testing"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestDBQueryTracerRecordsDashboardAttributes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	histogram, err := provider.Meter("telemetry-test").Float64Histogram("db.client.operation.duration", metric.WithUnit("s"))
	require.NoError(t, err)
	tracer := &dbQueryTracer{Tracer: otelpgx.NewTracer(otelpgx.WithMeterProvider(metricnoop.NewMeterProvider())), duration: histogram}
	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "INSERT INTO trip_status_history (trip_id) VALUES ($1)"})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: errors.New("database failed")})
	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	var point metricdata.HistogramDataPoint[float64]
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "db.client.operation.duration" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok)
			require.Len(t, hist.DataPoints, 1)
			point = hist.DataPoints[0]
		}
	}
	for key, want := range map[string]string{"db.operation.name": "INSERT", "db.collection.name": "trip_status_history"} {
		value, ok := point.Attributes.Value(attribute.Key(key))
		require.True(t, ok)
		require.Equal(t, want, value.AsString())
	}
	errorType, ok := point.Attributes.Value(attribute.Key("error.type"))
	require.True(t, ok)
	require.NotEmpty(t, errorType.AsString())
}

func TestActiveTripsGaugeObservesProvider(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	SetActiveTripsProvider(func(context.Context) (int64, error) { return 7, nil })
	t.Cleanup(func() { SetActiveTripsProvider(nil) })
	require.NoError(t, registerActiveTripsGauge(provider.Meter("telemetry-test")))
	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "tripgo.trips.active" {
				gauge, ok := m.Data.(metricdata.Gauge[int64])
				require.True(t, ok)
				require.Len(t, gauge.DataPoints, 1)
				require.Equal(t, int64(7), gauge.DataPoints[0].Value)
				return
			}
		}
	}
	t.Fatal("active trips gauge was not exported")
}

func TestBuildVersionIsSet(t *testing.T) { require.NotEmpty(t, buildVersion()) }
