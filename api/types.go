package api

import (
	"time"
)

type History struct {
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

type Object struct {
	Id      string                 `json:"id"`
	Model   string                 `json:"model"`
	Version uint64                 `json:"version,omitempty" yaml:"version,omitempty"`
	History *History               `json:"history,omitempty" yaml:"history,omitempty"`
	Val     map[string]interface{} `json:"val,omitempty"`
}

type PutObjectRequest struct {
	Object Object `json:"object"`
}

type PutObjectResponse struct {
	Error string `json:"error,omitempty"`
	Path  string `json:"path,omitempty"`
}

type Filter struct {
	Key string `json:"k"`

	// set at most one
	Equal   any `json:"eq,omitempty"`
	Greater any `json:"gt,omitempty"`
	Less    any `json:"lt,omitempty"`
}

type SearchRequest struct {
	Model   string   `json:"model"`
	Filters []Filter `json:"filters,omitempty"`
	Cursor  string   `json:"cursor,omitempty"`
}

type Cursor struct {
	Keys   []string `json:"keys"`
	Cursor string   `json:"cursor"`
}
