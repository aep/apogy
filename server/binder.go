package server

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
	"net/http"
)

type Binder struct {
	defaultBinder *echo.DefaultBinder
}

func (cb *Binder) Bind(i interface{}, c echo.Context) error {
	// Handle binding differently based on content type
	if c.Request().Method == http.MethodPost || c.Request().Method == http.MethodPut {
		contentType := c.Request().Header.Get(echo.HeaderContentType)

		if contentType == echo.MIMEApplicationJSON {
			// For JSON, use decoder.UseNumber to preserve numeric values
			dec := json.NewDecoder(c.Request().Body)
			dec.UseNumber()

			if err := dec.Decode(i); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			return nil
		}
	}

	// For other cases, use the default binder
	return cb.defaultBinder.Bind(i, c)
}
