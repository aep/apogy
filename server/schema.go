package server

import (
	"context"
	"fmt"
	yparser "github.com/aep/yema/parser"
	"net/http"
	"strings"

	openapi "github.com/aep/apogy/api/go"
	"github.com/labstack/echo/v4"
)

func (s *server) validateReactorSchema(ctx context.Context, object *openapi.Document) error {
	idparts := strings.FieldsFunc(object.Id, func(r rune) bool {
		return r == '.'
	})
	if len(idparts) < 2 {
		return fmt.Errorf("validation error (id): must be a domain, like com.example.Book")
	}
	return nil
}

func (s *server) validateSchemaSchema(ctx context.Context, doc *openapi.Document) error {

	idparts := strings.FieldsFunc(doc.Id, func(r rune) bool {
		return r == '.'
	})
	if len(idparts) < 2 {
		return echo.NewHTTPError(http.StatusBadRequest, "validation error (id): must be a domain , like com.example.Book")
	}

	val, _ := doc.Val.(map[string]interface{})
	if val == nil {
		return nil
	}

	ym, _ := val["schema"].(map[string]interface{})

	_, err := yparser.From(ym)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("validation error (val.schema): %s", err))
	}

	return nil
}

// FIXME this whole thing needs to go into a reactor

/*

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

// validateLinkedDocument checks if a linked document exists
func (s *server) validateLinkedDocument(ctx context.Context, model string, id string) error {
	var doc openapi.Document
	err := s.getDocument(ctx, model, id, &doc)
	if err != nil {
		return fmt.Errorf("linked document %s/%s does not exist", model, id)
	}
	return nil
}

// recursiveSchemaCheck performs recursive validation of linked models with loop detection
func (s *server) recursiveSchemaCheck(ctx context.Context, r kv.Read, model string, modelDoc *openapi.Document, path []string, depth int) error {
	if depth > 10 {
		return fmt.Errorf("maximum schema link depth exceeded (10) starting from model %s", model)
	}

	// Check if the current model creates a loop in the dependency path
	for _, visitedModel := range path {
		if visitedModel == model {
			// Create a readable path string showing the loop
			pathStr := strings.Join(append(path, model), " -> ")
			return fmt.Errorf("circular reference detected: %s", pathStr)
		}
	}

	// Add current model to the path
	currentPath := append(path, model)

	// For linked models (not the first one), we need to load them
	if modelDoc == nil {
		modelData, err := r.Get(ctx, []byte("o\xffModel\xff"+model+"\xff"))
		if err != nil {
			return fmt.Errorf("error loading model %s: %w", model, err)
		}
		if modelData == nil {
			return fmt.Errorf("model %s does not exist", model)
		}

		modelDoc = &openapi.Document{}
		if err := msgpack.Unmarshal(modelData, modelDoc); err != nil {
			return fmt.Errorf("error unmarshaling model %s: %w", model, err)
		}
	}

	if modelDoc.Val == nil {
		return nil
	}

	properties, ok := (*modelDoc.Val)["properties"].(map[string]interface{})
	if !ok {
		return nil
	}

	// Check all properties for links
	for _, propValue := range properties {
		prop, ok := propValue.(map[string]interface{})
		if !ok {
			continue
		}

		// Check direct links
		if link, exists := prop["link"].(string); exists {
			if err := s.recursiveSchemaCheck(ctx, r, link, nil, currentPath, depth+1); err != nil {
				return err
			}
		}

		// Check array items
		if propType, ok := prop["type"].(string); ok && propType == "array" {
			if items, ok := prop["items"].(map[string]interface{}); ok {
				if link, exists := items["link"].(string); exists {
					if err := s.recursiveSchemaCheck(ctx, r, link, nil, currentPath, depth+1); err != nil {
						return err
					}
				}
			}
		}

		// Check nested objects
		if propType, ok := prop["type"].(string); ok && propType == "object" {
			if nestedProps, ok := prop["properties"].(map[string]interface{}); ok {
				for _, nestedProp := range nestedProps {
					if nestedPropMap, ok := nestedProp.(map[string]interface{}); ok {
						if link, exists := nestedPropMap["link"].(string); exists {
							if err := s.recursiveSchemaCheck(ctx, r, link, nil, currentPath, depth+1); err != nil {
								return err
							}
						}
					}
				}
			}
		}
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
				itemsType, ok := items["type"].(string)
				if !ok || itemsType != "string" {
					return fmt.Errorf("linked items in %s must be of type string", currentPath)
				}

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


// validateNestedLinkedDocuments recursively validates linked documents in nested objects
func (s *server) validateNestedLinkedDocuments(ctx context.Context, schema map[string]interface{}, object map[string]interface{}, propPath string) error {
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return nil
	}

	for propName, propValue := range properties {
		prop, ok := propValue.(map[string]interface{})
		if !ok {
			continue
		}

		currentPath := propPath
		if currentPath != "" {
			currentPath += "."
		}
		currentPath += propName

		// Check if this property exists in the object
		objectValue, exists := object[propName]
		if !exists {
			continue
		}

		// Validate direct links
		if link, exists := prop["link"].(string); exists {
			if id, ok := objectValue.(string); ok && id != "" {
				if err := s.validateLinkedDocument(ctx, link, id); err != nil {
					return fmt.Errorf("invalid link in property %s: %w", currentPath, err)
				}
			}
		}

		// Validate array items with links
		if propType, ok := prop["type"].(string); ok && propType == "array" {
			if items, ok := prop["items"].(map[string]interface{}); ok {
				if link, exists := items["link"].(string); exists {
					if arrayVal, ok := objectValue.([]interface{}); ok {
						for i, item := range arrayVal {
							if id, ok := item.(string); ok && id != "" {
								if err := s.validateLinkedDocument(ctx, link, id); err != nil {
									return fmt.Errorf("invalid link in property %s[%d]: %w", currentPath, i, err)
								}
							}
						}
					}
				}

				// Recursively validate nested objects in arrays
				if itemType, ok := items["type"].(string); ok && itemType == "object" {
					if arrayVal, ok := objectValue.([]interface{}); ok {
						for i, item := range arrayVal {
							if itemMap, ok := item.(map[string]interface{}); ok {
								if err := s.validateNestedLinkedDocuments(ctx, items, itemMap, fmt.Sprintf("%s[%d]", currentPath, i)); err != nil {
									return err
								}
							}
						}
					}
				}
			}
		}

		// Recursively validate nested objects
		if propType, ok := prop["type"].(string); ok && propType == "object" {
			if nestedObj, ok := objectValue.(map[string]interface{}); ok {
				if err := s.validateNestedLinkedDocuments(ctx, prop, nestedObj, currentPath); err != nil {
					return err
				}
			}
		}
	}
	return nil
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
	err = msgpack.Unmarshal(schemaData, schemaObj)
	if err != nil {
		return nil, fmt.Errorf("cannot load model '%s': %w", object.Model, err)
	}

	if schemaObj.Val == nil {
		var v = make(map[string]interface{})
		schemaObj.Val = &v
	}
	(*schemaObj.Val)["type"] = "object"

	// Validate linked documents recursively
	if object.Val != nil {
		if err := s.validateNestedLinkedDocuments(ctx, *schemaObj.Val, *object.Val, ""); err != nil {
			return nil, err
		}
	}

	schemaJson, err := msgpack.Marshal(schemaObj.Val)
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

*/
