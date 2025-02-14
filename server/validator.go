package server

import (
	"apogy/api/go"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (self *server) makeValidationRequest(ctx context.Context, url string, payloadBytes []byte) (*http.Response, error) {

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

		// the caller doesnt close it if we're in retry
		resp.Body.Close()

		if attempt == maxAttempts {
			return resp, nil
		}

		time.Sleep(backoff)
		backoff *= 2
	}
	return nil, fmt.Errorf("request failed")
}

func (self *server) validate(ctx context.Context, schema *openapi.Document, old *openapi.Document, nuw *openapi.Document) error {
	if schema.Val == nil || (*schema.Val)["reactors"] == nil {
		return nil
	}

	reactors, ok := (*schema.Val)["reactors"].([]interface{})
	if !ok {
		return nil
	}

	for _, r := range reactors {
		reactorID, ok := r.(string)
		if !ok {
			continue
		}

		// Get the Reactor document from the database
		path, err := safeDBPath("Reactor", reactorID)
		if err != nil {
			return fmt.Errorf("invalid reactor ID in model: %s: %v", reactorID, err)
		}

		r := self.kv.Read()
		rb, err := r.Get(ctx, []byte(path))
		r.Close()
		if err != nil {
			return fmt.Errorf("invalid reactor in model: %s: %v", reactorID, err)
		}

		if len(rb) == 0 {
			return fmt.Errorf("invalid reactor in model: %s does not exist", reactorID)
		}

		var reactor openapi.Document
		if err := json.Unmarshal(rb, &reactor); err != nil {
			return fmt.Errorf("invalid reactor in model: %v", err)
		}

		// Extract the validator URL from the reactor
		if reactor.Val == nil {
			slog.Warn(fmt.Sprintf("reactor %s in model %s has no val", reactorID, schema.Id))
			continue
		}

		url, ok := (*reactor.Val)["url"].(string)
		if !ok {
			slog.Warn(fmt.Sprintf("reactor %s in model %s has no val.url", reactorID, schema.Id))
			continue
		}

		payload := openapi.ValidationRequest{
			Current: old,
			Pending: nuw,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		resp, err := self.makeValidationRequest(ctx, url, payloadBytes)
		if err != nil {
			return fmt.Errorf("validator %s failed to respond in time", reactorID)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errorResp openapi.ErrorResponse
			if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil || errorResp.Message == nil {
				return fmt.Errorf("validator %s failed with status %d %s", reactorID, resp.StatusCode, resp.Status)
			}
			return fmt.Errorf("validator %s failed: %s", reactorID, *errorResp.Message)
		}

		// Check for validation error in response
		var response openapi.ValidationResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return fmt.Errorf("validator %s responded with invalid json: %v", reactorID, err)
		}

		if response.Reject != nil && response.Reject.Message != nil {
			return fmt.Errorf("validator %s rejected: %s", reactorID, *response.Reject.Message)
		}
	}
	return nil
}
