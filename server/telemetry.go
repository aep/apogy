package server

import (
	"context"
	"os"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

// InitTracer initializes the OpenTelemetry tracer
func init() {

	collectorEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	// Create OTLP exporter
	exporter, err := newExporter(collectorEndpoint)
	if err != nil {
		panic(err)
	}

	// Create resource with service information
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName("apogy-api"),
			attribute.String("environment", "production"),
		),
	)
	if err != nil {
		panic(err)
	}

	// Create trace provider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)

	// Set global propagator for extracting trace context from incoming requests
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Initialize our package tracer
	tracer = tp.Tracer("github.com/aep/apogy/server")

}

// newExporter creates an OTLP exporter
func newExporter(endpoint string) (sdktrace.SpanExporter, error) {
	if endpoint == "" {
		endpoint = "localhost:4317" // Default collector endpoint
	}

	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)

	return otlptrace.New(context.Background(), client)
}

// TracingMiddleware adds OpenTelemetry tracing to Echo requests
func TracingMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Extract existing context if present
		ctx := c.Request().Context()

		// Create a span for this request
		req := c.Request()
		spanName := req.Method + " " + c.Path()

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithAttributes(
				attribute.String("http.method", req.Method),
				attribute.String("http.url", req.URL.String()),
				attribute.String("http.path", c.Path()),
			),
		)
		defer span.End()

		// Update the request context
		c.SetRequest(req.WithContext(ctx))

		// Call the next handler
		err := next(c)

		// Record error and status code
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.SetAttributes(attribute.String("error.message", err.Error()))
		}

		span.SetAttributes(attribute.Int("http.status_code", c.Response().Status))

		return err
	}
}
