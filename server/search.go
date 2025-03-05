package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	openapi "github.com/aep/apogy/api/go"
	"github.com/aep/apogy/aql"
	"github.com/aep/apogy/kv"
	"github.com/labstack/echo/v4"
	"github.com/vmihailenco/msgpack/v5"
)

const MAX_RESULTS = 200

type findResult struct {
	documents []openapi.Document
	cursor    *string
}

func makeKey(model string, filter *openapi.Filter) ([]byte, error) {
	var key []byte
	if filter == nil {
		key = []byte{'o', 0xff}
		key = append(key, []byte(model)...)
		key = append(key, 0xff)
		return key, nil
	}

	if filter.Key == "id" {
		key = []byte{'o', 0xff}
		key = append(key, []byte(model)...)
	} else {
		key = []byte{'f', 0xff}
		key = append(key, []byte(model)...)
		key = append(key, 0xff)
		key = append(key, []byte(filter.Key)...)
	}

	if filter.Equal != nil {
		if strVal, ok := (*filter.Equal).(string); ok {
			key = append(key, 0xff)
			key = append(key, []byte(strVal)...)
			key = append(key, 0xff)
		} else {
			return nil, fmt.Errorf("%T can't be used as search val", *filter.Equal)
		}
	} else if filter.Greater != nil {
		if strVal, ok := (*filter.Greater).(string); ok {
			key = append(key, 0xff)
			key = append(key, []byte(strVal)...)
		} else {
			return nil, fmt.Errorf("%T can't be used as search val for greater than", *filter.Greater)
		}
	} else if filter.Prefix != nil {
		if strVal, ok := (*filter.Prefix).(string); ok {
			key = append(key, 0xff)
			key = append(key, []byte(strVal)...)

			// FIXME this is a prefix search rather than actually greater
			key = append(key, 0x00)
		} else {
			return nil, fmt.Errorf("%T can't be used as search val for greater than", *filter.Greater)
		}
	} else if filter.Less != nil {
		// exact key but any value
		key = append(key, 0xff)
	} else {
		// any key including sub
		key = append(key, 0x00)
	}
	return key, nil
}

