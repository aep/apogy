// Package openapi provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen version v1.16.3 DO NOT EDIT.
package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/oapi-codegen/runtime"
	strictecho "github.com/oapi-codegen/runtime/strictmiddleware/echo"
)

// Document defines model for Document.
type Document struct {
	History *History                `json:"history,omitempty"`
	Id      string                  `json:"id"`
	Model   string                  `json:"model"`
	Status  *map[string]interface{} `json:"status,omitempty"`
	Val     *map[string]interface{} `json:"val,omitempty"`
	Version *uint64                 `json:"version,omitempty"`
}

// Filter defines model for Filter.
type Filter struct {
	Equal   *interface{} `json:"equal,omitempty"`
	Greater *interface{} `json:"greater,omitempty"`
	Key     string       `json:"key"`
	Less    *interface{} `json:"less,omitempty"`
}

// History defines model for History.
type History struct {
	Created *time.Time `json:"created,omitempty"`
	Updated *time.Time `json:"updated,omitempty"`
}

// PutDocumentOK defines model for PutDocumentOK.
type PutDocumentOK struct {
	Path string `json:"path"`
}

// ReactorActivation defines model for ReactorActivation.
type ReactorActivation struct {
	Id      string `json:"id"`
	Model   string `json:"model"`
	Version uint64 `json:"version"`
}

// ReactorDone defines model for ReactorDone.
type ReactorDone = map[string]interface{}

// ReactorIn defines model for ReactorIn.
type ReactorIn struct {
	Done    *ReactorDone    `json:"done,omitempty"`
	Start   *ReactorStart   `json:"start,omitempty"`
	Working *ReactorWorking `json:"working,omitempty"`
}

// ReactorOut defines model for ReactorOut.
type ReactorOut struct {
	Activation *ReactorActivation `json:"activation,omitempty"`
}

// ReactorStart defines model for ReactorStart.
type ReactorStart struct {
	Id string `json:"id"`
}

// ReactorWorking defines model for ReactorWorking.
type ReactorWorking = map[string]interface{}

// SearchRequest defines model for SearchRequest.
type SearchRequest struct {
	Cursor  *string   `json:"cursor,omitempty"`
	Filters *[]Filter `json:"filters,omitempty"`
	Model   string    `json:"model"`
}

// SearchResponse defines model for SearchResponse.
type SearchResponse struct {
	Cursor *string   `json:"cursor,omitempty"`
	Ids    *[]string `json:"ids,omitempty"`
}

// PutDocumentJSONRequestBody defines body for PutDocument for application/json ContentType.
type PutDocumentJSONRequestBody = Document

// SearchDocumentsJSONRequestBody defines body for SearchDocuments for application/json ContentType.
type SearchDocumentsJSONRequestBody = SearchRequest

// ReactorLoopJSONRequestBody defines body for ReactorLoop for application/json ContentType.
type ReactorLoopJSONRequestBody = ReactorIn

// RequestEditorFn  is the function signature for the RequestEditor callback function
type RequestEditorFn func(ctx context.Context, req *http.Request) error

// Doer performs HTTP requests.
//
// The standard http.Client implements this interface.
type HttpRequestDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client which conforms to the OpenAPI3 specification for this service.
type Client struct {
	// The endpoint of the server conforming to this interface, with scheme,
	// https://api.deepmap.com for example. This can contain a path relative
	// to the server, such as https://api.deepmap.com/dev-test, and all the
	// paths in the swagger spec will be appended to the server.
	Server string

	// Doer for performing requests, typically a *http.Client with any
	// customized settings, such as certificate chains.
	Client HttpRequestDoer

	// A list of callbacks for modifying requests which are generated before sending over
	// the network.
	RequestEditors []RequestEditorFn
}

// ClientOption allows setting custom parameters during construction
type ClientOption func(*Client) error

