package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	openapi "github.com/aep/apogy/api/go"
	"github.com/aep/apogy/bus"
	"github.com/aep/apogy/kv"
	"github.com/aep/apogy/reactor"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/maypok86/otter"
)

type server struct {
	kv         kv.KV
	bs         bus.Bus
	ro         *reactor.Reactor
	modelCache otter.Cache[string, *Model]
}

func Main(caCertPath, serverCertPath, serverKeyPath string) {

	kv, err := kv.NewTikv()
	if err != nil {
		panic(err)
	}

	bs, err := bus.NewSolo()
	if err != nil {
		panic(err)
	}

	cache, err := otter.MustBuilder[string, *Model](100000).
		WithTTL(60 * time.Second).
		Build()

	if err != nil {
		panic(err)
	}

	s := &server{
		kv:         kv,
		bs:         bs,
		modelCache: cache,
	}

	s.ro = reactor.NewReactor(caCertPath, serverCertPath, serverKeyPath)

	e := echo.New()
	e.HideBanner = true

	e.Binder = &Binder{
		defaultBinder: &echo.DefaultBinder{},
	}

	// Add middleware
	e.Use(TracingMiddleware)    // Add OpenTelemetry tracing middleware
	e.Use(PrometheusMiddleware) // Add Prometheus metrics middleware
	e.Use(loggingMiddleware)
	e.Use(middleware.BodyLimit("2M"))

	// Register OpenAPI handlers
	openapi.RegisterHandlers(e, s)

	// Start server
	s.startup()

	go s.statsd()

	if caCertPath != "" && serverCertPath != "" && serverKeyPath != "" {
		// Load CA certificate for client verification
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			panic(fmt.Sprintf("failed to read CA certificate: %v", err))
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			panic("failed to append CA certificate to pool")
		}

		// Configure TLS with client certificate verification
		tlsConfig := &tls.Config{
			ClientCAs:  caCertPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS12,
		}

		s := &http.Server{
			Addr:      ":27666",
			TLSConfig: tlsConfig,
			Handler:   e,
		}

		fmt.Println("⇨ APOGY [tikv, solo, mTLS]")
		if err := s.ListenAndServeTLS(serverCertPath, serverKeyPath); err != http.ErrServerClosed {
			panic(fmt.Sprintf("failed to serve with TLS: %v", err))
		}
	} else {
		fmt.Println("⇨ APOGY [tikv, solo, insecure]")
		// Wrap Echo handler with OpenTelemetry
		handler := e
		s := &http.Server{
			Addr:    ":27666",
			Handler: handler,
		}
		if err := s.ListenAndServe(); err != http.ErrServerClosed {
			panic(fmt.Sprintf("failed to serve: %v", err))
		}
	}
}

// Logging middleware
func loggingMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()

		err := next(c)
		if err != nil {
			c.Error(err)
		}

		req := c.Request()
		res := c.Response()

		slog.Info("http",
			"method", req.Method,
			"path", req.URL.Path,
			"status", res.Status,
			"duration", time.Since(start),
			"error", err,
		)

		return err
	}
}
