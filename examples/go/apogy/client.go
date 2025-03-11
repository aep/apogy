package apogy

import (
	openapi "github.com/aep/apogy/api/go"
)

type Document[Val any] struct {
	openapi.Document
	Val Val `json:"val"`
}

type Book Document[BookVal]

type Client struct {
	openapi.ClientInterface

	Book *openapi.TypedClient[Book]
}

type ClientOption openapi.ClientOption

func NewClient(server string, opts ...ClientOption) (*Client, error) {

	var optss []openapi.ClientOption
	for _, o := range opts {
		optss = append(optss, openapi.ClientOption(o))
	}

	client, err := openapi.NewClient(server, optss...)
	if err != nil {
		return nil, err
	}

	r := &Client{ClientInterface: client}

	r.Book = &openapi.TypedClient[Book]{
		Client: client,
		Model:  "com.example.Book",
	}

	return r, nil
}
