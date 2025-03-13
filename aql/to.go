package aql

import (
	openapi "github.com/aep/apogy/api/go"
)

// ToSearchRequest converts an AQL Query to an openapi.SearchRequest
func (q *Query) ToSearchRequest() *openapi.SearchRequest {

	var full = true

	req := &openapi.SearchRequest{
		Model: q.Type,
		Full:  &full,
	}

	// Use filters directly
	if len(q.Filter) > 0 {
		req.Filters = &q.Filter
	}

	// Map nested/linked queries
	if len(q.Links) > 0 {
		links := make([]openapi.SearchRequest, 0, len(q.Links))
		for _, link := range q.Links {
			if sr := link.ToSearchRequest(); sr != nil {
				links = append(links, *sr)
			}
		}
		if len(links) > 0 {
			req.Links = &links
		}
	}

	return req
}
