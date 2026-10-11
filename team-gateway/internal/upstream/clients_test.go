package upstream

import (
	"testing"
	"time"
)

// DIAL_TIMEOUT_SECONDS must bound the reconnect backoff and connect timeout, so
// a recreated upstream is reachable again within seconds.
func TestConnectParamsAreBoundedByDialTimeout(t *testing.T) {
	cp := connectParams(2 * time.Second)
	if cp.Backoff.MaxDelay != 2*time.Second || cp.MinConnectTimeout != 2*time.Second {
		t.Fatalf("connect params = %+v, want max delay and min connect timeout 2s", cp)
	}
	if cp.Backoff.BaseDelay >= cp.Backoff.MaxDelay {
		t.Fatalf("base delay %v must be below the cap %v", cp.Backoff.BaseDelay, cp.Backoff.MaxDelay)
	}
	if got := connectParams(0).Backoff.MaxDelay; got != 2*time.Second {
		t.Fatalf("zero timeout falls back to 2s, got %v", got)
	}
	if n := len(dialOptions(time.Second)); n != 3 {
		t.Fatalf("dialOptions = %d, want creds + stats + connect params", n)
	}
}
