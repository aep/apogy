package aql

import (
	"github.com/aep/apogy/api/go"
	"strings"
)

// ToSearchRequest converts an AQL Query to an openapi.SearchRequest
func (q *Query) ToSearchRequest() *openapi.SearchRequest {

	var full = true

	req := &openapi.SearchRequest{
		Model:  q.Type,
		Full:   &full,
		Cursor: q.Cursor,
	}

	// Map filters
	if len(q.Filter) > 0 {
		filters := make([]openapi.Filter, 0, len(q.Filter))
		for k, v := range q.Filter {
			// Check for operator in the key
			if strings.HasSuffix(k, "<") {
				baseKey := strings.TrimSuffix(k, "<")
				filter := openapi.Filter{
					Key:  baseKey,
					Less: &v,
				}
				filters = append(filters, filter)
			} else if strings.HasSuffix(k, ">") {
				baseKey := strings.TrimSuffix(k, ">")
				filter := openapi.Filter{
					Key:     baseKey,
					Greater: &v,
				}
				filters = append(filters, filter)
			} else if strings.HasSuffix(k, "^") {
				baseKey := strings.TrimSuffix(k, "^")
				filter := openapi.Filter{
					Key:    baseKey,
					Prefix: &v,
				}
				filters = append(filters, filter)
			} else {
				// Regular equals operator
				filter := openapi.Filter{
					Key: k,
				}
				if v != nil {
					filter.Equal = &v
				}
				filters = append(filters, filter)
			}
		}
		req.Filters = &filters
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

