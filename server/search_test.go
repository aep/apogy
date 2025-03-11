package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"encoding/json"
	openapi "github.com/aep/apogy/api/go"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func setupSearchTestData(t *testing.T, e *echo.Echo, s *server) {
	// Create test documents
	testDocs := []openapi.Document{
		{
			Model: "com.example.SearchTest",
			Id:    "doc1",
			Val: &map[string]interface{}{
				"name":  "Document 1",
				"type":  "test",
				"tags":  []string{"tag1", "tag2"},
				"count": 10,
			},
		},
		{
			Model: "com.example.SearchTest",
			Id:    "doc2",
			Val: &map[string]interface{}{
				"name":  "Document 2",
				"type":  "test",
				"tags":  []string{"tag2", "tag3"},
				"count": 20,
			},
		},
		{
			Model: "com.example.SearchTest",
			Id:    "doc3",
			Val: &map[string]interface{}{
				"name":  "Document 3",
				"type":  "production",
				"tags":  []string{"tag1", "tag3"},
				"count": 30,
			},
		},
	}

	// Create model first
	modelDoc := openapi.Document{
		Model: "Model",
		Id:    "com.example.SearchTest",
		Val: &map[string]interface{}{
			"name": "Search Test Model",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
				},
				"type": map[string]interface{}{
					"type": "string",
				},
				"tags": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"count": map[string]interface{}{
					"type": "integer",
				},
			},
		},
	}

	modelDocBytes, _ := json.Marshal(modelDoc)
	req := httptest.NewRequest(http.MethodPut, "/documents/Model/SearchTest", bytes.NewReader(modelDocBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	_ = s.PutDocument(c)

	// Create test documents
	for _, doc := range testDocs {
		docBytes, _ := json.Marshal(doc)
		req := httptest.NewRequest(http.MethodPut, "/documents/"+doc.Model+"/"+doc.Id, bytes.NewReader(docBytes))
		req.Header.Set(echo.HeaderContentType, "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		_ = s.PutDocument(c)
	}
}

func TestSearchDocuments_NoFilters(t *testing.T) {
	e, s := setupTestServer(t)
	setupSearchTestData(t, e, s)

	// Test search with no filters
	searchReq := openapi.SearchRequest{
		Model: "com.example.SearchTest",
	}

	reqBytes, _ := json.Marshal(searchReq)
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, s.SearchDocuments(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)

		var response openapi.SearchResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Documents)
		assert.Len(t, response.Documents, 3)
	}
}

func TestSearchDocuments_SingleFilter(t *testing.T) {
	e, s := setupTestServer(t)
	setupSearchTestData(t, e, s)

	// Test search with type=test filter
	equalValue := "test"
	var equalInterface interface{} = equalValue
	filters := []openapi.Filter{
		{
			Key:   "val.type",
			Equal: &equalInterface,
		},
	}

	searchReq := openapi.SearchRequest{
		Model:   "com.example.SearchTest",
		Filters: &filters,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, s.SearchDocuments(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)

		var response openapi.SearchResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Documents)
		assert.Len(t, response.Documents, 2)
	}
}

func TestSearchDocuments_MultipleFilters(t *testing.T) {
	e, s := setupTestServer(t)
	setupSearchTestData(t, e, s)

	// Test search with type=test AND name=Document 1
	typeValue := "test"
	var typeInterface interface{} = typeValue
	nameValue := "Document 1"
	var nameInterface interface{} = nameValue

	filters := []openapi.Filter{
		{
			Key:   "val.type",
			Equal: &typeInterface,
		},
		{
			Key:   "val.name",
			Equal: &nameInterface,
		},
	}

	searchReq := openapi.SearchRequest{
		Model:   "com.example.SearchTest",
		Filters: &filters,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, s.SearchDocuments(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)

		var response openapi.SearchResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Documents)
		assert.Len(t, response.Documents, 1)
		assert.Equal(t, "doc1", response.Documents[0].Id)
	}
}

func TestSearchDocuments_InvalidRequest(t *testing.T) {
	e, s := setupTestServer(t)

	// Test with missing model
	searchReq := openapi.SearchRequest{}
	reqBytes, _ := json.Marshal(searchReq)
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.SearchDocuments(c)
	if assert.Error(t, err) {
		he, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, he.Code)
		assert.Contains(t, he.Message, "Model is required")
	}

	// Test with invalid MessagePack
	req = httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader([]byte{0xc1, 0x2, 0x3})) // Invalid json data
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err = s.SearchDocuments(c)
	if assert.Error(t, err) {
		he, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, he.Code)
	}
}

func TestSearchDocuments_Cursor(t *testing.T) {
	e, s := setupTestServer(t)
	setupSearchTestData(t, e, s)

	var limit = 1
	searchReq := openapi.SearchRequest{
		Model: "com.example.SearchTest",
		Limit: &limit,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, s.SearchDocuments(c)) {
		var response openapi.SearchResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Cursor)

		// Search with cursor
		searchReq.Cursor = response.Cursor
		reqBytes, _ = json.Marshal(searchReq)
		req = httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
		req.Header.Set(echo.HeaderContentType, "application/json")
		rec = httptest.NewRecorder()
		c = e.NewContext(req, rec)

		if assert.NoError(t, s.SearchDocuments(c)) {
			var cursorResponse openapi.SearchResponse
			err = json.Unmarshal(rec.Body.Bytes(), &cursorResponse)
			assert.NoError(t, err)
			assert.NotNil(t, cursorResponse.Documents)
			// Verify that we get different results with cursor
			assert.NotEqual(t, response.Documents, cursorResponse.Documents)
		}
	}

}

func TestSearchDocuments_MultipleFiltersIdFirst(t *testing.T) {
	e, s := setupTestServer(t)
	setupSearchTestData(t, e, s)

	var idInterface interface{} = "doc1"
	nameValue := "Document 1"
	var nameInterface interface{} = nameValue

	filters := []openapi.Filter{
		{
			Key:   "id",
			Equal: &idInterface,
		},
		{
			Key:   "val.name",
			Equal: &nameInterface,
		},
	}

	searchReq := openapi.SearchRequest{
		Model:   "com.example.SearchTest",
		Filters: &filters,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, s.SearchDocuments(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)

		var response openapi.SearchResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Documents)
		assert.Len(t, response.Documents, 1)
		assert.Equal(t, "doc1", response.Documents[0].Id)
	}
}

func TestSearchDocuments_MultipleFiltersIdSecond(t *testing.T) {
	e, s := setupTestServer(t)
	setupSearchTestData(t, e, s)

	var idInterface interface{} = "doc1"
	nameValue := "Document 1"
	var nameInterface interface{} = nameValue

	filters := []openapi.Filter{
		{
			Key:   "val.name",
			Equal: &nameInterface,
		},
		{
			Key:   "id",
			Equal: &idInterface,
		},
	}

	searchReq := openapi.SearchRequest{
		Model:   "com.example.SearchTest",
		Filters: &filters,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req := httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if assert.NoError(t, s.SearchDocuments(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)

		var response openapi.SearchResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Documents)
		assert.Len(t, response.Documents, 1)
		assert.Equal(t, "doc1", response.Documents[0].Id)
	}
}
