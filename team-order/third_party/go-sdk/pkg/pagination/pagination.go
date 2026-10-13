package pagination

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
)

// Clamp returns the page size to use: def when size is not positive, limit
// when size is above limit, size otherwise.
func Clamp(size, def, limit int) int {
	switch {
	case size <= 0:
		return def
	case size > limit:
		return limit
	default:
		return size
	}
}

// EncodeOffset returns the opaque page token for row offset n.
func EncodeOffset(n int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(n)))
}

// DecodeOffset returns the row offset held by tok. An empty token decodes to
// 0; a malformed or negative token returns an error.
func DecodeOffset(tok string) (int, error) {
	if tok == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return 0, fmt.Errorf("invalid page token: %w", err)
	}
	n, err := strconv.Atoi(string(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid page token: %w", err)
	}
	if n < 0 {
		return 0, errors.New("invalid page token: negative offset")
	}
	return n, nil
}
