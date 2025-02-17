package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"

	"github.com/aep/apogy/api/go"
	"github.com/aep/apogy/aql"
	"github.com/aep/apogy/kv"
	"github.com/labstack/echo/v4"
)

const MAX_RESULTS = 200

type findResult struct {
	documents []openapi.Document
	cursor    *string
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

func (s *server) find(ctx context.Context, r kv.Read, model string, id string, filter *openapi.Filter, limit int, cursor *string, full bool) (findResult, error) {
	start := makeKey(model, filter)

	if id != "" {
		if filter.Equal == nil || *filter.Equal == nil {
			return findResult{}, echo.NewHTTPError(http.StatusBadRequest, "second filter currently can only be a k=v")
		}
		start = append(start, []byte(id)...)

		if cursor != nil {
			panic("incorrect usage")
		}
	}

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
	var documents []openapi.Document
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
			var doc openapi.Document
			doc.Model = model
			doc.Id = id
			if full {
				err := s.getDocument(ctx, model, id, &doc)
				if err != nil {
					return findResult{}, err
				}
			}
			documents = append(documents, doc)
			lastKey = kv.K
		}

		if len(documents) >= limit {
			break
		}
	}

	var nextCursor *string
	if len(documents) >= limit && lastKey != nil {
		nextKey := bytes.Clone(lastKey)
		nextKey[len(nextKey)-2] = nextKey[len(nextKey)-2] + 2
		cursor := base64.StdEncoding.EncodeToString(nextKey)
		nextCursor = &cursor
	}

	return findResult{documents: documents, cursor: nextCursor}, nil
}

func (s *server) SearchDocuments(c echo.Context) error {

	var req openapi.SearchRequest

	if c.Request().Header.Get("Content-Type") == "application/x-aql" {
		body, _ := io.ReadAll(c.Request().Body)
		c.Request().Body.Close()
		qa, err := aql.Parse(string(body))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
		}
		req = *qa.ToSearchRequest()
	} else {
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
		}
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
	var full = false
	if req.Full != nil {
		full = *req.Full
	}

	var response openapi.SearchResponse

	if req.Filters == nil || len(*req.Filters) == 0 {
		result, err := s.find(c.Request().Context(), r, req.Model, "", nil, limit, req.Cursor, full)
		if err != nil {
			return err
		}
		response.Documents = result.documents
		response.Cursor = result.cursor
		return c.JSON(http.StatusOK, response)
	}

	result, err := s.find(c.Request().Context(), r, req.Model, "", &(*req.Filters)[0], limit, req.Cursor, full)
	if err != nil {
		return err
	}

	var matchedDocs []openapi.Document
	for i, doc := range result.documents {
		allMatch := true
		for _, filter := range (*req.Filters)[1:] {

			subResult, err := s.find(c.Request().Context(), r, req.Model, doc.Id, &filter, 1, nil, false)
			if err != nil {
				return err
			}

			found := false
			for _, subId := range subResult.documents {
				if doc.Id == subId.Id {
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
			matchedDocs = append(matchedDocs, result.documents[i])
		}
	}

	response.Documents = matchedDocs
	response.Cursor = result.cursor
	return c.JSON(http.StatusOK, response)
}
