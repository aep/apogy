package server

import (
	"apogy/api/go"
	"context"
)

func (self *server) validate(ctx context.Context, schema *openapi.Document, old *openapi.Document, nuw *openapi.Document) error {

	if schema.Val == nil || (*schema.Val)["reactors"] == nil {
		return nil
	}

	reactors, ok := (*schema.Val)["reactors"].([]interface{})
	if !ok {
		return nil
	}

	for _, r := range reactors {
		reactorID, ok := r.(string)
		if !ok {
			continue
		}
		err := self.ra.Validate(ctx, reactorID, old, nuw)
		if err != nil {
			return err
		}
	}
	return nil
}

func (self *server) ensureReactor(ctx context.Context, doc *openapi.Document) error {
	return self.ra.Start(doc)
}
