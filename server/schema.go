package server

import (
	pb "apogy/proto"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func (s *server) validateSchemaSchema(ctx context.Context, object *pb.Document) error {

	idparts := strings.FieldsFunc(object.Id, func(r rune) bool {
		return r == '.'
	})
	if len(idparts) < 3 {
		return status.Errorf(codes.InvalidArgument, "validation error (id): must be a domain , like com.example.Book")
	}

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

func (s *server) validateObjectSchema(ctx context.Context, object *pb.Document) (*pb.Document, error) {

	r := s.kv.Read()
	defer r.Close()

	schemaData, err := r.Get(ctx, []byte("o\xffModel\xff"+object.Model+"\xff"))
	if err != nil {
		return nil, fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	if schemaData == nil {
		return nil, fmt.Errorf("cannot load model '%s'", object.Model)
	}

	var schemaObj = new(pb.Document)
	err = proto.Unmarshal(schemaData, schemaObj)
	if err != nil {
		return nil, fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	schemaJson, err := json.Marshal(schemaObj.Val)
	if err != nil {
		return nil, fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	compiler := jsonschema.NewCompiler()
	err = compiler.AddResource("schema://"+object.Model, bytes.NewReader(schemaJson))
	if err != nil {
		return nil, fmt.Errorf("failed to load schema '%s': %w", object.Model, err)
	}
	jschema, err := compiler.Compile("schema://" + object.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to load schema '%s': %w", object.Model, err)
	}

	err = jschema.Validate(object.Val)
	if err != nil {
		return nil, err
	}

	return schemaObj, nil
}
