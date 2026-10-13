package idempotency

import (
	"errors"
	"fmt"
)

// MDKey is the gRPC metadata key that carries the client's Idempotency-Key.
const MDKey = "idempotency-key"

// MaxLen is the longest accepted key, in bytes.
const MaxLen = 255

// Validate reports whether key can travel as gRPC metadata: 1 to 255
// printable ASCII characters (0x20 to 0x7E).
func Validate(key string) error {
	if key == "" {
		return errors.New("idempotency key is empty")
	}
	if len(key) > MaxLen {
		return fmt.Errorf("idempotency key is longer than %d characters", MaxLen)
	}
	for i := range len(key) {
		if key[i] < 0x20 || key[i] > 0x7e {
			return errors.New("idempotency key must be printable ASCII")
		}
	}
	return nil
}
