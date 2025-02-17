package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"

	"github.com/aep/apogy/api/go"
	"github.com/aep/apogy/kv"
	"github.com/labstack/echo/v4"
)

const MAX_RESULTS = 200

type findResult struct {
	ids    []string
	cursor *string
}

func makeKey(model string, filter *openapi.Filter) []byte {
	var key []byte
	if filter == nil {
		key = []byte{'o', 0xff}
		key = append(key, []byte(model)...)
		key = append(key, 0xff)
		return key
	}

	key = []byte{'f', 0xff}
	key = append(key, []byte(model)...)
	key = append(key, 0xff)
	key = append(key, []byte(filter.Key)...)

	if filter.Equal != nil {
		if strVal, ok := (*filter.Equal).(string); ok {
			key = append(key, 0xff)
			key = append(key, []byte(strVal)...)
			key = append(key, 0xff)
		}
	} else {
		key = append(key, 0x00)
	}
	return key
}

func (s *server) find(ctx context.Context, r kv.Read, model string, filter *openapi.Filter, limit int, cursor *string) (findResult, error) {
	start := makeKey(model, filter)
	end := bytes.Clone(start)
	end[len(end)-2] = end[len(end)-2] + 1

	if cursor != nil {
		if cursorBytes, err := base64.StdEncoding.DecodeString(*cursor); err == nil && len(cursorBytes) > 0 {
			if bytes.Compare(cursorBytes, start) >= 0 && bytes.Compare(cursorBytes, end) < 0 {
				start = cursorBytes
			}
		}
	}

	seen := make(map[string]bool)
	var ids []string
	var lastKey []byte

	for kv, err := range r.Iter(ctx, start, end) {
		if err != nil {
			return findResult{}, err
		}

		idx := bytes.Split(kv.K, []byte{0xff})
		if len(idx) < 3 {
			continue
		}
		id := string(idx[len(idx)-2])
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
			lastKey = kv.K
		}

		if len(ids) >= limit {
			break
		}
	}

	var nextCursor *string
	if len(ids) >= limit && lastKey != nil {
		nextKey := bytes.Clone(lastKey)
		nextKey[len(nextKey)-2] = nextKey[len(nextKey)-2] + 2
		cursor := base64.StdEncoding.EncodeToString(nextKey)
		nextCursor = &cursor
	}

	return findResult{ids: ids, cursor: nextCursor}, nil
}

func (s *server) SearchDocuments(c echo.Context) error {
	var req openapi.SearchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.Model == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Model is required")
	}

	r := s.kv.Read()
	if r, ok := r.(*kv.TikvRead); ok {
		r.SetKeyOnly(true)
	}
	defer r.Close()

	var limit = MAX_RESULTS
	if req.Limit != nil {
		limit = *req.Limit
	}

	var response openapi.SearchResponse

	if req.Filters == nil || len(*req.Filters) == 0 {
		result, err := s.find(c.Request().Context(), r, req.Model, nil, limit, req.Cursor)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		response.Ids = &result.ids
		response.Cursor = result.cursor
		return c.JSON(http.StatusOK, response)
	}

	result, err := s.find(c.Request().Context(), r, req.Model, &(*req.Filters)[0], limit, req.Cursor)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var matchedIds []string
	for _, id := range result.ids {
		allMatch := true
		for _, filter := range (*req.Filters)[1:] {
			subResult, err := s.find(c.Request().Context(), r, req.Model, &filter, limit, nil)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}
			found := false
			for _, subId := range subResult.ids {
				if id == subId {
					found = true
					break
				}
			}
			if !found {
				allMatch = false
				break
			}
		}
		if allMatch {
			matchedIds = append(matchedIds, id)
		}
	}

	response.Ids = &matchedIds
	response.Cursor = result.cursor
	return c.JSON(http.StatusOK, response)
}
