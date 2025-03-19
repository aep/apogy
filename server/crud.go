package server

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"time"

	openapi "github.com/aep/apogy/api/go"
	"github.com/aep/apogy/kv"
	"github.com/labstack/echo/v4"
	tikerr "github.com/tikv/client-go/v2/error"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (s *server) PutDocument(c echo.Context) error {
	ctx, span := tracer.Start(c.Request().Context(), "PutDocument")
	defer span.End()

	var doc = new(openapi.Document)
	if err := c.Bind(doc); err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	span.SetAttributes(
		attribute.String("model", doc.Model),
		attribute.String("id", doc.Id),
	)

	err := s.validateMeta(doc)

	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	switch doc.Model {
	case "Model":
		err = s.validateSchemaSchema(ctx, doc)
		if err != nil {
			span.RecordError(err)
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("validation error: %s", err))
		}
	case "Reactor":
		err = s.validateReactorSchema(ctx, doc)
		if err != nil {
			span.RecordError(err)
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("validation error: %s", err))
		}
	default:
	}

	model, err := s.getModel(ctx, doc.Model)

	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	path, err := safeDBPath(doc.Model, doc.Id)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// for contested keys, use pessimistic locking
	var hotKeys = [][]byte{}
	if doc.Mut != nil {
		hotKeys = append(hotKeys, path)
	}

	// TODO add unique

	var w2 kv.Write
	if len(hotKeys) == 0 {
		w2 = s.kv.Write()
	} else {
		w2, err = s.kv.ExclusiveWrite(ctx, hotKeys...)
		if err != nil {
			return err
		}

		statLockRetries := w2.(*kv.TikvWrite).Stat()
		span.SetAttributes(attribute.Int("lockRetries", statLockRetries))
		kvLockRetries.WithLabelValues("hot", "true").Observe(float64(statLockRetries))
	}
	defer w2.Close()

	now := time.Now()
	doc.History = &openapi.History{
		Created: &now,
		Updated: &now,
	}

	var old *openapi.Document

	if doc.Version != nil || doc.Mut != nil { // Versioned updates

		bytes, err := w2.Get(ctx, []byte(path))
		if err != nil {
			if !tikerr.IsErrNotFound(err) {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
			}
		} else if len(bytes) > 0 {
			old = new(openapi.Document)
			if err := DeserializeStore(bytes, old); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("unmarshal error: %v", err))
			}

			if err := s.deleteIndex(ctx, w2, model, old); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("index error: %v", err))
			}

			if old.History != nil {
				doc.History.Created = old.History.Created
			}

			if reflect.DeepEqual(old.Val, doc.Val) {
				return c.JSON(http.StatusOK, doc)
			}

			if old.Version != nil && doc.Version != nil {
				if *old.Version != *doc.Version {
					return echo.NewHTTPError(http.StatusConflict, "version is out of date")
				}
			}
		}

	} else { // Non-versioned updates

		r := s.kv.Read()
		bytes, err := r.Get(ctx, []byte(path))
		r.Close()
		if err != nil {
			if !tikerr.IsErrNotFound(err) {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
			}
		} else if len(bytes) > 0 {
			old = new(openapi.Document)
			if err := DeserializeStore(bytes, old); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("unmarshal error: %v", err))
			}

			if err := s.deleteIndex(ctx, w2, model, old); err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("index error: %v", err))
			}

			if old.History != nil {
				doc.History.Created = old.History.Created
			}

			doc.Version = old.Version
		}
	}

	if doc.Version == nil {
		if old == nil {
			version := uint64(0)
			doc.Version = &version
		} else {
			doc.Version = old.Version
		}
	}
	*doc.Version++

	var isMut = false
	if doc.Mut != nil {
		isMut = true
		var val map[string]interface{}
		if old == nil {
			val = make(map[string]interface{})
		} else {
			val, _ = old.Val.(map[string]interface{})
		}
		nval, err := Mutate(val, model, doc.Mut)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		doc.Val = nval
		doc.Mut = nil
	}

	doc, err = s.ro.Validate(ctx, old, doc)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	}

	bytes, err := SerializeStore(doc)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("marshal error: %v", err))
	}

	w2.Put([]byte(path), bytes)

	if err := s.createIndex(ctx, w2, model, doc); err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}

	commitStart := time.Now()
	err = w2.Commit(ctx)
	commitDuration := time.Since(commitStart)

	kvCommitDuration.WithLabelValues("write_transaction").Observe(commitDuration.Seconds())

	if err != nil {
		// Record failed commit metrics
		if tikerr.IsErrWriteConflict(err) {
			kvCommitFailures.WithLabelValues("write_transaction", "write_conflict").Inc()
			if !isMut {
				return echo.NewHTTPError(http.StatusConflict, fmt.Sprintf("preempted by a different parallel write"))
			} else {
				return err
			}
		} else {
			kvCommitFailures.WithLabelValues("write_transaction", "database_error").Inc()
			return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
		}
	}

	err = s.ro.Reconcile(ctx, old, doc)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, err.Error())
	}

	return c.JSON(http.StatusOK, doc)
}

