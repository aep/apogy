package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"encoding/json"
	openapi "github.com/aep/apogy/api/go"
	"github.com/aep/apogy/bus"
	"github.com/aep/apogy/kv"
	"github.com/aep/apogy/reactor"
	"github.com/labstack/echo/v4"
	"github.com/maypok86/otter"
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

	cache, err := otter.MustBuilder[string, *Model](100000).
		WithTTL(60 * time.Second).
		Build()

	if err != nil {
		panic(err)
	}

	s := &server{
		kv:         kv,
		bs:         bs,
		modelCache: cache,
		ro:         reactor.NewReactor("", "", ""),
	}
	e := echo.New()
	e.Binder = &Binder{
		defaultBinder: &echo.DefaultBinder{},
	}

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
		},
	}

	modelDocBytes, err := json.Marshal(modelDoc)
	if err != nil {
		t.Fatalf("Failed to marshal model document: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/documents/Model/Test.com.example", bytes.NewReader(modelDocBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
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
		},
	}

	// Convert document to MessagePack
	docBytes, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("Failed to marshal test document: %v", err)
	}

	// Create a new request for PUT
	req := httptest.NewRequest(http.MethodPut, "/documents/Model/bob.example.com", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Test the PUT handler
	if assert.NoError(t, s.PutDocument(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)

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
			"runtime":   "http",
			"validator": "https://google.com",
			"name":      "Test Reactor",
			"type":      "test",
		},
	}

	docBytes, err := json.Marshal(doc)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/documents/Reactor/asdasd.example.com", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.NoError(t, s.PutDocument(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPutDocument_InvalidFormat(t *testing.T) {
	e, s := setupTestServer(t)

	// Test with invalid MessagePack
	invalidMsgPack := []byte{0xc1, 0x2, 0x3} // Invalid json data
	req := httptest.NewRequest(http.MethodPut, "/documents/Invalid/test", bytes.NewReader(invalidMsgPack))
	req.Header.Set(echo.HeaderContentType, "application/json")
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

	docBytes, _ := json.Marshal(doc)
	req := httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/version-test", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.NoError(t, s.PutDocument(c))

	// Try to update with wrong version
	doc.Val = &map[string]interface{}{"data": "updated"}
	docBytes, _ = json.Marshal(doc)
	req = httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/version-test", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
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
	docBytes, _ := json.Marshal(initialDoc)
	req := httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/parallel-test", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
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

			docBytes, _ := json.Marshal(updateDoc)
			req := httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/parallel-test", bytes.NewReader(docBytes))
			req.Header.Set(echo.HeaderContentType, "application/json")
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
	docBytes, _ := json.Marshal(doc)
	req := httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/update-test", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.NoError(t, s.PutDocument(c))

	// Update document
	doc.Val = &map[string]interface{}{"data": "updated"}
	docBytes, _ = json.Marshal(doc)
	req = httptest.NewRequest(http.MethodPut, "/documents/Test.com.example/update-test", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
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

	// Convert the entire Val to JSON and back to a map to handle various potential types
	var valMap map[string]interface{}
	valBytes, err := json.Marshal(storedDoc.Val)
	assert.NoError(t, err, "Failed to marshal storedDoc.Val to JSON")

	err = json.Unmarshal(valBytes, &valMap)
	assert.NoError(t, err, "Failed to unmarshal JSON to map")

	// Now we can safely access the map
	assert.Equal(t, "updated", valMap["data"])
	assert.NotNil(t, storedDoc.Version)
	assert.Equal(t, uint64(2), *storedDoc.Version)
}

func TestConcurrentMutations_NeverFail(t *testing.T) {
	e, s := setupTestServer(t)

	// Create a document with a counter
	docId := "concurrent-mutations-test"
	initialVal := map[string]interface{}{
		"counter": json.Number("0"),
	}
	initialDoc := openapi.Document{
		Model: "Test.com.example",
		Id:    docId,
		Val:   initialVal,
	}

	// First PUT to create the document
	docBytes, _ := json.Marshal(initialDoc)
	req := httptest.NewRequest(http.MethodPost, "/v1/", bytes.NewReader(docBytes))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.NoError(t, s.PutDocument(c))

	// Number of concurrent mutations to perform
	concurrentCount := 50

	// Launch concurrent mutations, all incrementing the counter
	var wg sync.WaitGroup
	for i := 0; i < concurrentCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			vv := interface{}(json.Number("1"))
			mutationDoc := openapi.Document{
				Model: "Test.com.example",
				Id:    docId,
				Mut: &openapi.Mutations{
					"counter": {
						Add: &vv,
					},
				},
			}

			mutBytes, _ := json.Marshal(mutationDoc)
			reqMut := httptest.NewRequest(http.MethodPost, "/v1", bytes.NewReader(mutBytes))
			reqMut.Header.Set(echo.HeaderContentType, "application/json")
			recMut := httptest.NewRecorder()
			cMut := e.NewContext(reqMut, recMut)

			// This should never fail due to retry mechanism in PutDocument
			err := s.PutDocument(cMut)
			assert.NoError(t, err, "Concurrent mutation should not fail")
		}()
	}

	wg.Wait()

	// Check final value
	getReq := httptest.NewRequest(http.MethodGet, "/documents/Test.com.example/"+docId, nil)
	getRec := httptest.NewRecorder()
	getContext := e.NewContext(getReq, getRec)

	err := s.GetDocument(getContext, "Test.com.example", docId)
	assert.NoError(t, err)

	var finalDoc openapi.Document
	err = json.Unmarshal(getRec.Body.Bytes(), &finalDoc)
	assert.NoError(t, err)

	// Extract the counter value
	// Convert the Val to map[string]interface{} using JSON marshal/unmarshal
	var valMap map[string]interface{}
	valBytes, err := json.Marshal(finalDoc.Val)
	assert.NoError(t, err, "Failed to marshal finalDoc.Val to JSON")

	err = json.Unmarshal(valBytes, &valMap)
	assert.NoError(t, err, "Failed to unmarshal JSON to map")

	counter, ok := valMap["counter"].(json.Number)
	if !ok {
		// Try to convert it to a json.Number if it's a different numeric type
		counterFloat, isFloat := valMap["counter"].(float64)
		if isFloat {
			counter = json.Number(fmt.Sprintf("%v", counterFloat))
		} else {
			t.Fatalf("Counter value is not a number: %T", valMap["counter"])
		}
	}

	// Check that all increments were applied
	counterInt, err := counter.Int64()
	assert.NoError(t, err)
	assert.Equal(t, int64(concurrentCount), counterInt, "All mutations should have been applied")
}
