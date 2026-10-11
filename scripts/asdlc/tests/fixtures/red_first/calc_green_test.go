package calc

import "testing"

func TestAddLate(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("add")
	}
}
