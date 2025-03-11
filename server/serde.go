package server

import (
	"bytes"
	"encoding/json"
	"errors"
	openapi "github.com/aep/apogy/api/go"
)

func DeserializeStore(b []byte, doc *openapi.Document) error {
	if len(b) < 1 {
		return nil
	}
	if b[0] != 'j' {
		return errors.New("invalid encoding stored in database")
	}
	dec := json.NewDecoder(bytes.NewReader(b[1:]))
	dec.UseNumber()
	return dec.Decode(doc)
}

func SerializeStore(doc *openapi.Document) ([]byte, error) {
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return append([]byte{'j'}, b...), nil
}
