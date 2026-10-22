package kafkax_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
)

var fp12 = regexp.MustCompile(`^[0-9a-f]{12}$`)

func TestFingerprintFormatStableAndKeyed(t *testing.T) {
	a, err := kafkax.NewFingerprinter("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	b, err := kafkax.NewFingerprinter("other-secret")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("seller-123")
	first, second := a.Of(key), a.Of(key)
	otherKey := a.Of([]byte("seller-124"))
	long := a.Of([]byte(strings.Repeat("k", 200)))
	nilKey := a.Of(nil)
	otherSecret := b.Of(key)

	for name, got := range map[string]string{"first": first, "second": second, "otherKey": otherKey, "long": long, "nil": nilKey, "otherSecret": otherSecret} {
		if !fp12.MatchString(got) {
			t.Errorf("%s = %q, want 12 lowercase hex", name, got)
		}
	}
	if first != second {
		t.Errorf("same key gave %q then %q", first, second)
	}
	for name, got := range map[string]string{"otherKey": otherKey, "long": long, "nil": nilKey, "otherSecret": otherSecret} {
		if got == first {
			t.Errorf("%s fingerprint equals the first key's (%q)", name, got)
		}
	}
}

func TestNewFingerprinterRejectsEmptySecret(t *testing.T) {
	if f, err := kafkax.NewFingerprinter(""); err == nil {
		t.Fatalf("NewFingerprinter(\"\") = %v, nil; want an error", f)
	}
}
