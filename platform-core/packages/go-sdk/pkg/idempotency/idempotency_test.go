package idempotency_test

import (
	"strings"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/idempotency"
)

func TestValidateRule(t *testing.T) {
	bad := map[string]string{
		"empty":        "",
		"256 chars":    strings.Repeat("a", 256),
		"newline":      "a\nb",
		"tab":          "a\tb",
		"del":          "a\x7fb",
		"non-ascii":    "khóa",
		"leading ctrl": "\x01abc",
	}
	for name, k := range bad {
		if err := idempotency.Validate(k); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	ok := map[string]string{
		"one char":   "a",
		"255 chars":  strings.Repeat("a", 255),
		"with space": "order 1 ~!",
		"uuid":       "7b9f3c52-1d0e-4a7b-9c11-0a2f5e6d8b30",
	}
	for name, k := range ok {
		if err := idempotency.Validate(k); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if idempotency.MDKey != "idempotency-key" {
		t.Fatalf("MDKey = %q", idempotency.MDKey)
	}
}
