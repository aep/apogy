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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
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
		return Error{Code: rsp.StatusCode, Message: msg.Message}
	} else if msg.Error != "" {
		return Error{Code: rsp.StatusCode, Message: msg.Error}
	}

	return Error{Code: rsp.StatusCode, Message: rsp.Status}
}

// addTracingContext injects OpenTelemetry trace context into the request headers
func addTracingContext() RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		// Check if we have a span context in the current context
		spanCtx := trace.SpanContextFromContext(ctx)
		if spanCtx.IsValid() {
			// First, try to use the global propagator
			propagator := otel.GetTextMapPropagator()
			if propagator != nil {
				propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))
			} else {
				fmt.Println("Client: Warning - global propagator is nil, using direct header injection")
			}

			// As a fallback, directly set W3C trace context headers
			// This ensures trace context is propagated even if the global propagator isn't configured
			traceParent := fmt.Sprintf("00-%s-%s-%s",
				spanCtx.TraceID().String(),
				spanCtx.SpanID().String(),
				"01") // Simple flags, assuming sampled

			req.Header.Set("traceparent", traceParent)

			// If there's a tracestate, include it too
			if spanCtx.TraceState().Len() > 0 {
				req.Header.Set("tracestate", spanCtx.TraceState().String())
			}
		}

		return nil
	}
}

func (c *TypedClient[Doc]) Get(ctx context.Context, id string, reqEditors ...RequestEditorFn) (*Doc, error) {
	reqEditors = append([]RequestEditorFn{addTracingContext()}, reqEditors...)

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
	reqEditors = append([]RequestEditorFn{addTracingContext()}, reqEditors...)

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

func (c *TypedClient[Doc]) MutOne(ctx context.Context, id string, muts interface{}, reqEditors ...RequestEditorFn) (*Doc, error) {
	reqEditors = append([]RequestEditorFn{addTracingContext()}, reqEditors...)

	var body = map[string]interface{}{
		"id":    id,
		"model": c.Model,
		"mut":   muts,
	}

	var bodyReader io.Reader
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	bodyReader = bytes.NewReader(buf)

	req, err := NewPutDocumentRequestWithBody(c.Server, "application/json", bodyReader)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}

	rsp, err := c.Client.Client.Do(req)
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

func (c *TypedClient[Doc]) Put(ctx context.Context, body *Doc, reqEditors ...RequestEditorFn) (*Doc, error) {
	reqEditors = append([]RequestEditorFn{addTracingContext()}, reqEditors...)

	var bodyReader io.Reader
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	bodyReader = bytes.NewReader(buf)

	req, err := NewPutDocumentRequestWithBody(c.Server, "application/json", bodyReader)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}

	rsp, err := c.Client.Client.Do(req)
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
