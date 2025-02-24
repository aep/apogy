package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/aep/apogy/api/go"
	"github.com/aep/apogy/bus"
	"github.com/aep/apogy/kv"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type server struct {
	kv kv.KV
	bs bus.Bus
}

func newServer(kv kv.KV, bs bus.Bus) *server {
	return &server{
		kv: kv,
		bs: bs,
	}
}

func Main() {
	kv, err := kv.NewTikv()
	if err != nil {
		panic(err)
	}

	st, err := bus.NewSolo()
	if err != nil {
		panic(err)
	}

	s := newServer(kv, st)
	e := echo.New()
	e.HideBanner = true

	// Add logging middleware
	e.Use(loggingMiddleware)
	e.Use(middleware.BodyLimit("2M"))

	// Register OpenAPI handlers
	openapi.RegisterHandlers(e, s)

	// Start server
	fmt.Println("⇨ APOGY [tikv, solo]")
	if err := e.Start(":27666"); err != http.ErrServerClosed {
		panic(fmt.Sprintf("failed to serve: %v", err))
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
