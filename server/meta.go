package server

import (
	"errors"
	"fmt"
	"github.com/aep/apogy/api/go"
	"strings"
)

func (s *server) validateMeta(doc *openapi.Document) error {

	if len(doc.Model) < 1 {
		return fmt.Errorf("validation error: /model must not be empty")
	}
	if len(doc.Model) > 64 {
		return fmt.Errorf("validation error: /model must be less than 64 bytes")
	}
	if len(doc.Id) < 1 {
		return fmt.Errorf("validation error: /id must not be empty")
	}
	if len(doc.Id) > 64 {
		return fmt.Errorf("validation error: /id must be less than 64 bytes")
	}

	for _, char := range doc.Model {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char == '.') ||
			(char == '-') ||
			(char >= '0' && char <= '9')) {
			return fmt.Errorf("validation error: /model has invalid character: %c", char)
		}
	}

	/*

		TODO: i have no idea if its safe to allow arbitrary characters in the id

		for _, char := range doc.Id {
			if !((char >= 'a' && char <= 'z') ||
				(char >= 'A' && char <= 'Z') ||
				(char == '.') ||
				(char == '-') ||
				(char >= '0' && char <= '9')) {
				return fmt.Errorf("validation error: /id has invalid character: %c", char)
			}
		}
	*/

	return nil
}

func safeDBPath(model string, id string) ([]byte, error) {
	for _, ch := range model {
		if ch == 0xff {
			return nil, errors.New("invalid utf8 string")
		}
	}
	for _, ch := range id {
		if ch == 0xff {
			return nil, errors.New("invalid utf8 string")
		}
	}
	return []byte("o\xff" + model + "\xff" + id + "\xff"), nil
}

func safeDB(model string) ([]byte, error) {
	for _, ch := range model {
		if ch == 0xff {
			return nil, errors.New("invalid utf8 string")
		}
	}
	return []byte(model), nil
}

func escapeNonPrintable(b []byte) string {
	var result strings.Builder
	for _, c := range b {
		if c >= 32 && c <= 126 {
			result.WriteByte(c)
		} else {
			result.WriteString(fmt.Sprintf("\\x%02x", c))
		}
	}
	return result.String()
}
