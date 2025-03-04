package apogy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
)

type TypedClient[Val any] struct {
	*Client
	Model string
}

type TypedDocument[Val any] struct {
	Document
	Val Val `json:"val"`
}

type TypedSearchResponse[Val any] struct {
	Cursor    *string              `json:"cursor,omitempty"`
	Documents []TypedDocument[Val] `json:"documents"`
}

func parseError(rsp *http.Response) error {
	var msg ErrorResponse
	json.NewDecoder(rsp.Body).Decode(&msg)
	if msg.Message != nil && *msg.Message != "" {
		return fmt.Errorf("%d: %s", rsp.StatusCode, *msg.Message)
	}
	return fmt.Errorf("Status %d: %s", rsp.StatusCode, rsp.Status)
}

func (c *TypedClient[Val]) Get(ctx context.Context, id string, reqEditors ...RequestEditorFn) (*TypedDocument[Val], error) {
	rsp, err := c.GetDocument(ctx, c.Model, id, reqEditors...)
	if err != nil {
		return nil, err
	}

	defer rsp.Body.Close()

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest = new(TypedDocument[Val])
		if err := json.NewDecoder(rsp.Body).Decode(dest); err != nil {
			return nil, err
		}
		return dest, nil
	}
	return nil, parseError(rsp)
}

func (c *TypedClient[Val]) Put(ctx context.Context, body *TypedDocument[Val], reqEditors ...RequestEditorFn) error {

	var bodyReader io.Reader
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	bodyReader = bytes.NewReader(buf)

	req, err := NewPutDocumentRequestWithBody(c.Server, "application/json", bodyReader)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return err
	}

	rsp, err := c.Client.Client.Do(req)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode == 200 {
		return nil
	}
	return parseError(rsp)
}

func (c *TypedClient[Val]) Query(ctx context.Context, aql string, reqEditors ...RequestEditorFn) iter.Seq2[*TypedDocument[Val], error] {

	var cursor *string

	return func(yield func(*TypedDocument[Val], error) bool) {

		qq := aql
		if cursor != nil {
			qq = `(cursor="` + *cursor + `") ` + aql
		}

		rsp, err := c.Client.SearchDocumentsWithBody(context.Background(), "application/x-aql", strings.NewReader(qq))
		if err != nil {
			yield(nil, err)
			return
		}

		defer rsp.Body.Close()

		switch {
		case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
			var dest = new(TypedSearchResponse[Val])
			if err := json.NewDecoder(rsp.Body).Decode(dest); err != nil {
				yield(nil, err)
				return
			}
			for _, doc := range dest.Documents {
				if !yield(&doc, nil) {
					return
				}
			}

			cursor = dest.Cursor
		}
	}
}

func (c *TypedClient[Val]) QueryOne(ctx context.Context, aql string, reqEditors ...RequestEditorFn) (*TypedDocument[Val], error) {

	rsp, err := c.Client.SearchDocumentsWithBody(context.Background(), "application/x-aql", strings.NewReader(aql))
	if err != nil {
		return nil, err
	}

	defer rsp.Body.Close()

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest = new(TypedSearchResponse[Val])
		if err := json.NewDecoder(rsp.Body).Decode(dest); err != nil {
			return nil, err
		}
		for _, doc := range dest.Documents {
			return &doc, nil
		}
	}

	return nil, fmt.Errorf("not found")
}
