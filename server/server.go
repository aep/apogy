package server

import (
	"apogy/api"
	kv "apogy/kv"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type server struct {
	kv kv.KV
}

func newServer(kv kv.KV) *server {
	return &server{
		kv: kv,
	}
}

func validateMeta(object api.Object) error {

	if len(object.Model) < 1 {
		return fmt.Errorf("validation error: /model must not be empty")
	}
	if len(object.Model) > 64 {
		return fmt.Errorf("validation error: /model must be less than 64 bytes")
	}
	if len(object.Id) > 64 {
		return fmt.Errorf("validation error: /id must be less than 64 bytes")
	}

	for _, char := range object.Model {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char == '.') ||
			(char == '-') ||
			(char >= '0' && char <= '9')) {
			return fmt.Errorf("validation error: /$schema has invalid character: %c", char)
		}
	}

	for _, char := range object.Id {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char == '.') ||
			(char == '-') ||
			(char >= '0' && char <= '9')) {
			return fmt.Errorf("validation error: /$id has invalid character: %c", char)
		}
	}
	return nil
}

type Object map[string]interface{}

func (s *server) handlePutObject(c echo.Context) error {
	var req api.PutObjectRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
			Error: fmt.Sprintf("invalid request body: %v", err),
		})
	}

	err := validateMeta(req.Object)
	if err != nil {
		return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
			Error: err.Error(),
		})
	}

	if req.Object.Model == "Model" {
		return s.handlePutSchema(c, req)
	}

	err = s.validateObjectSchema(c.Request().Context(), req.Object)
	if err != nil {
		return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
			Error: fmt.Sprintf("validation error: %s", err),
		})
	}

	w2 := s.kv.Write()
	defer w2.Close()

	path := fmt.Sprintf("o\xff%s\xff%s\xff", req.Object.Model, req.Object.Id)

	now := time.Now()
	req.Object.History = &api.History{
		Created: now,
		Updated: now,
	}

	if req.Object.Version != 0 {

		jso, err := w2.Get(c.Request().Context(), []byte(path))
		if err != nil {
			if !strings.Contains(err.Error(), "not exist") {
				return c.JSON(http.StatusInternalServerError, api.PutObjectResponse{
					Error: fmt.Sprintf("tikv error %v", err),
				})
			}
		} else {
			var original api.Object
			json.Unmarshal(jso, &original)

			deleteIndex(w2, original)

			if original.History != nil {
				req.Object.History.Created = original.History.Created
			}

			if reflect.DeepEqual(original.Val, req.Object.Val) {
				return c.JSON(http.StatusOK, api.PutObjectResponse{
					Path: req.Object.Model + "/" + req.Object.Id,
				})
			}

			if original.Version != req.Object.Version {
				return c.JSON(http.StatusConflict, api.PutObjectResponse{
					Error: fmt.Sprintf("version is out of date"),
				})
			}

		}
	} else {

		// even when user didnt request versioning we need to delete outdated index
		// use a separate reader because we don't care about conflicts
		r := s.kv.Read()
		jso, err := r.Get(c.Request().Context(), []byte(path))
		r.Close()
		if err != nil {
			if !strings.Contains(err.Error(), "not exist") {
				return c.JSON(http.StatusInternalServerError, api.PutObjectResponse{
					Error: fmt.Sprintf("tikv error %v", err),
				})
			}
		} else {
			var original api.Object
			json.Unmarshal(jso, &original)
			deleteIndex(w2, original)
		}
	}

	req.Object.Version += 1

	rawjson, err := json.Marshal(req.Object)
	if err != nil {
		return fmt.Errorf("json marshal error: %v", err)
	}

	w2.Put([]byte(path), rawjson)

	err = createIndex(w2, req.Object)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, api.PutObjectResponse{
			Error: err.Error(),
		})
	}

	err = w2.Commit(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, api.PutObjectResponse{
			Error: fmt.Sprintf("tikv error: %v", err),
		})
	}

	return c.JSON(http.StatusOK, api.PutObjectResponse{
		Path: req.Object.Model + "/" + req.Object.Id,
	})
}

func (s *server) handleGetObject(c echo.Context) error {
	model := c.Param("model")
	id := c.Param("id")

	path := fmt.Sprintf("o\xff%s\xff%s\xff", model, id)

	r := s.kv.Read()
	defer r.Close()

	value, err := r.Get(c.Request().Context(), []byte(path))
	if err != nil {
		return c.NoContent(http.StatusNotFound)
	}

	if value == nil {
		return c.NoContent(http.StatusNotFound)
	}

	var obj api.Object
	err = json.Unmarshal(value, &obj)
	if err != nil {
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, obj)
}

func Main() {
	kv, err := kv.NewTikv()
	if err != nil {
		panic(err)
	}

	s := newServer(kv)

	e := echo.New()

	// Add logger middleware
	e.Use(middleware.Logger())

	// Routes
	e.GET("/o/:model/:id", s.handleGetObject)
	e.GET("/q", s.handleSearch)
	e.POST("/q", s.handleSearch)
	e.POST("/o", s.handlePutObject)

	// Start server
	e.Logger.Fatal(e.Start(":5051"))
}
