package idempotency

import (
	"errors"
	"fmt"
)

// MDKey is the gRPC metadata key that carries the client's Idempotency-Key.
const MDKey = "idempotency-key"

// MaxLen is the longest accepted key, in bytes.
const MaxLen = 255

// ErrInvalidKey is wrapped by every error Validate returns; test with errors.Is.
var ErrInvalidKey = errors.New("invalid idempotency key")

// Validate reports whether key can travel as gRPC metadata: 1 to 255
// printable ASCII characters (0x20 to 0x7E).
func Validate(key string) error {
	if key == "" {
		return fmt.Errorf("%w: empty", ErrInvalidKey)
	}
	if len(key) > MaxLen {
		return fmt.Errorf("%w: longer than %d characters", ErrInvalidKey, MaxLen)
	}
	for i := range len(key) {
		if key[i] < 0x20 || key[i] > 0x7e {
			return fmt.Errorf("%w: must be printable ASCII", ErrInvalidKey)
		}
	}
	return nil
}
