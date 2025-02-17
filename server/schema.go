package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aep/apogy/api/go"
	"strings"

	"github.com/aep/apogy/kv"
	"github.com/labstack/echo/v4"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"net/http"
)

// validateLink checks if a linked model exists
func (s *server) validateLink(ctx context.Context, r kv.Read, link string) error {
	linkedModelKey := []byte("o\xffModel\xff" + link + "\xff")
	exists, err := r.Get(ctx, linkedModelKey)
	if err != nil {
		return fmt.Errorf("error checking linked model %s: %w", link, err)
	}
	if exists == nil {
		return fmt.Errorf("linked model %s does not exist", link)
	}
	return nil
}

// validateSchemaProperties recursively validates all properties and their links
func (s *server) validateSchemaProperties(ctx context.Context, sourceModel string, r kv.Read, properties map[string]interface{}, path string) error {
	for propName, propValue := range properties {
		currentPath := path
		if currentPath != "" {
			currentPath += "."
		}
		currentPath += propName

		prop, ok := propValue.(map[string]interface{})
		if !ok {
			continue
		}

		// Check direct link fields
		if link, exists := prop["link"].(string); exists {

			propType, ok := prop["type"].(string)
			if !ok || propType != "string" {
				return fmt.Errorf("property %s with link must be of type string", currentPath)
			}

			if sourceModel == link {
				// return fmt.Errorf("property %s cannot link to itself", currentPath)
				// actually its fine i think, we need a recursion check anyway later on the actual values
				continue
			}

			if err := s.validateLink(ctx, r, link); err != nil {
				return err
			}
		}

		// Handle nested objects
		if propType, ok := prop["type"].(string); ok && propType == "object" {
			if nestedProps, ok := prop["properties"].(map[string]interface{}); ok {
				if err := s.validateSchemaProperties(ctx, sourceModel, r, nestedProps, currentPath); err != nil {
					return err
				}
			}
		}

		// Handle arrays
		if propType, ok := prop["type"].(string); ok && propType == "array" {
			items, hasItems := prop["items"].(map[string]interface{})
			if !hasItems {
				continue
			}

			// Check for direct links in array items
			if link, exists := items["link"].(string); exists {
				// Validate items type
				itemsType, ok := items["type"].(string)
				if !ok || itemsType != "string" {
					return fmt.Errorf("linked items in %s must be of type string", currentPath)
				}

				// Validate the link exists
				if err := s.validateLink(ctx, r, link); err != nil {
					return err
				}
			}

			// Handle nested object arrays
			if itemType, ok := items["type"].(string); ok && itemType == "object" {
				if nestedProps, ok := items["properties"].(map[string]interface{}); ok {
					if err := s.validateSchemaProperties(ctx, sourceModel, r, nestedProps, currentPath+"[]"); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (s *server) validateSchemaSchema(ctx context.Context, object *openapi.Document) error {
	idparts := strings.FieldsFunc(object.Id, func(r rune) bool {
		return r == '.'
	})
	if len(idparts) < 3 {
		return echo.NewHTTPError(http.StatusBadRequest, "validation error (id): must be a domain , like com.example.Book")
	}

	// First validate the basic schema structure
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

	if object.Val == nil {
		return fmt.Errorf("invalid schema format")
	}

	properties, ok := (*object.Val)["properties"].(map[string]interface{})
	if !ok {
		return nil // No properties to validate
	}

	r := s.kv.Read()
	defer r.Close()

	// Recursively validate all properties and their links
	return s.validateSchemaProperties(ctx, object.Id, r, properties, "")
}

func (s *server) validateObjectSchema(ctx context.Context, object *openapi.Document) (*openapi.Document, error) {
	r := s.kv.Read()
	defer r.Close()

	schemaData, err := r.Get(ctx, []byte("o\xffModel\xff"+object.Model+"\xff"))
	if err != nil {
		return nil, fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	if schemaData == nil {
		return nil, fmt.Errorf("cannot load model '%s'", object.Model)
	}

	var schemaObj = new(openapi.Document)
	err = json.Unmarshal(schemaData, schemaObj)
	if err != nil {
		return nil, fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	if schemaObj.Val == nil {
		var v = make(map[string]interface{})
		schemaObj.Val = &v
	}
	(*schemaObj.Val)["type"] = "object"

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

	if object.Val == nil {
		var v = make(map[string]interface{})
		object.Val = &v
	}

	err = jschema.Validate(*object.Val)
	if err != nil {
		return nil, err
	}

	return schemaObj, nil
}
