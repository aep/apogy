package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aep/apogy/api/go"
	"github.com/aep/apogy/bus"
	"github.com/aep/apogy/kv"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func setupTestServer(t *testing.T) (*echo.Echo, *server) {
	kv, err := kv.NewTikv()
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	bs, err := bus.NewSolo()
	if err != nil {
		t.Fatalf("Failed to create test bus: %v", err)
	}

	s := newServer(kv, bs)
	e := echo.New()

	// Clean up test data using the Delete function
	testDocuments := []struct {
		model string
		id    string
	}{
		{"Model", "Test.com.example"},
		{"Model", "bob.example.com"},
		{"Reactor", "asdasd.example.com"},
		{"Test.com.example", "version-test"},
		{"Test.com.example", "parallel-test"},
		{"Test.com.example", "update-test"},
	}

	for _, doc := range testDocuments {
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		_ = s.DeleteDocument(c, doc.model, doc.id)
	}

	// Create the Test.com.example model
	modelDoc := openapi.Document{
		Model: "Model",
		Id:    "Test.com.example",
		Val: &map[string]interface{}{
			"name": "Test Model",
			"properties": map[string]interface{}{
				"data": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}

	jsonDoc, err := json.Marshal(modelDoc)
	if err != nil {
		t.Fatalf("Failed to marshal model document: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/documents/Model/Test.com.example", bytes.NewReader(jsonDoc))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.PutDocument(c); err != nil {
		t.Fatalf("Failed to create test model: %v", err)
	}

	return e, s
}

func TestPutDocument_Model(t *testing.T) {
	e, s := setupTestServer(t)

	// Create a test Model document
	doc := openapi.Document{
		Model: "Model",
		Id:    "bob.example.com",
		Val: &map[string]interface{}{
			"name": "Test Model",
			"properties": map[string]interface{}{
				"testField": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}

	// Convert document to JSON
	jsonDoc, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Failed to marshal test document: %v", err)
	}

	// Create a new request for PUT
	req := httptest.NewRequest(http.MethodPut, "/documents/Model/bob.example.com", bytes.NewReader(jsonDoc))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Test the PUT handler
	if assert.NoError(t, s.PutDocument(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)

		var response openapi.PutDocumentOK
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "Model/bob.example.com", response.Path)

		// Create a new request for GET to verify the document
		getReq := httptest.NewRequest(http.MethodGet, "/documents/Model/bob.example.com", nil)
		getRec := httptest.NewRecorder()
		getContext := e.NewContext(getReq, getRec)

		// Test the GET handler
		err = s.GetDocument(getContext, "Model", "bob.example.com")
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, getRec.Code)

		var storedDoc openapi.Document
		err = json.Unmarshal(getRec.Body.Bytes(), &storedDoc)
		assert.NoError(t, err)

		// Verify the stored document
		assert.Equal(t, doc.Model, storedDoc.Model)
		assert.Equal(t, doc.Id, storedDoc.Id)
		assert.NotNil(t, storedDoc.Version)
		assert.Equal(t, uint64(1), *storedDoc.Version)
		assert.NotNil(t, storedDoc.History)
		assert.NotNil(t, storedDoc.History.Created)
		assert.NotNil(t, storedDoc.History.Updated)
	}
}

func TestPutDocument_Reactor(t *testing.T) {
	e, s := setupTestServer(t)

	// Create a test Reactor document
	doc := openapi.Document{
		Model: "Reactor",
		Id:    "asdasd.example.com",
		Val: &map[string]interface{}{
			"name": "Test Reactor",
			"type": "test",
		},
	}

	jsonDoc, err := json.Marshal(doc)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/documents/Reactor/asdasd.example.com", bytes.NewReader(jsonDoc))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.NoError(t, s.PutDocument(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPutDocument_InvalidFormat(t *testing.T) {
	e, s := setupTestServer(t)

	// Test with invalid JSON
	invalidJSON := []byte(`{"model": "Invalid"`)
	req := httptest.NewRequest(http.MethodPut, "/documents/Invalid/test", bytes.NewReader(invalidJSON))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.PutDocument(c)
	if assert.Error(t, err) {
		he, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, he.Code)
	}
}

func TestPutDocument_VersionConflict(t *testing.T) {
	e, s := setupTestServer(t)

	// Create initial document
	version := uint64(1)
	doc := openapi.Document{
		Model:   "Test.com.example",
		Id:      "version-test",
		Version: &version,
		Val: &map[string]interface{}{
			"data": "initial",
		},
	}

	jsonDoc, _ := json.Marshal(doc)
	req := httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/version-test", bytes.NewReader(jsonDoc))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.NoError(t, s.PutDocument(c))

	// Try to update with wrong version
	doc.Val = &map[string]interface{}{"data": "updated"}
	jsonDoc, _ = json.Marshal(doc)
	req = httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/version-test", bytes.NewReader(jsonDoc))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	err := s.PutDocument(c)
	if assert.Error(t, err) {
		he, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusConflict, he.Code)
	}
}

