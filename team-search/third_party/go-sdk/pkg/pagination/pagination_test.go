package pagination_test

import (
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pagination"
)

func TestOffsetToken(t *testing.T) {
	n, err := pagination.DecodeOffset(pagination.EncodeOffset(40))
	if err != nil || n != 40 {
		t.Errorf("round trip = %d, %v; want 40, nil", n, err)
	}
	if n, err := pagination.DecodeOffset(""); err != nil || n != 0 {
		t.Errorf("empty = %d, %v; want 0, nil", n, err)
	}
	for _, bad := range []string{"zz", "!!!", pagination.EncodeOffset(-1)} {
		if _, err := pagination.DecodeOffset(bad); err == nil {
			t.Errorf("DecodeOffset(%q) accepted", bad)
		}
	}
}

func TestClamp(t *testing.T) {
	for _, tc := range []struct{ size, def, max, want int }{
		{0, 20, 100, 20}, {-5, 20, 100, 20}, {1, 20, 100, 1}, {100, 20, 100, 100}, {101, 20, 100, 100},
	} {
		if got := pagination.Clamp(tc.size, tc.def, tc.max); got != tc.want {
			t.Errorf("Clamp(%d,%d,%d) = %d, want %d", tc.size, tc.def, tc.max, got, tc.want)
		}
	}
}
