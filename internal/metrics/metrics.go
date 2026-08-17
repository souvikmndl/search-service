package metrics

import (
	"context"
	"time"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Meter defines the interface for recording metrics.
// This abstraction makes handlers and middlewares unit-testable and decoupled from OTel SDK.
type Meter interface {
	RecordHTTPRequest(ctx context.Context, method, path string, status int, duration time.Duration)
}

// otelMeter implements Meter using OpenTelemetry.
type otelMeter struct {
	requestCounter  metric.Int64Counter
	requestDuration metric.Float64Histogram
}

// NewOTelMeter creates a new OTel-backed Meter.
func NewOTelMeter(provider metric.MeterProvider) (Meter, error) {
	meter := provider.Meter("search-service")

	counter, err := meter.Int64Counter("http_requests_total",
		metric.WithDescription("Total number of HTTP requests processed"),
	)
	if err != nil {
		return nil, err
	}

	duration, err := meter.Float64Histogram("http_request_duration_seconds",
		metric.WithDescription("Latency of HTTP requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	return &otelMeter{
		requestCounter:  counter,
		requestDuration: duration,
	}, nil
}

func (m *otelMeter) RecordHTTPRequest(ctx context.Context, method, path string, status int, duration time.Duration) {
	attrs := []attribute.KeyValue{
		attribute.String("http.method", method),
		attribute.String("http.route", path),
		attribute.Int("http.status_code", status),
	}

	m.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// InitMetrics starts the Prometheus metrics exporter and registers the /metrics endpoint on Echo.
func InitMetrics(e *echo.Echo) (Meter, error) {
	// Initialize the Prometheus exporter
	exporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))

	meter, err := NewOTelMeter(provider)
	if err != nil {
		return nil, err
	}

	// Expose Prometheus /metrics endpoint
	e.GET("/metrics", echo.WrapHandler(exporter))

	return meter, nil
}
