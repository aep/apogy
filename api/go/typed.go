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

type TypedClient[Doc any] struct {
	*Client
	Model string
}

type TypedSearchResponse[Doc any] struct {
	Cursor    *string `json:"cursor,omitempty"`
	Documents []Doc   `json:"documents"`
}

func parseError(rsp *http.Response) error {
	var msg struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	json.NewDecoder(rsp.Body).Decode(&msg)
	if msg.Message != "" {
		return fmt.Errorf("%d: %s", rsp.StatusCode, msg.Message)
	} else if msg.Error != "" {
		return fmt.Errorf("%d: %s", rsp.StatusCode, msg.Error)
	}

	return fmt.Errorf("Status %d: %s", rsp.StatusCode, rsp.Status)
}

func (c *TypedClient[Doc]) Get(ctx context.Context, id string, reqEditors ...RequestEditorFn) (*Doc, error) {
	rsp, err := c.GetDocument(ctx, c.Model, id, reqEditors...)
	if err != nil {
		return nil, err
	}

	defer rsp.Body.Close()

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest = new(Doc)
		if err := json.NewDecoder(rsp.Body).Decode(dest); err != nil {
			return nil, err
		}
		return dest, nil
	}
	return nil, parseError(rsp)
}

func (c *TypedClient[Doc]) Delete(ctx context.Context, id string, reqEditors ...RequestEditorFn) error {
	rsp, err := c.DeleteDocument(ctx, c.Model, id, reqEditors...)
	if err != nil {
		return err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode >= 300 {
		return parseError(rsp)
	}

	return nil
}

func (c *TypedClient[Doc]) Put(ctx context.Context, body *Doc, reqEditors ...RequestEditorFn) error {

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

func (c *TypedClient[Doc]) Query(ctx context.Context, args ...interface{}) iter.Seq2[*Doc, error] {

	q := c.Model
	if len(args) > 0 {
		q = fmt.Sprintf("%s(%s)", c.Model, args[0])
		args = args[1:]
	}

	return query[Doc](c, ctx, q, args...)
}

func (c *TypedClient[Doc]) QueryOne(ctx context.Context, args ...interface{}) (*Doc, error) {
	q := c.Model
	if len(args) > 0 {
		q = fmt.Sprintf("%s(%s)", c.Model, args[0])
		args = args[1:]
	}

	return queryOne[Doc](c, ctx, q, args...)
}
