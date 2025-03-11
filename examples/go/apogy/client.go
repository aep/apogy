package apogy

import (
	openapi "github.com/aep/apogy/api/go"
)

type Mutation struct {
	Add interface{} `json:"add",omitempty`
	Sub interface{} `json:"sub",omitempty`
	Mul interface{} `json:"mul",omitempty`
	Div interface{} `json:"div",omitempty`
	Min interface{} `json:"min",omitempty`
	Max interface{} `json:"max",omitempty`
	Set interface{} `json:"set",omitempty`
}
type Mutations map[string]Mutation

type Document[Val any] struct {
	openapi.Document

	Id    string    `json:"id"`
	Model string    `json:"model"`
	Val   Val       `json:"val"`
	Mut   Mutations `json:"mut",omitempty`
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

	r.Book = &openapi.TypedClient[Book]{client, "com.example.Book"}

	return r, nil
}
