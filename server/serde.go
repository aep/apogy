package server

import (
	"bytes"
	"encoding/json"
	openapi "github.com/aep/apogy/api/go"
)

func DeserializeStore(b []byte, doc *openapi.Document) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(doc)
}

func SerializeStore(doc *openapi.Document) ([]byte, error) {
	return json.Marshal(doc)
}