// Creates a new Client, with reasonable defaults
func NewClient(server string, opts ...ClientOption) (*Client, error) {
	// create a client with sane default values
	client := Client{
		Server: server,
	}
	// mutate client and add all optional params
	for _, o := range opts {
		if err := o(&client); err != nil {
			return nil, err
		}
	}
	// ensure the server URL always has a trailing slash
	if !strings.HasSuffix(client.Server, "/") {
		client.Server += "/"
	}
	// create httpClient, if not already present
	if client.Client == nil {
		client.Client = &http.Client{}
	}
	return &client, nil
}

// WithHTTPClient allows overriding the default Doer, which is
// automatically created using http.Client. This is useful for tests.
func WithHTTPClient(doer HttpRequestDoer) ClientOption {
	return func(c *Client) error {
		c.Client = doer
		return nil
	}
}

// WithRequestEditorFn allows setting up a callback function, which will be
// called right before sending the request. This can be used to mutate the request.
func WithRequestEditorFn(fn RequestEditorFn) ClientOption {
	return func(c *Client) error {
		c.RequestEditors = append(c.RequestEditors, fn)
		return nil
	}
}

// The interface specification for the client above.
type ClientInterface interface {
	// PutDocumentWithBody request with any body
	PutDocumentWithBody(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*http.Response, error)

	PutDocument(ctx context.Context, body PutDocumentJSONRequestBody, reqEditors ...RequestEditorFn) (*http.Response, error)

	// SearchDocumentsWithBody request with any body
	SearchDocumentsWithBody(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*http.Response, error)

	SearchDocuments(ctx context.Context, body SearchDocumentsJSONRequestBody, reqEditors ...RequestEditorFn) (*http.Response, error)

	// ReactorLoopWithBody request with any body
	ReactorLoopWithBody(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*http.Response, error)

	ReactorLoop(ctx context.Context, body ReactorLoopJSONRequestBody, reqEditors ...RequestEditorFn) (*http.Response, error)

	// GetDocument request
	GetDocument(ctx context.Context, model string, id string, reqEditors ...RequestEditorFn) (*http.Response, error)
}

