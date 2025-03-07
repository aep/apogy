package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"bytes"
	"encoding/json"
	openapi "github.com/aep/apogy/api/go"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func setupIndexTestData(t *testing.T, e *echo.Echo, s *server) *Model {
	// Create a model with indexes
	modelDoc := openapi.Document{
		Model: "Model",
		Id:    "com.example.IndexTest",
		Val: &map[string]interface{}{
			"name": "Index Test Model",
			"schema": map[string]interface{}{
				"name":  "string",
				"code":  "string",
				"price": "uint64",
				"tags":  []string{"string"},
			},
			"index": map[string]interface{}{
				"code": "unique",
			},
		},
	}

	modelDocBytes, _ := json.Marshal(modelDoc)
	req := httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(modelDocBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := s.PutDocument(c)
	if err != nil {
		panic(err)
	}

	// Get the model from the server
	model, err := s.getModel(context.Background(), "com.example.IndexTest")
	assert.NoError(t, err)
	assert.NotNil(t, model)
	assert.Equal(t, "unique", model.Index["val.code"])

	return model
}

func TestCreateIndex(t *testing.T) {
	e, s := setupTestServer(t)
	setupIndexTestData(t, e, s)

	// Create a document to test indexing
	testDoc := openapi.Document{
		Model: "com.example.IndexTest",
		Id:    "doc1",
		Val: &map[string]interface{}{
			"name":  "Test Document",
			"code":  "ABC123",
			"count": 10,
			"price": 1999,
			"tags":  []string{"test", "index"},
		},
	}

	docBytes, _ := json.Marshal(testDoc)
	req := httptest.NewRequest(http.MethodPut, "/documents/com.example.IndexTest/doc1", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := s.PutDocument(c)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Now search for the document by indexed field
	codeValue := "ABC123"
	var codeInterface interface{} = codeValue
	filters := []openapi.Filter{
		{
			Key:   "val.code",
			Equal: &codeInterface,
		},
	}

	searchReq := openapi.SearchRequest{
		Model:   "com.example.IndexTest",
		Filters: &filters,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req = httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

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

func TestUniqueIndexConstraint(t *testing.T) {
	e, s := setupTestServer(t)
	_ = setupIndexTestData(t, e, s)

	// Create first document with unique code
	doc1 := openapi.Document{
		Model: "com.example.IndexTest",
		Id:    "doc1",
		Val: &map[string]interface{}{
			"name":  "Document 1",
			"code":  "UNIQUE100",
			"count": 10,
		},
	}

	docBytes, _ := json.Marshal(doc1)
	req := httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := s.PutDocument(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Try to create second document with same unique code
	doc2 := openapi.Document{
		Model: "com.example.IndexTest",
		Id:    "doc2",
		Val: &map[string]interface{}{
			"name":  "Document 2",
			"code":  "UNIQUE100", // Same code as doc1
			"count": 20,
		},
	}

	docBytes, _ = json.Marshal(doc2)
	req = httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	err = s.PutDocument(c)

	// Should get an error for unique constraint violation
	assert.Error(t, err)
	he, ok := err.(*echo.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusConflict, he.Code)
	assert.Contains(t, he.Message, "unique index")
}

func TestDeleteIndex(t *testing.T) {
	e, s := setupTestServer(t)
	_ = setupIndexTestData(t, e, s)

	// Create a document first
	testDoc := openapi.Document{
		Model: "com.example.IndexTest",
		Id:    "doc1",
		Val: &map[string]interface{}{
			"name":  "Test Document",
			"code":  "DELETE123",
			"count": 15,
		},
	}

	docBytes, _ := json.Marshal(testDoc)
	req := httptest.NewRequest(http.MethodPut, "/documents/com.example.IndexTest/doc1", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := s.PutDocument(c)
	assert.NoError(t, err)

	// Verify it exists by searching
	codeValue := "DELETE123"
	var codeInterface interface{} = codeValue
	filters := []openapi.Filter{
		{
			Key:   "val.code",
			Equal: &codeInterface,
		},
	}

	searchReq := openapi.SearchRequest{
		Model:   "com.example.IndexTest",
		Filters: &filters,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req = httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	err = s.SearchDocuments(c)
	assert.NoError(t, err)

	var response openapi.SearchResponse
	json.Unmarshal(rec.Body.Bytes(), &response)
	assert.Len(t, response.Documents, 1)

	// Now delete the document
	req = httptest.NewRequest(http.MethodDelete, "/v1/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	err = s.DeleteDocument(c, "com.example.IndexTest", "doc1")
	assert.NoError(t, err)

	// Search again, should not find anything
	req = httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	err = s.SearchDocuments(c)
	assert.NoError(t, err)

	json.Unmarshal(rec.Body.Bytes(), &response)
	assert.Len(t, response.Documents, 0)
}

func TestNumericIndexing(t *testing.T) {
	e, s := setupTestServer(t)
	_ = setupIndexTestData(t, e, s)

	// Create a document with a numeric field to test indexing
	testDoc := openapi.Document{
		Model: "com.example.IndexTest",
		Id:    "doc1",
		Val: &map[string]interface{}{
			"name":  "Numeric Test",
			"code":  "NUM123",
			"count": 50,
			"price": 2999,
		},
	}

	docBytes, _ := json.Marshal(testDoc)
	req := httptest.NewRequest(http.MethodPut, "/documents/com.example.IndexTest/doc1", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := s.PutDocument(c)
	assert.NoError(t, err)

	// Search by numeric field
	var priceValue uint64 = 2999
	var priceInterface interface{} = priceValue
	filters := []openapi.Filter{
		{
			Key:   "val.price",
			Equal: &priceInterface,
		},
	}

	searchReq := openapi.SearchRequest{
		Model:   "com.example.IndexTest",
		Filters: &filters,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req = httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	if assert.NoError(t, s.SearchDocuments(c)) {
		var response openapi.SearchResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Documents)
		assert.Len(t, response.Documents, 1)
		assert.Equal(t, "doc1", response.Documents[0].Id)
	}
}

/*

this doesnt work because i cant get golangs json to send invalid utf8

func TestIndexWithInvalidCharacters(t *testing.T) {
	e, s := setupTestServer(t)
	_ = setupIndexTestData(t, e, s)

	// Create a document with a field containing invalid characters
	testDoc := openapi.Document{
		Model: "com.example.IndexTest",
		Id:    "doc1",
		Val: &map[string]interface{}{
			"name": "Valid Document",
			"code": unsafe.String(unsafe.SliceData([]byte{0xFF, 0x00, 0x01}), 3),
		},
	}

	docBytes, _ := json.Marshal(testDoc)
	req := httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Should get an error since 0xFF is not allowed in indexed fields
	err := s.PutDocument(c)
	assert.Error(t, err)
	he, ok := err.(*echo.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusBadRequest, he.Code)
}

*/

func TestArrayValueIndexing(t *testing.T) {
	e, s := setupTestServer(t)
	_ = setupIndexTestData(t, e, s)

	// Create a document with an array field
	testDoc := openapi.Document{
		Model: "com.example.IndexTest",
		Id:    "doc1",
		Val: &map[string]interface{}{
			"name": "Array Test",
			"code": "ARR123",
			"tags": []string{"tag1", "tag2", "tag3"},
		},
	}

	docBytes, _ := json.Marshal(testDoc)
	req := httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := s.PutDocument(c)
	assert.NoError(t, err)

	// Search by one of the array values
	tagValue := "tag2"
	var tagInterface interface{} = tagValue
	filters := []openapi.Filter{
		{
			Key:   "val.tags",
			Equal: &tagInterface,
		},
	}

	searchReq := openapi.SearchRequest{
		Model:   "com.example.IndexTest",
		Filters: &filters,
	}

	reqBytes, _ := json.Marshal(searchReq)
	req = httptest.NewRequest(http.MethodPost, "/search", bytes.NewReader(reqBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	if assert.NoError(t, s.SearchDocuments(c)) {
		var response openapi.SearchResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.NotNil(t, response.Documents)
		assert.Len(t, response.Documents, 1)
		assert.Equal(t, "doc1", response.Documents[0].Id)
	}
}
