package apogy

import (
	"fmt"
)

type Error struct {
	Code    int
	Message string
}

func (e Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

func IsErrorValidationFailed(e error) bool {
	ee, ok := e.(Error)
	if !ok {
		return false
	}
	return ee.Code == 422
}
