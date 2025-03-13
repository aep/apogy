package reactor

import (
	"fmt"

	"context"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cuejson "cuelang.org/go/encoding/json"
	"encoding/json"
	openapi "github.com/aep/apogy/api/go"
	ycue "github.com/aep/yema/cue"
	yparser "github.com/aep/yema/parser"
	"gopkg.in/yaml.v3"
)

type cueReactor struct {
}

func NewCueReactor() Runtime {
	return &cueReactor{}
}

func (cr *cueReactor) Stop() {}

type cueReady struct {
	cctx   *cue.Context
	schema cue.Value
}

func (cr *cueReactor) Ready(model *openapi.Document, args interface{}) (interface{}, error) {

	val, _ := model.Val.(map[string]interface{})
	if val == nil {
		return nil, nil
	}

	ss, ok := val["schema"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	yy, err := yparser.From(ss)
	if err != nil {
		return nil, err
	}

	ctx := cuecontext.New()

	schema, err := ycue.ToCue(ctx, yy)
	if err != nil {
		return nil, err
	}

	var src string
	if srcAsMap, ok := val["cue"].(map[string]interface{}); ok {
		srcB, _ := yaml.Marshal(srcAsMap)
		src = string(srcB)
	} else if srcAsList, ok := val["cue"].([]map[string]interface{}); ok {
		for _, li := range srcAsList {
			srcB, _ := yaml.Marshal(li)
			src += string(srcB)
		}
	} else if srcAsList, ok := val["cue"].([]interface{}); ok {
		for _, li := range srcAsList {
			srcB, _ := yaml.Marshal(li)
			src += string(srcB)
		}
	} else {
		return nil, fmt.Errorf("couldnt parse cue")
	}

	schema2 := ctx.CompileString(src, cue.InferBuiltins(true))
	if schema.Err() != nil {
		return nil, schema.Err()
	}

	schema = schema.Unify(schema2)
	if schema.Err() != nil {
		return nil, schema.Err()
	}

	return &cueReady{cctx: ctx, schema: schema2}, nil
}

func (jsr *cueReactor) Validate(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) (*openapi.Document, error) {

	if nuw == nil {
		return nil, nil
	}

	if args == nil {
		return nil, nil
	}

	cueReady := args.(*cueReady)

	js, err := json.Marshal(nuw.Val)
	if err != nil {
		return nil, err
	}

	err = cuejson.Validate(js, cueReady.schema)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// FIXME I think this leaks because the ctx is never released
	/*
		yj, err := cuejson.Extract("doc", js)
		if err != nil {
			return nil, fmt.Errorf("encode to cue failed: %w", err)
		}

		y := cueReady.cctx.BuildExpr(yj)
		if y.Err() != nil {
			return nil, fmt.Errorf("build: %w", y.Err())
		}

		unified := cueReady.schema.Unify(y)
		if unified.Err() != nil {
			return nil, fmt.Errorf("unify: %w", unified.Err())
		}

		fmt.Println(unified)

		err = unified.Validate(cue.Final(), cue.Concrete(true))
		if err != nil {
			return nil, fmt.Errorf("validation: %w", err)
		}

		err = unified.Decode(nuw)
		if err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}

	*/

	return nuw, nil
}

func (cr *cueReactor) Reconcile(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) error {
	return nil
}
