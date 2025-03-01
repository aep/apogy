package reactor

import (
	"fmt"

	"context"
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/aep/apogy/api/go"
)

type cueReactor struct {
	cctx   *cue.Context
	schema cue.Value
}

func StartCueReactor(doc *openapi.Document) (Runtime, error) {
	if doc.Val == nil {
		return nil, fmt.Errorf("val must not be empty")
	}

	src, _ := (*doc.Val)["source"].(string)
	if src == "" {
		return nil, fmt.Errorf("val.source must not be empty")
	}

	ctx := cuecontext.New()

	schema := ctx.CompileString(src)
	if schema.Err() != nil {
		return nil, schema.Err()
	}

	return &cueReactor{cctx: ctx, schema: schema}, nil
}

func (cr *cueReactor) Stop() {
}

func (jsr *cueReactor) Validate(ctx context.Context, old *openapi.Document, nuw *openapi.Document) (*openapi.Document, error) {

	if nuw == nil {
		return nil, nil
	}

	y := jsr.cctx.Encode(nuw)
	if y.Err() != nil {
		return nil, fmt.Errorf("encode to cue failed: %w", y.Err())
	}

	unified := jsr.schema.Unify(y)
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

func (cr *cueReactor) Reconcile(ctx context.Context, old *openapi.Document, nuw *openapi.Document) error {
	return nil
}
