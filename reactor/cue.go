package reactor

import (
	"fmt"

	"context"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/aep/apogy/api/go"
	ycue "github.com/aep/yema/cue"
	yparser "github.com/aep/yema/parser"
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

	if model.Val == nil {
		return nil, nil
	}

	ss, ok := (*model.Val)["schema"].(map[string]interface{})
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

	src, _ := args.(string)
	if src != "" {

		schema2 := ctx.CompileString(src)
		if schema.Err() != nil {
			return nil, schema.Err()
		}

		schema = schema.Unify(schema2)
		if schema.Err() != nil {
			return nil, schema.Err()
		}

	}

	return &cueReady{cctx: ctx, schema: schema}, nil
}

func (jsr *cueReactor) Validate(ctx context.Context, old *openapi.Document, nuw *openapi.Document, args interface{}) (*openapi.Document, error) {

	if nuw == nil {
		return nil, nil
	}

	if args == nil {
		return nil, nil
	}

	cueReady := args.(*cueReady)

	y := cueReady.cctx.Encode(nuw.Val)
	if y.Err() != nil {
		return nil, fmt.Errorf("encode to cue failed: %w", y.Err())
	}

	unified := cueReady.schema.Unify(y)
	if unified.Err() != nil {
		return nil, fmt.Errorf("validation failed: %w", unified.Err())
	}

	err := unified.Validate(cue.Final(), cue.Concrete(true))
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	err = unified.Decode(nuw)
	if err != nil {
		return nil, fmt.Errorf("decode from cue failed: %w", err)
	}

	return nuw, nil
}

func (cr *cueReactor) Reconcile(ctx context.Context, old *openapi.Document, nuw *openapi.Document, args interface{}) error {
	return nil
}
