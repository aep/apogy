package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Global registry so it can be accessed from middleware
var promRegistry *prometheus.Registry

// HTTP request metrics
var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	kvCommitDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kv_commit_duration_seconds",
			Help:    "Duration of KV commit operations",
			Buckets: []float64{0.00001, 0.0001, 0.001, 0.01, 0.1, 0.2, 0.5, 1, 1.5, 2},
		},
		[]string{"operation"},
	)

	kvLockRetries = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kv_lock_retries",
			Help:    "Number of commit retries",
			Buckets: []float64{0, 1, 2, 3, 4, 5, 10, 20, 40, 80, 160, 320},
		},
		[]string{"operation", "status"},
	)

	kvCommitFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kv_commit_failures_total",
			Help: "Total number of failed KV commit operations",
		},
		[]string{"operation", "error_type"},
	)
)

// Initialize Prometheus metrics
func init() {
	// Create a new registry
	promRegistry = prometheus.NewRegistry()

	// Register the standard process metrics
	promRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Register Go runtime metrics
	promRegistry.MustRegister(collectors.NewGoCollector())

	// Register our custom metrics
	promRegistry.MustRegister(httpRequestsTotal)
	promRegistry.MustRegister(httpRequestDuration)
	promRegistry.MustRegister(kvCommitDuration)
	promRegistry.MustRegister(kvLockRetries)
	promRegistry.MustRegister(kvCommitFailures)
}

func (s *server) statsd() {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		err := s.kv.Ping()
		if err != nil {
			w.WriteHeader(503)
			return
		}

		w.Write([]byte("OK"))
	})

	// Prometheus metrics endpoint with our custom registry
	mux.Handle("/metrics", promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{}))

	healthServer := &http.Server{
		Addr:    ":27667",
		Handler: mux,
	}

	err := healthServer.ListenAndServe()
	panic(err)
}

// PrometheusMiddleware records HTTP request metrics
func PrometheusMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()

		err := next(c)

		// Record metrics after the request is processed
		duration := time.Since(start).Seconds()
		status := c.Response().Status
		method := c.Request().Method
		path := c.Request().URL.Path

		// Increment the request counter
		httpRequestsTotal.WithLabelValues(method, path, fmt.Sprintf("%d", status)).Inc()

		// Record the request duration
		httpRequestDuration.WithLabelValues(method, path, fmt.Sprintf("%d", status)).Observe(duration)

		return err
	}
}
