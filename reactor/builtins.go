package reactor

import (
	"context"
	"fmt"

	"github.com/aep/apogy/api/go"
)

type immutableReactor struct{}

func (*immutableReactor) Stop() {}

func (*immutableReactor) Validate(ctx context.Context, old *openapi.Document, nuw *openapi.Document) (*openapi.Document, error) {
	if old != nil && nuw != nil {
		return nil, fmt.Errorf("Document is immutable")
	}
	return nuw, nil
}

func (*immutableReactor) Reconcile(ctx context.Context, old *openapi.Document, nuw *openapi.Document) error {
	return nil
}

func (ro *Reactor) startBuiltins() {
	ro.running["Immutable"] = &immutableReactor{}
}
