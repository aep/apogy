package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/aep/apogy/api/go"
	"github.com/aep/apogy/kv"
	"iter"
)

func (s *server) find(ctx context.Context, r kv.Read, model string, byID *string,
	cursor *string, filter *openapi.Filter) iter.Seq2[string, error] {

	var start = []byte{'f', 0xff}
	start = append(start, []byte(model)...)
	start = append(start, 0xff)

	if filter != nil {
		start = append(start, []byte(filter.Key)...)

		if filter.Equal != nil {
			if strVal, ok := (*filter.Equal).(string); ok {
				start = append(start, 0xff)
				start = append(start, []byte(strVal)...)
				start = append(start, 0xff)
			}

			if byID != nil {
				start = append(start, []byte(*byID)...)
			}
		} else {
			start = append(start, 0x00)
		}
	}

	end := bytes.Clone(start)
	if filter != nil {
		end[len(end)-2] = end[len(end)-2] + 1
	} else {
		end[len(end)-1] = end[len(end)-1] + 1
	}

	if cursor != nil {
		cursorBytes, err := base64.StdEncoding.DecodeString(*cursor)
		if err == nil && len(cursorBytes) > 0 {
			// Verify the cursor is within our range
			if bytes.Compare(cursorBytes, start) >= 0 && bytes.Compare(cursorBytes, end) < 0 {
				start = cursorBytes
			}
			// If cursor is invalid or out of range, we'll use the original start position
		}
	}

	return func(yield func(string, error) bool) {
		seen := make(map[string]bool)

		for kv, err := range r.Iter(ctx, start, end) {
			if err != nil {
				yield("", err)
				return
			}

			kk := bytes.Split(kv.K, []byte{0xff})
			if len(kk) < 3 {
				continue
			}
			id := string(kk[len(kk)-1])

			if byID != nil && *byID != id {
				continue
			}

			if seen[id] {
				continue
			}
			seen[id] = true

			if !yield(id, nil) {
				return
			}
		}
	}
}

func (s *server) SearchDocuments(c echo.Context) error {
	var req openapi.SearchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Validate request
	if err := s.validateSearchRequest(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	r := s.kv.Read()
	if r, ok := r.(*kv.TikvRead); ok {
		r.SetKeyOnly(true)
	}
	defer r.Close()

	var response openapi.SearchResponse
	var ids []string
	var lastKey []byte
	count := 0
	maxResults := 10

	if req.Filters == nil || len(*req.Filters) == 0 {
		// When no filters are provided, use find with nil filter
		for id, err := range s.find(c.Request().Context(), r, req.Model, nil, req.Cursor, nil) {
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Search operation failed")
			}
			ids = append(ids, id)
			count++
			
			// Store the last seen key for cursor
			var start = []byte{'f', 0xff}
			start = append(start, []byte(req.Model)...)
			start = append(start, 0xff)
			start = append(start, []byte(id)...)
			lastKey = start

			if count >= maxResults {
				break
			}
		}
	} else {
		for id, err := range s.find(c.Request().Context(), r, req.Model, nil, req.Cursor, &(*req.Filters)[0]) {
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Search operation failed")
			}

			allMatch := true
			for _, filter := range (*req.Filters)[1:] {
				thisMatch := false
				for matchID, err := range s.find(c.Request().Context(), r, req.Model, &id, nil, &filter) {
					if err != nil {
						return echo.NewHTTPError(http.StatusInternalServerError, "Search operation failed")
					}
					if id == matchID {
						thisMatch = true
						break
					}
				}
				if !thisMatch {
					allMatch = false
					break
				}
			}

			if allMatch {
				ids = append(ids, id)
				count++

				// Store the last seen key for cursor
				var start = []byte{'f', 0xff}
				start = append(start, []byte(req.Model)...)
				start = append(start, 0xff)
				start = append(start, []byte(id)...)
				lastKey = start

				if count >= maxResults {
					break
				}
			}
		}
	}

	response.Ids = &ids
	
	// Set cursor only if we have reached the maximum results
	if count >= maxResults && lastKey != nil {
		cursor := base64.StdEncoding.EncodeToString(lastKey)
		response.Cursor = &cursor
	}

	return c.JSON(http.StatusOK, response)
}

func (s *server) validateSearchRequest(req *openapi.SearchRequest) error {
	if req.Model == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Model is required")
	}

	if bytes.Contains([]byte(req.Model), []byte{0xff}) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid utf8 string in model")
	}

	if req.Cursor != nil && bytes.Contains([]byte(*req.Cursor), []byte{0xff}) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid utf8 string in cursor")
	}

	if req.Filters != nil {
		for _, filter := range *req.Filters {
			if bytes.Contains([]byte(filter.Key), []byte{0xff}) {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid utf8 string in filter key")
			}

			if err := s.validateFilterConditions(filter); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *server) validateFilterConditions(filter openapi.Filter) error {
	if filter.Equal != nil {
		if strVal, ok := (*filter.Equal).(string); ok {
			if bytes.Contains([]byte(strVal), []byte{0xff}) {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid utf8 string in query")
			}
		}
	}

	if filter.Less != nil {
		if strVal, ok := (*filter.Less).(string); ok {
			if bytes.Contains([]byte(strVal), []byte{0xff}) {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid utf8 string in query")
			}
		}
	}

	if filter.Greater != nil {
		if strVal, ok := (*filter.Greater).(string); ok {
			if bytes.Contains([]byte(strVal), []byte{0xff}) {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid utf8 string in query")
			}
		}
	}

	return nil
}