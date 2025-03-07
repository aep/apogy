package server

import (
	"context"

	"fmt"
	openapi "github.com/aep/apogy/api/go"
	"github.com/aep/yema"
	yparser "github.com/aep/yema/parser"
)

type Model struct {
	Id     string
	Schema *yema.Type
	Index  map[string]string
}

var MODEL_MODEL = &Model{
	Id: "Model",
}

var MODEL_REACTOR = &Model{
	Id: "Reactor",
}

func (s *server) getModel(ctx context.Context, id string) (*Model, error) {

	if id == "Model" {
		return MODEL_MODEL, nil
	} else if id == "Reactor" {
		return MODEL_REACTOR, nil
	}

	model, found := s.modelCache.Get(id)
	if found {
		return model, nil
	}

	var doc openapi.Document
	err := s.getDocument(ctx, "Model", id, &doc)
	if err != nil {
		return nil, fmt.Errorf("can't load model: %w", err)
	}

	model = new(Model)
	model.Id = doc.Id
	model.Index = make(map[string]string)

	val, _ := doc.Val.(map[string]interface{})
	if val != nil {

		ss, ok := val["schema"].(map[string]interface{})
		if ok {

			yy, err := yparser.From(ss)
			if err != nil {
				return nil, fmt.Errorf("can't load model schema: %w", err)
			}
			model.Schema = yy
		}

		ix, _ := val["index"].(map[string]interface{})
		for k, v := range ix {
			if v, ok := v.(string); ok {
				model.Index["val."+k] = v
			}
		}

	}

	s.modelCache.Set(id, model)

	return model, nil
}
