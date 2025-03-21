package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	openapi "github.com/aep/apogy/api/go"
	"github.com/aep/apogy/aql"
	"github.com/aep/apogy/kv"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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

		} else if num, ok := (*filter.Equal).(json.Number); ok {
			vbin := make([]byte, 8)
			if i64, err := num.Int64(); err == nil {
				binary.LittleEndian.PutUint64(vbin, uint64(i64))
			} else {
				return nil, fmt.Errorf("%s can't be used as equal val", num)
			}
			key = append(key, 0xff)
			key = append(key, []byte(vbin)...)
			key = append(key, 0xff)
		} else {
			return nil, fmt.Errorf("%T can't be used as euqal val", *filter.Equal)
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

func (s *server) scan(ctx context.Context, r kv.Read, model string, id string, filter *openapi.Filter, limit int, cursor *string) (findResult, error) {

	fasj, _ := json.Marshal(filter)
	ctx, span := tracer.Start(ctx, "scan", trace.WithAttributes(
		attribute.String("model", model),
		attribute.String("subid", id),
		attribute.String("filter", string(fasj)),
		attribute.Int("limit", limit),
	))
	if cursor != nil {
		span.SetAttributes(attribute.String("cursor", *cursor))
	}
	defer span.End()

	// if the filter is by exact id, use a get
	// dont remove this, its actually nessesary if we're in a subfiltr.
	// we should optimize this some day to just filter in memory instead of doing a tikv roundtrip
	if filter != nil && filter.Key == "id" && filter.Equal != nil {
		if id, ok := (*filter.Equal).(string); ok {
			var doc openapi.Document
			err := s.getDocument(ctx, model, id, &doc)
			if err == nil {
				return findResult{documents: []openapi.Document{doc}}, nil
			} else {
				return findResult{documents: []openapi.Document{}}, nil
			}
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

	seen := make(map[string]bool)
	var documents []openapi.Document

	if cursor != nil {
		if cursorBytes, err := base64.StdEncoding.DecodeString(*cursor); err == nil && len(cursorBytes) > 0 {
			if bytes.Compare(cursorBytes, start) >= 0 && bytes.Compare(cursorBytes, end) < 0 {
				start = cursorBytes
			} else {
				span.RecordError(fmt.Errorf("invalid cursor"))
				slog.Warn("got invalid cursor")
				return findResult{}, nil
			}
		}
	}

	var prefixStartForSkipFilter []byte
	var skipFilter []byte
	if filter != nil && filter.Skip != nil {
		if skipstr, ok := (*filter.Skip).(string); ok {
			skipFilter = []byte(skipstr)
			var err error
			prefixStartForSkipFilter, err = makeKey(model, filter)
			if err != nil {
				return findResult{}, echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
		}
	}

	var lastKey []byte

restartAfterSkip:

	for kv, err := range r.Iter(ctx, start, end) {
		if err != nil {
			return findResult{}, err
		}

		var id string
		var doc openapi.Document

		if filter == nil || filter.Key == "id" {
			idx := bytes.Split(kv.K, []byte{0xff})
			if len(idx) < 3 {
				continue
			}
			id = string(idx[len(idx)-2])

			if seen[id] {
				continue
			}

			seen[id] = true
			doc.Model = model
			doc.Id = id

			// TODO its actually refetched anyway :(
			//if err := DeserializeStore(kv.V, &doc); err != nil {
			//	return findResult{}, echo.NewHTTPError(http.StatusInternalServerError, "unmarshal error")
			//}

			lastKey = kv.K

		} else {
			vdx := bytes.Split(kv.V, []byte{0xff})
			id = string(vdx[0])

			if seen[id] {
				continue
			}

			seen[id] = true
			doc.Model = model
			doc.Id = id
			lastKey = kv.K
		}

		documents = append(documents, doc)
		if len(documents) >= limit {
			break
		}

		if skipFilter != nil {

			prefixLen := len(prefixStartForSkipFilter)

			if prefixLen < len(kv.K) {

				// Search in the part after the prefix
				skipIndex := bytes.Index(kv.K[prefixLen:], []byte(skipFilter))

				if skipIndex >= 0 {
					// Adjust skipIndex to account for the prefix
					skipIndex += prefixLen

					// Create a new nextKey based on current key up to the skip string
					nextKey := make([]byte, skipIndex) // Only copy up to the skip position
					copy(nextKey, kv.K[:skipIndex])

					// Increment the last byte
					nextKey[len(nextKey)-1] = nextKey[len(nextKey)-1] + 1

					// Restart the iterator with the new position
					start = nextKey
					goto restartAfterSkip
				}
			}
		}

	}

	var nextCursor *string
	if len(documents) >= limit && lastKey != nil {
		nextKey := bytes.Clone(lastKey)
		nextKey[len(nextKey)-2] = nextKey[len(nextKey)-2] + 2
		cursor := base64.StdEncoding.EncodeToString(nextKey)
		nextCursor = &cursor
	}

	span.SetAttributes(
		attribute.Int("result", len(documents)),
	)

	return findResult{documents: documents, cursor: nextCursor}, nil
}

func (s *server) SearchDocuments(c echo.Context) error {

	ctx, span := tracer.Start(c.Request().Context(), "Search")
	defer span.End()

	var req openapi.SearchRequest

	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	r := s.kv.Read()
	defer r.Close()

	rsp, err := s.query(ctx, r, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rsp)
}

func (s *server) QueryDocuments(c echo.Context) error {

	ctx, span := tracer.Start(c.Request().Context(), "Query")
	defer span.End()

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

	span.SetAttributes(
		attribute.String("query", qa.String()),
	)

	srq := *qa.ToSearchRequest()
	srq.Cursor = req.Cursor
	srq.Limit = req.Limit

	r := s.kv.Read()
	defer r.Close()

	if strings.Contains(c.Request().Header.Get("Accept"), "application/jsonl") {

		c.Response().Header().Set("Content-Type", "application/jsonl")

		for {
			rsp, err := s.query(ctx, r, srq)
			if err != nil {
				select {
				case <-ctx.Done():
					return nil
				default:
					errstr := err.Error()
					return c.JSON(http.StatusBadRequest, &openapi.SearchResponse{Error: &errstr})
				}
			}

			err = json.NewEncoder(c.Response()).Encode(rsp)
			if err != nil {
				select {
				case <-ctx.Done():
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

			if srq.Limit != nil {
				if *srq.Limit <= len(rsp.Documents) {
					return nil
				}
				*srq.Limit = *srq.Limit - len(rsp.Documents)
			}
		}

	} else {

		rsp, err := s.query(ctx, r, srq)
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
		if err := DeserializeStore(val, &doc); err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError, "unmarshal error")
		}

		ret = append(ret, doc)
	}

	return ret, nil
}

func (s *server) query(ctx context.Context, r kv.Read, req openapi.SearchRequest) (*openapi.SearchResponse, error) {

	ctx, span := tracer.Start(ctx, "query")
	defer span.End()

	if req.Model == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "Model is required")
	}

	if req.Links != nil && len(*req.Links) > 0 {
		var full = true
		req.Full = &full
	}

	var limit = 1000
	if req.Limit != nil {
		limit = *req.Limit
	}

	var cursor *string
	var matchedDocs []openapi.Document

	if req.Filters == nil || len(*req.Filters) == 0 {

		result, err := s.scan(ctx, r, req.Model, "", nil, limit, req.Cursor)
		if err != nil {
			return nil, err
		}
		matchedDocs = result.documents
		cursor = result.cursor

	} else {

		result, err := s.scan(ctx, r, req.Model, "", &(*req.Filters)[0], limit, req.Cursor)
		if err != nil {
			return nil, err
		}
		cursor = result.cursor

		for i, doc := range result.documents {
			allMatch := true

			for _, filter := range (*req.Filters)[1:] {

				subResult, err := s.scan(ctx, r, req.Model, doc.Id, &filter, 1, nil)
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

		links, ok := modelVal["links"].(map[string]interface{})
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
				propDef, ok := links[propname]
				if !ok {
					return nil, echo.NewHTTPError(http.StatusBadRequest, "unable to resolve subquery: not a link "+propname)
				}

				linkDef, ok := propDef.(string)
				if !ok {
					continue
				}

				linkedModel := strings.TrimSpace(strings.TrimPrefix(linkDef, "=>"))

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
