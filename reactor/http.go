package reactor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	openapi "github.com/aep/apogy/api/go"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type HttpReactor struct {
	validator  string
	reconciler string
}

func StartHttpReactor(doc *openapi.Document) (Runtime, error) {

	val, _ := doc.Val.(map[string]interface{})
	if val == nil {
		return nil, fmt.Errorf("val must not be empty")
	}

	validator, _ := val["validator"].(string)
	reconciler, _ := val["reconciler"].(string)
	if validator == "" && reconciler == "" {
		return nil, fmt.Errorf("set val.validator or val.reconciler to a url")
	}

	return &HttpReactor{
		validator:  validator,
		reconciler: reconciler,
	}, nil
}

func (hr *HttpReactor) Stop() {
}

func (*HttpReactor) Ready(model *openapi.Document, args interface{}) (interface{}, error) {
	return nil, nil
}

func (hr *HttpReactor) Validate(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) (*openapi.Document, error) {

	if hr.validator == "" {
		return nuw, nil
	}

	payload := openapi.ValidationRequest{
		Current: old,
		Pending: nuw,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := hr.makeValidationRequest(ctx, hr.validator, payloadBytes, ro)
	if err != nil {
		return nil, fmt.Errorf("reconciler failed to respond in time: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp openapi.ErrorResponse
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&errorResp); err != nil || errorResp.Message == nil {
			return nil, fmt.Errorf("reconciler failed with status %d %s", resp.StatusCode, resp.Status)
		}
		return nil, fmt.Errorf("reconcilerfailed: %s", *errorResp.Message)
	}

	// Check for validation error in response
	var response openapi.ValidationResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("reconciler responded with invalid body: %v", err)
	}

	if response.Reject != nil && response.Reject.Message != nil {
		return nil, fmt.Errorf("reconciler rejected: %s", *response.Reject.Message)
	}

	return nuw, nil
}

func (hr *HttpReactor) Reconcile(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) error {

	if hr.reconciler == "" {
		return nil
	}

	payload := openapi.ValidationRequest{
		Current: old,
		Pending: nuw,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := hr.makeValidationRequest(ctx, hr.reconciler, payloadBytes, ro)
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
		return fmt.Errorf("reconciler responded with invalid body: %v", err)
	}

	if response.Reject != nil && response.Reject.Message != nil {
		return fmt.Errorf("reconciler rejected: %s", *response.Reject.Message)
	}

	return nil
}

func (hr *HttpReactor) makeValidationRequest(ctx context.Context, url string, payloadBytes []byte, ro *Reactor) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	initialBackoff := 100 * time.Millisecond
	maxAttempts := 5

	// Create a custom client with mTLS for https URLs
	client := &http.Client{}

	backoff := initialBackoff
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		var resp *http.Response

		// If URL starts with https://, use mTLS
		if strings.HasPrefix(url, "https://") {
			resp, err = hr.doMTLSRequest(req, ro)
		} else {
			resp, err = client.Do(req)
		}

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

func (hr *HttpReactor) doMTLSRequest(req *http.Request, ro *Reactor) (*http.Response, error) {
	// Check if TLS client is already created
	if ro.tlsClient == nil {
		return nil, fmt.Errorf("apogy cant make https request to reactor since its not running with tls")
	}

	// Use the pre-configured HTTP client
	return ro.tlsClient.Do(req)
}
