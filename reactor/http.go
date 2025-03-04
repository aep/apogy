package reactor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	openapi "github.com/aep/apogy/api/go"
	"log/slog"
	"net/http"
	"time"
)

type HttpReactor struct {
	url string
}

func StartHttpReactor(doc *openapi.Document) (Runtime, error) {

	val, _ := doc.Val.(map[string]interface{})
	if val == nil {
		return nil, fmt.Errorf("val must not be empty")
	}

	url, ok := val["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("val.url must be a non-empty string")
	}

	return &HttpReactor{
		url: url,
	}, nil
}

func (hr *HttpReactor) Stop() {
}

func (*HttpReactor) Ready(model *openapi.Document, args interface{}) (interface{}, error) {
	return nil, nil
}

func (hr *HttpReactor) Validate(ctx context.Context, old *openapi.Document, nuw *openapi.Document, args interface{}) (*openapi.Document, error) {
	return nuw, nil
}

func (hr *HttpReactor) Reconcile(ctx context.Context, old *openapi.Document, nuw *openapi.Document, args interface{}) error {
	payload := openapi.ValidationRequest{
		Current: old,
		Pending: nuw,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := hr.makeValidationRequest(ctx, hr.url, payloadBytes)
	if err != nil {
		return fmt.Errorf("reconciler failed to respond in time: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp openapi.ErrorResponse
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&errorResp); err != nil || errorResp.Message == nil {
			return fmt.Errorf("reconciler failed with status %d %s", resp.StatusCode, resp.Status)
		}
		return fmt.Errorf("reconcilerfailed: %s", *errorResp.Message)
	}

	// Check for validation error in response
	var response openapi.ValidationResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("reconciler responded with invalid msgpack: %v", err)
	}

	if response.Reject != nil && response.Reject.Message != nil {
		return fmt.Errorf("reconciler rejected: %s", *response.Reject.Message)
	}

	return nil
}

func (hr *HttpReactor) makeValidationRequest(ctx context.Context, url string, payloadBytes []byte) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	initialBackoff := 100 * time.Millisecond
	maxAttempts := 5

	backoff := initialBackoff
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(payloadBytes))
		if err != nil {
			slog.Error("validator failed", "validator", url, "error", err)
			if attempt == maxAttempts {
				return nil, err
			}
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		// the caller doesn't close it if we're in retry
		resp.Body.Close()

		if attempt == maxAttempts {
			return resp, nil
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	return nil, fmt.Errorf("request failed after %d attempts", maxAttempts)
}