func (c *Client) PutDocumentWithBody(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewPutDocumentRequestWithBody(c.Server, contentType, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) PutDocument(ctx context.Context, body PutDocumentJSONRequestBody, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewPutDocumentRequest(c.Server, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) SearchDocumentsWithBody(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewSearchDocumentsRequestWithBody(c.Server, contentType, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) SearchDocuments(ctx context.Context, body SearchDocumentsJSONRequestBody, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewSearchDocumentsRequest(c.Server, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) ReactorLoopWithBody(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewReactorLoopRequestWithBody(c.Server, contentType, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) ReactorLoop(ctx context.Context, body ReactorLoopJSONRequestBody, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewReactorLoopRequest(c.Server, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

func (c *Client) GetDocument(ctx context.Context, model string, id string, reqEditors ...RequestEditorFn) (*http.Response, error) {
	req, err := NewGetDocumentRequest(c.Server, model, id)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	if err := c.applyEditors(ctx, req, reqEditors); err != nil {
		return nil, err
	}
	return c.Client.Do(req)
}

// NewPutDocumentRequest calls the generic PutDocument builder with application/json body
func NewPutDocumentRequest(server string, body PutDocumentJSONRequestBody) (*http.Request, error) {
	var bodyReader io.Reader
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	bodyReader = bytes.NewReader(buf)
	return NewPutDocumentRequestWithBody(server, "application/json", bodyReader)
}

// NewPutDocumentRequestWithBody generates requests for PutDocument with any type of body
func NewPutDocumentRequestWithBody(server string, contentType string, body io.Reader) (*http.Request, error) {
	var err error

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	operationPath := fmt.Sprintf("/v1")
	if operationPath[0] == '/' {
		operationPath = "." + operationPath
	}

	queryURL, err := serverURL.Parse(operationPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", queryURL.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", contentType)

	return req, nil
}

// NewSearchDocumentsRequest calls the generic SearchDocuments builder with application/json body
func NewSearchDocumentsRequest(server string, body SearchDocumentsJSONRequestBody) (*http.Request, error) {
	var bodyReader io.Reader
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	bodyReader = bytes.NewReader(buf)
	return NewSearchDocumentsRequestWithBody(server, "application/json", bodyReader)
}

// NewSearchDocumentsRequestWithBody generates requests for SearchDocuments with any type of body
func NewSearchDocumentsRequestWithBody(server string, contentType string, body io.Reader) (*http.Request, error) {
	var err error

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	operationPath := fmt.Sprintf("/v1/q")
	if operationPath[0] == '/' {
		operationPath = "." + operationPath
	}

	queryURL, err := serverURL.Parse(operationPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", queryURL.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", contentType)

	return req, nil
}

// NewReactorLoopRequest calls the generic ReactorLoop builder with application/json body
func NewReactorLoopRequest(server string, body ReactorLoopJSONRequestBody) (*http.Request, error) {
	var bodyReader io.Reader
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	bodyReader = bytes.NewReader(buf)
	return NewReactorLoopRequestWithBody(server, "application/json", bodyReader)
}

// NewReactorLoopRequestWithBody generates requests for ReactorLoop with any type of body
func NewReactorLoopRequestWithBody(server string, contentType string, body io.Reader) (*http.Request, error) {
	var err error

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	operationPath := fmt.Sprintf("/v1/reactor")
	if operationPath[0] == '/' {
		operationPath = "." + operationPath
	}

	queryURL, err := serverURL.Parse(operationPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", queryURL.String(), body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", contentType)

	return req, nil
}

// NewGetDocumentRequest generates requests for GetDocument
func NewGetDocumentRequest(server string, model string, id string) (*http.Request, error) {
	var err error

	var pathParam0 string

	pathParam0, err = runtime.StyleParamWithLocation("simple", false, "model", runtime.ParamLocationPath, model)
	if err != nil {
		return nil, err
	}

	var pathParam1 string

	pathParam1, err = runtime.StyleParamWithLocation("simple", false, "id", runtime.ParamLocationPath, id)
	if err != nil {
		return nil, err
	}

	serverURL, err := url.Parse(server)
	if err != nil {
		return nil, err
	}

	operationPath := fmt.Sprintf("/v1/%s/%s", pathParam0, pathParam1)
	if operationPath[0] == '/' {
		operationPath = "." + operationPath
	}

	queryURL, err := serverURL.Parse(operationPath)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", queryURL.String(), nil)
	if err != nil {
		return nil, err
	}

	return req, nil
}

func (c *Client) applyEditors(ctx context.Context, req *http.Request, additionalEditors []RequestEditorFn) error {
	for _, r := range c.RequestEditors {
		if err := r(ctx, req); err != nil {
			return err
		}
	}
	for _, r := range additionalEditors {
		if err := r(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// ClientWithResponses builds on ClientInterface to offer response payloads
type ClientWithResponses struct {
	ClientInterface
}

// NewClientWithResponses creates a new ClientWithResponses, which wraps
// Client with return type handling
func NewClientWithResponses(server string, opts ...ClientOption) (*ClientWithResponses, error) {
	client, err := NewClient(server, opts...)
	if err != nil {
		return nil, err
	}
	return &ClientWithResponses{client}, nil
}

// WithBaseURL overrides the baseURL.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) error {
		newBaseURL, err := url.Parse(baseURL)
		if err != nil {
			return err
		}
		c.Server = newBaseURL.String()
		return nil
	}
}

// ClientWithResponsesInterface is the interface specification for the client with responses above.
type ClientWithResponsesInterface interface {
	// PutDocumentWithBodyWithResponse request with any body
	PutDocumentWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*PutDocumentResponse, error)

	PutDocumentWithResponse(ctx context.Context, body PutDocumentJSONRequestBody, reqEditors ...RequestEditorFn) (*PutDocumentResponse, error)

	// SearchDocumentsWithBodyWithResponse request with any body
	SearchDocumentsWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*SearchDocumentsResponse, error)

	SearchDocumentsWithResponse(ctx context.Context, body SearchDocumentsJSONRequestBody, reqEditors ...RequestEditorFn) (*SearchDocumentsResponse, error)

	// ReactorLoopWithBodyWithResponse request with any body
	ReactorLoopWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*ReactorLoopResponse, error)

	ReactorLoopWithResponse(ctx context.Context, body ReactorLoopJSONRequestBody, reqEditors ...RequestEditorFn) (*ReactorLoopResponse, error)

	// GetDocumentWithResponse request
	GetDocumentWithResponse(ctx context.Context, model string, id string, reqEditors ...RequestEditorFn) (*GetDocumentResponse, error)
}

type PutDocumentResponse struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *PutDocumentOK
	JSON400      *struct {
		Message *string `json:"message,omitempty"`
	}
}

// Status returns HTTPResponse.Status
func (r PutDocumentResponse) Status() string {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.Status
	}
	return http.StatusText(0)
}

// StatusCode returns HTTPResponse.StatusCode
func (r PutDocumentResponse) StatusCode() int {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.StatusCode
	}
	return 0
}

type SearchDocumentsResponse struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *SearchResponse
}

// Status returns HTTPResponse.Status
func (r SearchDocumentsResponse) Status() string {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.Status
	}
	return http.StatusText(0)
}

