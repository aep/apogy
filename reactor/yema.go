package reactor

import (
	"errors"
	"fmt"

	"context"

	openapi "github.com/aep/apogy/api/go"
	"github.com/aep/yema"
	yparser "github.com/aep/yema/parser"
	yvalidator "github.com/aep/yema/validator"
)

type yemaReactor struct {
}

func NewYemaReactor() Runtime {
	return &yemaReactor{}
}

func (*yemaReactor) Ready(model *openapi.Document, args interface{}) (interface{}, error) {

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

	return yy, nil
}

func (cr *yemaReactor) Stop() {
}

func (yr *yemaReactor) Validate(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) (*openapi.Document, error) {

	if nuw == nil {
		return nil, nil
	}

	if args == nil {
		return nuw, nil
	}

	val, _ := nuw.Val.(map[string]interface{})

	errs := yvalidator.Validate(val, args.(*yema.Type))

	var errStr = "not serializable"
	if len(errs) > 0 {
		for i, e := range errs {
			errStr = fmt.Sprintf("%s, %s", errStr, e)
			if i > 10 {
				errStr = fmt.Sprintf("%s, ...", errStr)
				break
			}
		}

		return nil, errors.New(errStr)
	}

	return nuw, nil
}

func (cr *yemaReactor) Reconcile(ctx context.Context, ro *Reactor, old *openapi.Document, nuw *openapi.Document, args interface{}) error {
	return nil
}
