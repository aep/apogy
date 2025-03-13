package reactor

import (
	"context"
	"fmt"

	openapi "github.com/aep/apogy/api/go"
)

type immutableReactor struct{}

func (*immutableReactor) Ready(model *openapi.Document, args interface{}) (interface{}, error) {
	return nil, nil
}

func (*immutableReactor) Stop() {}

func (*immutableReactor) Validate(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) (*openapi.Document, error) {
	if old != nil && nuw != nil {
		return nil, fmt.Errorf("Document is immutable")
	}
	return nuw, nil
}

func (*immutableReactor) Reconcile(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) error {
	return nil
}

func (ro *Reactor) startBuiltins() {
	ro.running["immutable"] = &immutableReactor{}
	ro.running["schema"] = NewYemaReactor()
	ro.running["cue"] = NewCueReactor()
}