// StatusCode returns HTTPResponse.StatusCode
func (r SearchDocumentsResponse) StatusCode() int {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.StatusCode
	}
	return 0
}

type ReactorLoopResponse struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *ReactorOut
}

// Status returns HTTPResponse.Status
func (r ReactorLoopResponse) Status() string {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.Status
	}
	return http.StatusText(0)
}

// StatusCode returns HTTPResponse.StatusCode
func (r ReactorLoopResponse) StatusCode() int {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.StatusCode
	}
	return 0
}

type GetDocumentResponse struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *Document
}

// Status returns HTTPResponse.Status
func (r GetDocumentResponse) Status() string {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.Status
	}
	return http.StatusText(0)
}

// StatusCode returns HTTPResponse.StatusCode
func (r GetDocumentResponse) StatusCode() int {
	if r.HTTPResponse != nil {
		return r.HTTPResponse.StatusCode
	}
	return 0
}

// PutDocumentWithBodyWithResponse request with arbitrary body returning *PutDocumentResponse
func (c *ClientWithResponses) PutDocumentWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*PutDocumentResponse, error) {
	rsp, err := c.PutDocumentWithBody(ctx, contentType, body, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParsePutDocumentResponse(rsp)
}

func (c *ClientWithResponses) PutDocumentWithResponse(ctx context.Context, body PutDocumentJSONRequestBody, reqEditors ...RequestEditorFn) (*PutDocumentResponse, error) {
	rsp, err := c.PutDocument(ctx, body, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParsePutDocumentResponse(rsp)
}

// SearchDocumentsWithBodyWithResponse request with arbitrary body returning *SearchDocumentsResponse
func (c *ClientWithResponses) SearchDocumentsWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*SearchDocumentsResponse, error) {
	rsp, err := c.SearchDocumentsWithBody(ctx, contentType, body, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseSearchDocumentsResponse(rsp)
}

func (c *ClientWithResponses) SearchDocumentsWithResponse(ctx context.Context, body SearchDocumentsJSONRequestBody, reqEditors ...RequestEditorFn) (*SearchDocumentsResponse, error) {
	rsp, err := c.SearchDocuments(ctx, body, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseSearchDocumentsResponse(rsp)
}

// ReactorLoopWithBodyWithResponse request with arbitrary body returning *ReactorLoopResponse
func (c *ClientWithResponses) ReactorLoopWithBodyWithResponse(ctx context.Context, contentType string, body io.Reader, reqEditors ...RequestEditorFn) (*ReactorLoopResponse, error) {
	rsp, err := c.ReactorLoopWithBody(ctx, contentType, body, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseReactorLoopResponse(rsp)
}

func (c *ClientWithResponses) ReactorLoopWithResponse(ctx context.Context, body ReactorLoopJSONRequestBody, reqEditors ...RequestEditorFn) (*ReactorLoopResponse, error) {
	rsp, err := c.ReactorLoop(ctx, body, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseReactorLoopResponse(rsp)
}

// GetDocumentWithResponse request returning *GetDocumentResponse
func (c *ClientWithResponses) GetDocumentWithResponse(ctx context.Context, model string, id string, reqEditors ...RequestEditorFn) (*GetDocumentResponse, error) {
	rsp, err := c.GetDocument(ctx, model, id, reqEditors...)
	if err != nil {
		return nil, err
	}
	return ParseGetDocumentResponse(rsp)
}

// ParsePutDocumentResponse parses an HTTP response from a PutDocumentWithResponse call
func ParsePutDocumentResponse(rsp *http.Response) (*PutDocumentResponse, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	response := &PutDocumentResponse{
		Body:         bodyBytes,
		HTTPResponse: rsp,
	}

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest PutDocumentOK
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON200 = &dest

	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 400:
		var dest struct {
			Message *string `json:"message,omitempty"`
		}
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON400 = &dest

	}

	return response, nil
}

// ParseSearchDocumentsResponse parses an HTTP response from a SearchDocumentsWithResponse call
func ParseSearchDocumentsResponse(rsp *http.Response) (*SearchDocumentsResponse, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	response := &SearchDocumentsResponse{
		Body:         bodyBytes,
		HTTPResponse: rsp,
	}

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest SearchResponse
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON200 = &dest

	}

	return response, nil
}

// ParseReactorLoopResponse parses an HTTP response from a ReactorLoopWithResponse call
func ParseReactorLoopResponse(rsp *http.Response) (*ReactorLoopResponse, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	response := &ReactorLoopResponse{
		Body:         bodyBytes,
		HTTPResponse: rsp,
	}

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest ReactorOut
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON200 = &dest

	}

	return response, nil
}

// ParseGetDocumentResponse parses an HTTP response from a GetDocumentWithResponse call
func ParseGetDocumentResponse(rsp *http.Response) (*GetDocumentResponse, error) {
	bodyBytes, err := io.ReadAll(rsp.Body)
	defer func() { _ = rsp.Body.Close() }()
	if err != nil {
		return nil, err
	}

	response := &GetDocumentResponse{
		Body:         bodyBytes,
		HTTPResponse: rsp,
	}

	switch {
	case strings.Contains(rsp.Header.Get("Content-Type"), "json") && rsp.StatusCode == 200:
		var dest Document
		if err := json.Unmarshal(bodyBytes, &dest); err != nil {
			return nil, err
		}
		response.JSON200 = &dest

	}

	return response, nil
}

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// Create or update a document
	// (POST /v1)
	PutDocument(ctx echo.Context) error
	// Search for documents
	// (POST /v1/q)
	SearchDocuments(ctx echo.Context) error
	// Bidirectional streaming for reactor operations
	// (GET /v1/reactor)
	ReactorLoop(ctx echo.Context) error
	// Get a document by model and ID
	// (GET /v1/{model}/{id})
	GetDocument(ctx echo.Context, model string, id string) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// PutDocument converts echo context to params.
func (w *ServerInterfaceWrapper) PutDocument(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshaled arguments
	err = w.Handler.PutDocument(ctx)
	return err
}

// SearchDocuments converts echo context to params.
func (w *ServerInterfaceWrapper) SearchDocuments(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshaled arguments
	err = w.Handler.SearchDocuments(ctx)
	return err
}

// ReactorLoop converts echo context to params.
func (w *ServerInterfaceWrapper) ReactorLoop(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshaled arguments
	err = w.Handler.ReactorLoop(ctx)
	return err
}

// GetDocument converts echo context to params.
func (w *ServerInterfaceWrapper) GetDocument(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "model" -------------
	var model string

	err = runtime.BindStyledParameterWithLocation("simple", false, "model", runtime.ParamLocationPath, ctx.Param("model"), &model)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter model: %s", err))
	}

	// ------------- Path parameter "id" -------------
	var id string

	err = runtime.BindStyledParameterWithLocation("simple", false, "id", runtime.ParamLocationPath, ctx.Param("id"), &id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter id: %s", err))
	}

	// Invoke the callback with all the unmarshaled arguments
	err = w.Handler.GetDocument(ctx, model, id)
	return err
}

// This is a simple interface which specifies echo.Route addition functions which
// are present on both echo.Echo and echo.Group, since we want to allow using
// either of them for path registration
type EchoRouter interface {
	CONNECT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	DELETE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	HEAD(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	OPTIONS(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PATCH(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	POST(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PUT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	TRACE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
}

// RegisterHandlers adds each server route to the EchoRouter.
func RegisterHandlers(router EchoRouter, si ServerInterface) {
	RegisterHandlersWithBaseURL(router, si, "")
}

// Registers handlers, and prepends BaseURL to the paths, so that the paths
// can be served under a prefix.
func RegisterHandlersWithBaseURL(router EchoRouter, si ServerInterface, baseURL string) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.POST(baseURL+"/v1", wrapper.PutDocument)
	router.POST(baseURL+"/v1/q", wrapper.SearchDocuments)
	router.GET(baseURL+"/v1/reactor", wrapper.ReactorLoop)
	router.GET(baseURL+"/v1/:model/:id", wrapper.GetDocument)

}

type PutDocumentRequestObject struct {
	Body *PutDocumentJSONRequestBody
}

type PutDocumentResponseObject interface {
	VisitPutDocumentResponse(w http.ResponseWriter) error
}

type PutDocument200JSONResponse PutDocumentOK

func (response PutDocument200JSONResponse) VisitPutDocumentResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	return json.NewEncoder(w).Encode(response)
}

type PutDocument400JSONResponse struct {
	Message *string `json:"message,omitempty"`
}

func (response PutDocument400JSONResponse) VisitPutDocumentResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(400)

	return json.NewEncoder(w).Encode(response)
}

type SearchDocumentsRequestObject struct {
	Body *SearchDocumentsJSONRequestBody
}

type SearchDocumentsResponseObject interface {
	VisitSearchDocumentsResponse(w http.ResponseWriter) error
}

type SearchDocuments200JSONResponse SearchResponse

func (response SearchDocuments200JSONResponse) VisitSearchDocumentsResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	return json.NewEncoder(w).Encode(response)
}

type ReactorLoopRequestObject struct {
	Body *ReactorLoopJSONRequestBody
}

type ReactorLoopResponseObject interface {
	VisitReactorLoopResponse(w http.ResponseWriter) error
}

type ReactorLoop200JSONResponse ReactorOut

func (response ReactorLoop200JSONResponse) VisitReactorLoopResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	return json.NewEncoder(w).Encode(response)
}

