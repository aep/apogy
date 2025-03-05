package server

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	openapi "github.com/aep/apogy/api/go"
	"github.com/labstack/echo/v4"
	"github.com/vmihailenco/msgpack/v5"
)

// TODO cant accept msgpack before checking if https://github.com/vmihailenco/msgpack/issues/376 is real
func (s *server) PutDocument(c echo.Context) error {

	var doc = new(openapi.Document)
	if err := c.Bind(doc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if err := s.validateMeta(doc); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	var err error

	switch doc.Model {
	case "Model":
		if err := s.validateSchemaSchema(c.Request().Context(), doc); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("validation error: %s", err))
		}
	case "Reactor":
		if err := s.validateReactorSchema(c.Request().Context(), doc); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("validation error: %s", err))
		}
	default:
	}

	w2 := s.kv.Write()
	defer w2.Close()

	path, err := safeDBPath(doc.Model, doc.Id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	now := time.Now()
	doc.History = &openapi.History{
		Created: &now,
		Updated: &now,
	}

	var old *openapi.Document

	if doc.Version != nil {
		// Handle versioned updates
		bytes, err := w2.Get(c.Request().Context(), []byte(path))
		if err != nil {
			if !strings.Contains(err.Error(), "not exist") {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
			}
		} else if len(bytes) > 0 {
			old = new(openapi.Document)
			if err := msgpack.Unmarshal(bytes, old); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("unmarshal error: %v", err))
			}

			if err := s.deleteIndex(w2, old); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("index error: %v", err))
			}

			if old.History != nil {
				doc.History.Created = old.History.Created
			}

			if reflect.DeepEqual(old.Val, doc.Val) {
				return c.JSON(http.StatusOK, openapi.PutDocumentOK{
					Path: doc.Model + "/" + doc.Id,
				})
			}

			if old.Version != nil && doc.Version != nil {
				if *old.Version != *doc.Version {
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
			old = new(openapi.Document)
			if err := msgpack.Unmarshal(bytes, old); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("unmarshal error: %v", err))
			}

			if err := s.deleteIndex(w2, old); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("index error: %v", err))
			}

			if old.History != nil {
				doc.History.Created = old.History.Created
			}

			doc.Version = old.Version
		}
	}

	if doc.Version == nil {
		version := uint64(0)
		doc.Version = &version
	}
	*doc.Version++

	doc, err = s.ro.Validate(c.Request().Context(), old, doc)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	bytes, err := msgpack.Marshal(doc)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("marshal error: %v", err))
	}

	w2.Put([]byte(path), bytes)

	if err := s.createIndex(w2, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := w2.Commit(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
	}

	err = s.ro.Reconcile(c.Request().Context(), old, doc)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, openapi.PutDocumentOK{
		Path: doc.Model + "/" + doc.Id,
	})
}

func (s *server) GetDocument(c echo.Context, model string, id string) error {

	// fastpath
	if strings.Contains(c.Request().Header.Get("Accept"), "application/msgpack") && model != "Reactor" {
		path, err := safeDBPath(model, id)
		if err != nil {
			return err
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

		return c.Blob(http.StatusOK, "application/msgpack", bytes)
	}

	var doc openapi.Document
	err := s.getDocument(c.Request().Context(), model, id, &doc)
	if err != nil {
		return err
	}

	if model == "Reactor" {
		s.ro.Status(c.Request().Context(), &doc)
	}

	return c.JSON(http.StatusOK, doc)
}

func (s *server) getDocument(ctx context.Context, model string, id string, doc *openapi.Document) error {
	path, err := safeDBPath(model, id)
	if err != nil {
		return err
	}

	r := s.kv.Read()
	defer r.Close()

	bytes, err := r.Get(ctx, []byte(path))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if bytes == nil {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	if err := msgpack.Unmarshal(bytes, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "unmarshal error")
	}

	return nil
}

func (s *server) DeleteDocument(c echo.Context, model string, id string) error {
	path, err := safeDBPath(model, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	w := s.kv.Write()
	defer w.Close()

	// First get the document to remove its indexes
	bytes, err := w.Get(c.Request().Context(), []byte(path))
	if err != nil {
		if strings.Contains(err.Error(), "not exist") {
			return echo.NewHTTPError(http.StatusNotFound, "document not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
	}
	if bytes == nil {
		return echo.NewHTTPError(http.StatusNotFound, "document not found")
	}

	var doc = new(openapi.Document)
	if err := msgpack.Unmarshal(bytes, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "unmarshal error")
	}

	switch doc.Model {
	case "Model":
		err := s.checkNothingNeedsModel(c.Request().Context(), doc.Id)
		if err != nil {
			return err
		}
	case "Reactor":
	default:
		var schema openapi.Document
		err := s.getDocument(c.Request().Context(), "Model", doc.Model, &schema)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "cannot load model")
		}
	}

	_, err = s.ro.Validate(c.Request().Context(), doc, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Remove indexes first
	if err := s.deleteIndex(w, doc); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("index error: %v", err))
	}

	// Delete the document
	w.Del([]byte(path))

	if err := w.Commit(c.Request().Context()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
	}

	err = s.ro.Reconcile(c.Request().Context(), doc, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.NoContent(http.StatusOK)
}
