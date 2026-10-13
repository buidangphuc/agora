package edge

import (
	"strings"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/idempotency"
)

func TestValidIdempotencyKey(t *testing.T) {
	for key, want := range map[string]bool{
		"abc-123":                    true,
		strings.Repeat("k", 255):     true,
		"":                           false,
		strings.Repeat("k", 256):     false,
		"a\x01b":                     false,
		"a\x7fb":                     false,
		"khóa":                       false,
		"with space and ~ symbols!?": true,
	} {
		if got := idempotency.Validate(key) == nil; got != want {
			t.Errorf("idempotency.Validate(%q) ok = %v, want %v", key, got, want)
		}
	}
}