type GetDocumentRequestObject struct {
	Model string `json:"model"`
	Id    string `json:"id"`
}

type GetDocumentResponseObject interface {
	VisitGetDocumentResponse(w http.ResponseWriter) error
}

type GetDocument200JSONResponse Document

func (response GetDocument200JSONResponse) VisitGetDocumentResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	return json.NewEncoder(w).Encode(response)
}

// StrictServerInterface represents all server handlers.
type StrictServerInterface interface {
	// Create or update a document
	// (POST /v1)
	PutDocument(ctx context.Context, request PutDocumentRequestObject) (PutDocumentResponseObject, error)
	// Search for documents
	// (POST /v1/q)
	SearchDocuments(ctx context.Context, request SearchDocumentsRequestObject) (SearchDocumentsResponseObject, error)
	// Bidirectional streaming for reactor operations
	// (GET /v1/reactor)
	ReactorLoop(ctx context.Context, request ReactorLoopRequestObject) (ReactorLoopResponseObject, error)
	// Get a document by model and ID
	// (GET /v1/{model}/{id})
	GetDocument(ctx context.Context, request GetDocumentRequestObject) (GetDocumentResponseObject, error)
}

type StrictHandlerFunc = strictecho.StrictEchoHandlerFunc
type StrictMiddlewareFunc = strictecho.StrictEchoMiddlewareFunc