func (s *server) find(ctx context.Context, r kv.Read, model string, id string, filter *openapi.Filter, limit int, cursor *string) (findResult, error) {

	// if the filter is by exact id, just return the object directly
	if filter != nil && filter.Key == "id" && filter.Equal != nil {
		if id, ok := (*filter.Equal).(string); ok {

			var doc openapi.Document
			doc.Id = id
			doc.Model = model
			return findResult{documents: []openapi.Document{doc}}, nil
		}
	}

	start, err := makeKey(model, filter)
	if err != nil {
		return findResult{}, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// we're in a sub filter
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

	if filter != nil && filter.Less != nil {
		if strVal, ok := (*filter.Less).(string); ok {
			// For Less filter, explicitly set the end key to the specified value
			end = []byte{'f', 0xff}
			end = append(end, []byte(model)...)
			end = append(end, 0xff)
			end = append(end, []byte(filter.Key)...)
			end = append(end, 0xff)
			end = append(end, []byte(strVal)...)
			end = append(end, 0xff)
		}
	} else {
		end[len(end)-2] = end[len(end)-2] + 1
	}

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

	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	r := s.kv.Read()
	defer r.Close()

	rsp, err := s.query(c.Request().Context(), r, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rsp)
}

func (s *server) QueryDocuments(c echo.Context) error {

	var req openapi.Query
	var qa *aql.Query
	var err error

	err = c.Bind(&req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.Params != nil {
		qa, err = aql.Parse(req.Q, *req.Params...)
	} else {
		qa, err = aql.Parse(req.Q)
	}

	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	srq := *qa.ToSearchRequest()
	srq.Cursor = req.Cursor
	srq.Limit = req.Limit

	r := s.kv.Read()
	defer r.Close()

	if strings.Contains(c.Request().Header.Get("Accept"), "application/jsonl") {

		c.Response().Header().Set("Content-Type", "application/jsonl")

		for {
			rsp, err := s.query(c.Request().Context(), r, srq)
			if err != nil {
				select {
				case <-c.Request().Context().Done():
					return nil
				default:
					errstr := err.Error()
					return c.JSON(http.StatusBadRequest, &openapi.SearchResponse{Error: &errstr})
				}
			}

			err = json.NewEncoder(c.Response()).Encode(rsp)
			if err != nil {
				select {
				case <-c.Request().Context().Done():
					return nil
				default:
					errstr := err.Error()
					return c.JSON(http.StatusBadRequest, &openapi.SearchResponse{Error: &errstr})
				}
			}
			c.Response().Write([]byte("\n"))

			if rsp.Cursor == nil {
				return nil
			}

			srq.Cursor = rsp.Cursor
		}

	} else {

		rsp, err := s.query(c.Request().Context(), r, srq)
		if err != nil {
			return err
		}

		return c.JSON(http.StatusOK, rsp)
	}

}

func (s *server) resolveFullDocs(ctx context.Context, r kv.Read, fr []openapi.Document) ([]openapi.Document, error) {

	keys := [][]byte{}
	keysExtraCheck := make(map[string]bool)
	for _, doc := range fr {
		path, err := safeDBPath(doc.Model, doc.Id)
		if err != nil {
			return nil, err
		}

		keys = append(keys, path)

		keysExtraCheck[string(path)] = true
	}

	vals, err := r.BatchGet(ctx, keys)
	if err != nil {
		return nil, err
	}

	var ret []openapi.Document

	for key, val := range vals {

		if val == nil {
			continue
		}
		if !keysExtraCheck[string(key)] {
			// unlikely bug in tikv, but lets make extra sure
			continue
		}

		var doc openapi.Document
		if err := msgpack.Unmarshal(val, &doc); err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError, "unmarshal error")
		}

		ret = append(ret, doc)
	}

	return ret, nil
}
func (s *server) query(ctx context.Context, r kv.Read, req openapi.SearchRequest) (*openapi.SearchResponse, error) {

	if req.Model == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "Model is required")
	}

	if req.Links != nil && len(*req.Links) > 0 {
		var full = true
		req.Full = &full
	}

	// TODO: do we need this? the search values are empty anyway
	if r, ok := r.(*kv.TikvRead); ok {
		r.SetKeyOnly(true)
	}

	var limit = MAX_RESULTS
	if req.Limit != nil {
		limit = *req.Limit
	}

	var cursor *string
	var matchedDocs []openapi.Document

	if req.Filters == nil || len(*req.Filters) == 0 {
		result, err := s.find(ctx, r, req.Model, "", nil, limit, req.Cursor)
		if err != nil {
			return nil, err
		}
		matchedDocs = result.documents
		cursor = result.cursor

	} else {

		result, err := s.find(ctx, r, req.Model, "", &(*req.Filters)[0], limit, req.Cursor)
		if err != nil {
			return nil, err
		}
		cursor = result.cursor

		for i, doc := range result.documents {
			allMatch := true
			for _, filter := range (*req.Filters)[1:] {

				subResult, err := s.find(ctx, r, req.Model, doc.Id, &filter, 1, nil)
				if err != nil {
					return nil, err
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
	}

	if req.Full != nil && *req.Full {
		var err error
		matchedDocs, err = s.resolveFullDocs(ctx, r, matchedDocs)
		if err != nil {
			return nil, err
		}
		if req.Model == "Reactor" {
			for i, doc := range matchedDocs {
				s.ro.Status(ctx, &doc)
				matchedDocs[i] = doc
			}
		}
	}

	if req.Links != nil && len(*req.Links) > 0 {

		var modelDoc openapi.Document
		err := s.getDocument(ctx, "Model", req.Model, &modelDoc)
		if err != nil {
			return nil, err
		}

		modelVal, _ := modelDoc.Val.(map[string]interface{})
		if modelVal == nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError, "unable to resolve subquery: model has missing or invalid schema")
		}

		properties, ok := modelVal["schema"].(map[string]interface{})
		if !ok {
			return nil, echo.NewHTTPError(http.StatusInternalServerError, "unable to resolve subquery: model has missing or invalid schema")
		}

		for i := range matchedDocs {

			vals, _ := matchedDocs[i].Val.(map[string]interface{})
			if vals == nil {
				continue
			}

			for _, link := range *req.Links {

				if !strings.HasPrefix(link.Model, "val.") {
					return nil, echo.NewHTTPError(http.StatusBadRequest, "unable to resolve subquery: did you mean val."+link.Model+" ?")
				}

				propname := strings.TrimPrefix(link.Model, "val.")
				propDef, ok := properties[propname]
				if !ok {
					return nil, echo.NewHTTPError(http.StatusBadRequest, "unable to resolve subquery: model has no property "+propname)
				}

				propMap, ok := propDef.(map[string]interface{})
				if !ok {
					continue
				}

				// Check if this property has a link
				linkedModel, ok := propMap["link"].(string)
				if !ok {
					continue
				}

				val, ok := vals[propname]
				if !ok {
					continue
				}

				link.Model = linkedModel
				if link.Filters == nil {
					link.Filters = new([]openapi.Filter)
				}
				*link.Filters = append(*link.Filters, openapi.Filter{
					Key:   "id",
					Equal: &val,
				})

				linkResult, err := s.query(ctx, r, link)
				if err != nil {
					// TODO what to do with dangling link?
					continue
				}

				if len(linkResult.Documents) < 1 {
					continue
				}

				vals[propname] = linkResult.Documents[0]
				matchedDocs[i].Val = vals
			}
		}
	}

	return &openapi.SearchResponse{
		Documents: matchedDocs,
		Cursor:    cursor,
	}, nil
}
