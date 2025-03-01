package reactor

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/aep/apogy/api/go"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"strings"
)

type JsonSchemaReactor struct {
	compiled *jsonschema.Schema
}

func StartJsonSchemaReactor(doc *openapi.Document) (Runtime, error) {

	if doc.Val == nil {
		return nil, fmt.Errorf("val must not be empty")
	}

	if doc.Val == nil {
		var v = make(map[string]interface{})
		doc.Val = &v
	}

	if _, ok := (*doc.Val)["type"]; !ok {
		(*doc.Val)["type"] = "object"
	}

	compiler := jsonschema.NewCompiler()
	ob, err := json.Marshal(doc.Val)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	err = compiler.AddResource(doc.Id, strings.NewReader(string(ob)))
	if err != nil {
		return nil, fmt.Errorf("failed to add schema resource: %w", err)
	}

	// Attempt to compile the schema
	compiled, err := compiler.Compile(doc.Id)
	if err != nil {
		switch v := err.(type) {
		case *jsonschema.SchemaError:
			return nil, fmt.Errorf("invalid schema: %w", v.Err)
		default:
			return nil, fmt.Errorf("failed to compile schema: %w", err)
		}
	}

	return &JsonSchemaReactor{
		compiled: compiled,
	}, nil
}

func (jsr *JsonSchemaReactor) Validate(ctx context.Context, old *openapi.Document, nuw *openapi.Document) (*openapi.Document, error) {

	if nuw == nil {
		return nil, nil
	}

	if nuw.Val == nil {
		var v = make(map[string]interface{})
		nuw.Val = &v
	}

	err := jsr.compiled.Validate(*nuw.Val)
	if err != nil {
		return nil, err
	}

	return nuw, nil
}

func (jsr *JsonSchemaReactor) Reconcile(ctx context.Context, old *openapi.Document, nuw *openapi.Document) error {
	return nil
}

func (jsr *JsonSchemaReactor) Stop() {
}
