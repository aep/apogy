package server

import (
	"apogy/api"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"strings"

	kv "apogy/kv"

	"github.com/labstack/echo/v4"
)

func (s *server) find(ctx context.Context, r kv.Read, model string, byID *string,
	cursor *string, filter api.Filter) iter.Seq2[string, error] {

	var start = []byte{'f', 0xff}
	start = append(start, []byte(model)...)
	start = append(start, 0xff)
	start = append(start, []byte(filter.Key)...)

	if filter.Equal != nil {
		start = append(start, 0xff)
		start = append(start, filter.Equal.(string)...)
		start = append(start, 0xff)

		if byID != nil {
			start = append(start, []byte(*byID)...)
		}
		//FIXME optimize the byID case

	} else {
		start = append(start, 0x00)
	}

	end := bytes.Clone(start)
	end[len(end)-2] = end[len(end)-2] + 1

	fmt.Println(escapeNonPrintable(start))
	fmt.Println(escapeNonPrintable(end))

	if cursor != nil {
		// FIXME check if cursor is above start
		// FIXME change start to cursor
	}

	return func(yield func(string, error) bool) {

		var seen = make(map[string]bool)

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

			if byID != nil {
				if *byID != id {
					continue
				}
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

func (s *server) handleSearch(c echo.Context) error {

	var qq api.SearchRequest

	if c.Request().Method == "POST" {
		if err := c.Bind(&qq); err != nil {
			return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
				Error: fmt.Sprintf("invalid request body: %v", err),
			})
		}
	} else {
		q := c.QueryParam("q")
		err := json.Unmarshal([]byte(q), &qq)
		if err != nil {
			return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
				Error: fmt.Sprintf("invalid query: %v", err),
			})
		}
	}

	for _, ch := range qq.Model {
		if ch == 0xff {
			return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
				Error: fmt.Sprintf("invalid query"),
			})
		}
	}
	for _, ch := range qq.Cursor {
		if ch == 0xff {
			return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
				Error: fmt.Sprintf("invalid query"),
			})
		}
	}
	for _, f := range qq.Filters {
		for _, ch := range f.Key {
			if ch == 0xff {
				return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
					Error: fmt.Sprintf("invalid query"),
				})
			}
		}
		if v, ok := f.Equal.(string); ok {
			for _, ch := range v {
				if ch == 0xff {
					return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
						Error: fmt.Sprintf("invalid query"),
					})
				}
			}
		}
		if v, ok := f.Greater.(string); ok {
			for _, ch := range v {
				if ch == 0xff {
					return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
						Error: fmt.Sprintf("invalid query"),
					})
				}
			}
		}
		if v, ok := f.Less.(string); ok {
			for _, ch := range v {
				if ch == 0xff {
					return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
						Error: fmt.Sprintf("invalid query"),
					})
				}
			}
		}
	}

	if len(qq.Filters) == 0 || qq.Model == "" {
		return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
			Error: fmt.Sprintf("invalid query"),
		})
	}

	r := s.kv.Read()
	r.(*kv.TikvRead).SetKeyOnly(true)
	defer r.Close()

	// TODO cursor

	var rsp api.Cursor

	for k, err := range s.find(c.Request().Context(), r, qq.Model, nil, nil, qq.Filters[0]) {
		if err != nil {
			return c.JSON(http.StatusInternalServerError, api.PutObjectResponse{
				Error: fmt.Sprintf("tikv range: %v", err),
			})
		}

		allMatch := true
		for _, fine := range qq.Filters[1:] {

			thisMatch := false
			for k2, err := range s.find(c.Request().Context(), r, qq.Model, &k, nil, fine) {
				if err != nil {
					return c.JSON(http.StatusInternalServerError, api.PutObjectResponse{
						Error: fmt.Sprintf("tikv range: %v", err),
					})
				}
				if k == k2 {
					thisMatch = true
					break
				}
			}
			if !thisMatch {
				allMatch = false
				break
			}
		}

		if !allMatch {
			continue
		}
		rsp.Keys = append(rsp.Keys, k)
	}

	return c.JSON(http.StatusOK, rsp)
}

func escapeNonPrintable(b []byte) string {
	var result strings.Builder
	for _, c := range b {
		if c >= 32 && c <= 126 {
			result.WriteByte(c)
		} else {
			result.WriteString(fmt.Sprintf("\\x%02x", c))
		}
	}
	return result.String()
}
