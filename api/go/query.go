package apogy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"io"
	"iter"
	"net/http"
)

func (client *ClientWithResponses) Query(ctx context.Context, q string, args ...interface{}) iter.Seq2[*Document, error] {
	return query[Document](client.ClientInterface, ctx, q, args...)
}

func (client *ClientWithResponses) QueryOne(ctx context.Context, q string, args ...interface{}) (*Document, error) {
	return queryOne[Document](client.ClientInterface, ctx, q, args...)
}

type searchResponseT[Document any] struct {
	Cursor    *string    `json:"cursor,omitempty"`
	Documents []Document `json:"documents"`
	Error     *string    `json:"error,omitempty"`
}

func queryOne[Document any](client ClientInterface, ctx context.Context, q string, args ...interface{}) (*Document, error) {

	var limit = 1
	rsp, err := client.QueryDocuments(ctx, Query{
		Q:      q,
		Params: &args,
		Limit:  &limit,
	}, func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Accept", "application/jsonl")
		return nil
	})
	if err != nil {
		return nil, err
	}

	defer rsp.Body.Close()

	if rsp.StatusCode != 200 {
		return nil, parseError(rsp)
	}

	var searchResponse searchResponseT[Document]

	if rsp.Header.Get("Content-Type") == "application/jsonl" {

		err = json.NewDecoder(rsp.Body).Decode(&searchResponse)
		if err != nil {
			return nil, err
		}

	}

	if searchResponse.Error != nil {
		return nil, errors.New(*searchResponse.Error)
	}

	if len(searchResponse.Documents) == 0 {
		return nil, errors.New("not found")
	}
	return &searchResponse.Documents[0], nil
}

func query[Document any](client ClientInterface, ctx context.Context, q string, args ...interface{}) iter.Seq2[*Document, error] {

	return func(yield func(*Document, error) bool) {

		rsp, err := client.QueryDocuments(ctx, Query{
			Q:      q,
			Params: &args,
		}, func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Accept", "application/msgpack,application/jsonl")
			return nil
		})
		if err != nil {
			yield(nil, err)
			return
		}

		defer rsp.Body.Close()

		if rsp.StatusCode != 200 {
			yield(nil, parseError(rsp))
			return
		}

		if rsp.Header.Get("Content-Type") == "application/msgpack" {

			br := bufio.NewReader(rsp.Body)
			dec := msgpack.NewDecoder(br)

			for {
				var searchResponse searchResponseT[Document]
				err := dec.Decode(&searchResponse)
				if err != nil {
					if err == io.EOF {
						break
					}
					yield(nil, err)
					return
				}
				if searchResponse.Error != nil {
					yield(nil, errors.New(*searchResponse.Error))
					return
				}

				for _, doc := range searchResponse.Documents {
					if !yield(&doc, nil) {
						return
					}
				}
			}

		} else if rsp.Header.Get("Content-Type") == "application/jsonl" {

			scanner := bufio.NewScanner(rsp.Body)

			for scanner.Scan() {

				line := scanner.Text()
				if line == "" {
					continue
				}

				var searchResponse searchResponseT[Document]

				err := json.Unmarshal([]byte(line), &searchResponse)
				if err != nil {
					yield(nil, err)
					return
				}

				if searchResponse.Error != nil {
					yield(nil, errors.New(*searchResponse.Error))
					return
				}

				for _, doc := range searchResponse.Documents {
					if !yield(&doc, nil) {
						return
					}
				}
			}

			if scanner.Err() != nil {
				yield(nil, scanner.Err())
				return
			}

		}
	}
}
