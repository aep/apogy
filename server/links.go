package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (s *server) checkNothingNeedsModel(ctx context.Context, model string) error {

	r := s.kv.Read()
	defer r.Close()

	res, _ := s.find(ctx, r, model, "", nil, 1, nil)

	if len(res.documents) > 0 {
		return echo.NewHTTPError(http.StatusConflict, fmt.Sprint("model is in use"))
	}

	return nil
}