func TestPutDocument_ParallelVersionConflict(t *testing.T) {
	e, s := setupTestServer(t)

	// Create initial document
	initialDoc := openapi.Document{
		Model: "Test.com.example",
		Id:    "parallel-test",
		Val: &map[string]interface{}{
			"data": "initial",
		},
	}

	// First PUT to create the document
	jsonDoc, _ := json.Marshal(initialDoc)
	req := httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/parallel-test", bytes.NewReader(jsonDoc))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.NoError(t, s.PutDocument(c))

	// Get the document to get its version
	getReq := httptest.NewRequest(http.MethodGet, "/documents/Test.com.example/parallel-test", nil)
	getRec := httptest.NewRecorder()
	getContext := e.NewContext(getReq, getRec)

	err := s.GetDocument(getContext, "Test.com.example", "parallel-test")
	assert.NoError(t, err)

	var storedDoc openapi.Document
	err = json.Unmarshal(getRec.Body.Bytes(), &storedDoc)
	assert.NoError(t, err)

	// Prepare for parallel updates
	var wg sync.WaitGroup
	successCount := 0
	var successMutex sync.Mutex

	// Launch 10 parallel updates
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			updateDoc := storedDoc
			updateDoc.Val = &map[string]interface{}{
				"data": "updated",
				"by":   index,
			}

			jsonDoc, _ := json.Marshal(updateDoc)
			req := httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/parallel-test", bytes.NewReader(jsonDoc))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := s.PutDocument(c)
			if err == nil {
				successMutex.Lock()
				successCount++
				successMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// Verify that only one update succeeded
	assert.Equal(t, 1, successCount, "Only one concurrent update should succeed")

	// Verify final version is exactly one more than initial
	getReq = httptest.NewRequest(http.MethodGet, "/documents/Test.com.example/parallel-test", nil)
	getRec = httptest.NewRecorder()
	getContext = e.NewContext(getReq, getRec)

	err = s.GetDocument(getContext, "Test.com.example", "parallel-test")
	assert.NoError(t, err)

	var finalDoc openapi.Document
	err = json.Unmarshal(getRec.Body.Bytes(), &finalDoc)
	assert.NoError(t, err)
	assert.Equal(t, *storedDoc.Version+1, *finalDoc.Version, "Document version should be incremented exactly once")
}

func TestGetDocument_NotFound(t *testing.T) {
	e, s := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/documents/NonExistent/id", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := s.GetDocument(c, "NonExistent", "id")
	if assert.Error(t, err) {
		he, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusNotFound, he.Code)
	}
}

func TestPutDocument_Update(t *testing.T) {
	e, s := setupTestServer(t)

	// Create initial document
	doc := openapi.Document{
		Model: "Test.com.example",
		Id:    "update-test",
		Val: &map[string]interface{}{
			"data": "initial",
		},
	}

	// First PUT
	jsonDoc, _ := json.Marshal(doc)
	req := httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/update-test", bytes.NewReader(jsonDoc))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.NoError(t, s.PutDocument(c))

	// Update document
	doc.Val = &map[string]interface{}{"data": "updated"}
	jsonDoc, _ = json.Marshal(doc)
	req = httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/update-test", bytes.NewReader(jsonDoc))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)

	// Test update
	assert.NoError(t, s.PutDocument(c))

	// Verify updated content
	getReq := httptest.NewRequest(http.MethodGet, "/documents/Test.com.example/update-test", nil)
	getRec := httptest.NewRecorder()
	getContext := e.NewContext(getReq, getRec)

	err := s.GetDocument(getContext, "Test.com.example", "update-test")
	assert.NoError(t, err)

	var storedDoc openapi.Document
	err = json.Unmarshal(getRec.Body.Bytes(), &storedDoc)
	assert.NoError(t, err)

	assert.Equal(t, "updated", (*storedDoc.Val)["data"])
	assert.NotNil(t, storedDoc.Version)
	assert.Equal(t, uint64(2), *storedDoc.Version)
}
