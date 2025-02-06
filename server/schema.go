package server

import (
	"apogy/api"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func (s *server) handlePutSchema(c echo.Context, req api.PutObjectRequest) error {

	idparts := strings.FieldsFunc(req.Object.Id, func(r rune) bool {
		return r == '.'
	})
	if len(idparts) < 3 {
		return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
			Error: "validation error (id): must be a domain , like com.example.Book",
		})
	}

	err := s.validateSchemaSchema(c.Request().Context(), req.Object)
	if err != nil {
		return c.JSON(http.StatusBadRequest, api.PutObjectResponse{
			Error: fmt.Sprintf("validation error: %s", err),
		})
	}

	jo, err := json.Marshal(req.Object)
	if err != nil {
		return err
	}

	w := s.kv.Write()
	defer w.Rollback()
	defer w.Close()

	w.Put([]byte("o\xffModel\xff"+req.Object.Id+"\xff"), jo)

	err = w.Commit(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, api.PutObjectResponse{
			Error: fmt.Sprintf("kv error: %s", err.Error()),
		})
	}

	return c.JSON(http.StatusOK, api.PutObjectResponse{
		Path: "Model/" + req.Object.Id,
	})
}

func (s *server) validateSchemaSchema(ctx context.Context, object api.Object) error {

	compiler := jsonschema.NewCompiler()
	ob, err := json.Marshal(object.Val)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	err = compiler.AddResource(object.Id, strings.NewReader(string(ob)))
	if err != nil {
		return fmt.Errorf("failed to add schema resource: %w", err)
	}

	// Attempt to compile the schema
	_, err = compiler.Compile(object.Id)
	if err != nil {
		switch v := err.(type) {
		case *jsonschema.SchemaError:
			return fmt.Errorf("invalid schema: %w", v.Err)
		default:
			return fmt.Errorf("failed to compile schema: %w", err)
		}
	}

	return nil
}

func (s *server) validateObjectSchema(ctx context.Context, object api.Object) error {

	r := s.kv.Read()
	defer r.Close()

	schemaJson, err := r.Get(ctx, []byte("o\xffModel\xff"+object.Model+"\xff"))
	if err != nil {
		return fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	var schemaObj api.Object
	err = json.Unmarshal(schemaJson, &schemaObj)
	if err != nil {
		return fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	schemaJson, err = json.Marshal(schemaObj.Val)
	if err != nil {
		return fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	compiler := jsonschema.NewCompiler()
	err = compiler.AddResource("schema://"+object.Model, bytes.NewReader(schemaJson))
	if err != nil {
		return fmt.Errorf("failed to load schema '%s': %w", object.Model, err)
	}
	jschema, err := compiler.Compile("schema://" + object.Model)
	if err != nil {
		return fmt.Errorf("failed to load schema '%s': %w", object.Model, err)
	}

	err = jschema.Validate(object.Val)
	if err != nil {
		return err
	}

	return nil
}
