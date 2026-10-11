package calc

import "testing"

func TestMul(t *testing.T) {
	if Mul(2, 3) != 6 {
		t.Fatal("mul")
	}
}