func (s *server) GetDocument(c echo.Context, model string, id string) error {
	ctx, span := tracer.Start(c.Request().Context(), "GetDocument",
		trace.WithAttributes(
			attribute.String("model", model),
			attribute.String("id", id),
		),
	)
	defer span.End()

	var doc openapi.Document
	err := s.getDocument(ctx, model, id, &doc)
	if err != nil {
		span.RecordError(err)
		return err
	}

	if model == "Reactor" {
		s.ro.Status(ctx, &doc)
	}

	return c.JSON(http.StatusOK, doc)
}

func (s *server) getDocument(ctx context.Context, model string, id string, doc *openapi.Document) error {

	ctx, span := tracer.Start(ctx, "getDocument",
		trace.WithAttributes(
			attribute.String("model", model),
			attribute.String("id", id),
		),
	)
	defer span.End()

	path, err := safeDBPath(model, id)
	if err != nil {
		span.RecordError(err)
		return err
	}

	r := s.kv.Read()
	defer r.Close()

	// Add span for database get operation
	bytes, err := r.Get(ctx, []byte(path))

	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if bytes == nil {
		notFoundErr := echo.NewHTTPError(http.StatusNotFound, "document not found")
		span.RecordError(notFoundErr)
		return notFoundErr
	}

	// Add span for deserialization
	err = DeserializeStore(bytes, doc)

	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusInternalServerError, "unmarshal error")
	}

	return nil
}

func (s *server) DeleteDocument(c echo.Context, model string, id string) error {
	ctx, span := tracer.Start(c.Request().Context(), "DeleteDocument",
		trace.WithAttributes(
			attribute.String("model", model),
			attribute.String("id", id),
		),
	)
	defer span.End()

	path, err := safeDBPath(model, id)
	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	w, err := s.kv.ExclusiveWrite(ctx, path)
	if err != nil {
		return err
	}
	defer w.Close()

	// First get the document to remove its indexes
	bytes, err := w.Get(ctx, []byte(path))

	if err != nil {
		span.RecordError(err)
		if tikerr.IsErrNotFound(err) {
			return c.NoContent(http.StatusOK)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
	}
	if bytes == nil {
		notFoundErr := echo.NewHTTPError(http.StatusNotFound, "document is empty")
		span.RecordError(notFoundErr)
		return notFoundErr
	}

	var doc = new(openapi.Document)
	err = DeserializeStore(bytes, doc)

	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusInternalServerError, "unmarshal error")
	}

	span.SetAttributes(attribute.String("document.model", doc.Model))

	switch doc.Model {
	case "Model":
		err := s.checkNothingNeedsModel(ctx, doc.Id)
		if err != nil {
			span.RecordError(err)
			return err
		}
	case "Reactor":
	default:
		var schema openapi.Document
		err := s.getDocument(ctx, "Model", doc.Model, &schema)
		if err != nil {
			span.RecordError(err)
			return echo.NewHTTPError(http.StatusInternalServerError, "cannot load model")
		}
	}

	_, err = s.ro.Validate(ctx, doc, nil)
	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	modell, err := s.getModel(ctx, doc.Model)
	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Remove indexes first
	err = s.deleteIndex(ctx, w, modell, doc)
	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("index error: %v", err))
	}

	// Delete the document
	w.Del([]byte(path))

	// TODO: this should eventually run async with a "about to be deleted" state
	err = s.ro.Reconcile(ctx, doc, nil)
	if err != nil {
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Measure KV commit time
	commitStart := time.Now()
	err = w.Commit(ctx)
	commitDuration := time.Since(commitStart)

	// Always record the commit duration, even if it failed
	kvCommitDuration.WithLabelValues("delete_document").Observe(commitDuration.Seconds())

	if err != nil {
		span.RecordError(err)
		// Record failed commit
		if tikerr.IsErrWriteConflict(err) {
			kvCommitFailures.WithLabelValues("delete_document", "write_conflict").Inc()
		} else {
			kvCommitFailures.WithLabelValues("delete_document", "database_error").Inc()
		}
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("database error: %v", err))
	}

	return c.NoContent(http.StatusOK)
}
