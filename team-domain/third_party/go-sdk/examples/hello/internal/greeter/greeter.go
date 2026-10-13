// Package greeter is the hello service's domain: it knows nothing about gRPC.
package greeter

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEmptyName is returned when there is nobody to greet.
var ErrEmptyName = errors.New("name must not be empty")

// Greet returns the greeting for name.
func Greet(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrEmptyName
	}
	return fmt.Sprintf("hello, %s", name), nil
}