func NewStrictHandler(ssi StrictServerInterface, middlewares []StrictMiddlewareFunc) ServerInterface {
	return &strictHandler{ssi: ssi, middlewares: middlewares}
}

type strictHandler struct {
	ssi         StrictServerInterface
	middlewares []StrictMiddlewareFunc
}

// PutDocument operation middleware
func (sh *strictHandler) PutDocument(ctx echo.Context) error {
	var request PutDocumentRequestObject

	var body PutDocumentJSONRequestBody
	if err := ctx.Bind(&body); err != nil {
		return err
	}
	request.Body = &body

	handler := func(ctx echo.Context, request interface{}) (interface{}, error) {
		return sh.ssi.PutDocument(ctx.Request().Context(), request.(PutDocumentRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "PutDocument")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(PutDocumentResponseObject); ok {
		return validResponse.VisitPutDocumentResponse(ctx.Response())
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// SearchDocuments operation middleware
func (sh *strictHandler) SearchDocuments(ctx echo.Context) error {
	var request SearchDocumentsRequestObject

	var body SearchDocumentsJSONRequestBody
	if err := ctx.Bind(&body); err != nil {
		return err
	}
	request.Body = &body

	handler := func(ctx echo.Context, request interface{}) (interface{}, error) {
		return sh.ssi.SearchDocuments(ctx.Request().Context(), request.(SearchDocumentsRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "SearchDocuments")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(SearchDocumentsResponseObject); ok {
		return validResponse.VisitSearchDocumentsResponse(ctx.Response())
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// ReactorLoop operation middleware
func (sh *strictHandler) ReactorLoop(ctx echo.Context) error {
	var request ReactorLoopRequestObject

	var body ReactorLoopJSONRequestBody
	if err := ctx.Bind(&body); err != nil {
		return err
	}
	request.Body = &body

	handler := func(ctx echo.Context, request interface{}) (interface{}, error) {
		return sh.ssi.ReactorLoop(ctx.Request().Context(), request.(ReactorLoopRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "ReactorLoop")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(ReactorLoopResponseObject); ok {
		return validResponse.VisitReactorLoopResponse(ctx.Response())
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}

// GetDocument operation middleware
func (sh *strictHandler) GetDocument(ctx echo.Context, model string, id string) error {
	var request GetDocumentRequestObject

	request.Model = model
	request.Id = id

	handler := func(ctx echo.Context, request interface{}) (interface{}, error) {
		return sh.ssi.GetDocument(ctx.Request().Context(), request.(GetDocumentRequestObject))
	}
	for _, middleware := range sh.middlewares {
		handler = middleware(handler, "GetDocument")
	}

	response, err := handler(ctx, request)

	if err != nil {
		return err
	} else if validResponse, ok := response.(GetDocumentResponseObject); ok {
		return validResponse.VisitGetDocumentResponse(ctx.Response())
	} else if response != nil {
		return fmt.Errorf("unexpected response type: %T", response)
	}
	return nil
}
