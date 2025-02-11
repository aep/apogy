package server

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"apogy/api/go"
	"bytes"
	"encoding/json"
)

func (s *server) PutDocument(c echo.Context) error {

	var doc openapi.Document
	if err := c.Bind(&doc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if err := s.validateMeta(&doc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var schema *openapi.Document
	var err error

	switch doc.Model {
	case "Model":
		if err := s.validateSchemaSchema(c.Request().Context(), &doc); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("validation error: %s", err))
		}
	case "Reactor":
		if err := s.validateReactorSchema(c.Request().Context(), &doc); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("validation error: %s", err))
		}
	default:
		schema, err = s.validateObjectSchema(c.Request().Context(), &doc)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("validation error: %s", err))
		}
	}

	w2 := s.kv.Write()
	defer w2.Close()

	path, err := safeDBPath(doc.Model, doc.Id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Set history timestamps
	now := time.Now()
	doc.History = &openapi.History{
		Created: &now,
		Updated: &now,
	}

	if doc.Version != nil {
		// Handle versioned updates
		bytes, err := w2.Get(c.Request().Context(), []byte(path))
		if err != nil {
			if !strings.Contains(err.Error(), "not exist") {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
			}
		} else if len(bytes) > 0 {
			var original openapi.Document
			if err := json.Unmarshal(bytes, &original); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("unmarshal error: %v", err))
			}

			if err := s.deleteIndex(w2, &original); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("index error: %v", err))
			}

			if original.History != nil {
				doc.History.Created = original.History.Created
			}

			if reflect.DeepEqual(original.Val, doc.Val) {
				return c.JSON(http.StatusOK, openapi.PutDocumentOK{
					Path: doc.Model + "/" + doc.Id,
				})
			}

			if original.Version != nil && doc.Version != nil {
				if *original.Version != *doc.Version {
					return echo.NewHTTPError(http.StatusConflict, "version is out of date")
				}
			}
		}
	} else {
		// Handle non-versioned updates
		r := s.kv.Read()
		bytes, err := r.Get(c.Request().Context(), []byte(path))
		r.Close()
		if err != nil {
			if !strings.Contains(err.Error(), "not exist") {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
			}
		} else if len(bytes) > 0 {
			var original openapi.Document
			if err := json.Unmarshal(bytes, &original); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("unmarshal error: %v", err))
			}

			if err := s.deleteIndex(w2, &original); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("index error: %v", err))
			}

			if original.History != nil {
				doc.History.Created = original.History.Created
			}

			doc.Version = original.Version
		}
	}

	// Handle versioning
	if doc.Version == nil {
		version := uint64(0)
		doc.Version = &version
	}
	*doc.Version++

	// Special handling for Reactor type
	if doc.Model == "Reactor" {
		if err := s.ensureReactor(c.Request().Context(), &doc); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
	}

	// Marshal document
	bytes, err := json.Marshal(doc)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("marshal error: %v", err))
	}

	// Store document and update indices
	w2.Put([]byte(path), bytes)

	if err := s.createIndex(w2, &doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := w2.Commit(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
	}

	// Handle schema reconciliation
	if schema != nil {
		if err := s.reconcile(c.Request().Context(), schema, &doc); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	return c.JSON(http.StatusOK, openapi.PutDocumentOK{
		Path: doc.Model + "/" + doc.Id,
	})
}

func (s *server) GetDocument(c echo.Context, model string, id string) error {

	path, err := safeDBPath(model, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	r := s.kv.Read()
	defer r.Close()

	bytes, err := r.Get(c.Request().Context(), []byte(path))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if bytes == nil {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	var doc openapi.Document
	if err := json.Unmarshal(bytes, &doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "unmarshal error")
	}

	return c.JSON(http.StatusOK, doc)
}

// Helper functions

func (s *server) validateMeta(doc *openapi.Document) error {
	if doc.Model == "" || doc.Id == "" {
		return status.Error(codes.InvalidArgument, "model and id are required")
	}

	if bytes.Contains([]byte(doc.Model), []byte{0xff}) || bytes.Contains([]byte(doc.Id), []byte{0xff}) {
		return status.Error(codes.InvalidArgument, "invalid utf8 string in model or id")
	}

	return nil
}
