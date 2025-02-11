package server

import (
	"apogy/api/go"
	"apogy/bus"
	"apogy/kv"
	"apogy/reactor"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type server struct {
	kv kv.KV
	bs bus.Bus
	ra *reactor.Reactor
}

func newServer(kv kv.KV, bs bus.Bus) *server {
	return &server{
		kv: kv,
		bs: bs,
		ra: reactor.New(),
	}
}

func Main() {
	kv, err := kv.NewPebble()
	if err != nil {
		panic(err)
	}

	st, err := bus.NewSolo()
	if err != nil {
		panic(err)
	}

	s := newServer(kv, st)
	e := echo.New()

	// Add logging middleware
	e.Use(loggingMiddleware)

	// Register handlers
	openapi.RegisterHandlers(e, s)

	// Start server
	fmt.Println("Starting Echo server on :5052")
	if err := e.Start(":5052"); err != http.ErrServerClosed {
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
