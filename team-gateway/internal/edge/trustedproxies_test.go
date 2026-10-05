package edge

import (
	"errors"
	"net/netip"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	for _, bad := range []string{"10.0.0.0/33", "not a host!", "10.0.0.0/8, 1.2.3.4/"} {
		if _, err := ParseTrustedProxies(bad, nil); err == nil {
			t.Errorf("ParseTrustedProxies(%q): want an error", bad)
		}
	}
	empty, err := ParseTrustedProxies("", nil)
	if err != nil || empty.Contains(netip.MustParseAddr("127.0.0.1")) {
		t.Fatalf("empty list must trust nobody (err=%v)", err)
	}
	var nilTP *TrustedProxies
	if nilTP.Contains(netip.MustParseAddr("127.0.0.1")) {
		t.Fatal("nil TrustedProxies must trust nobody")
	}

	tp, err := ParseTrustedProxies("10.0.0.0/8, 192.168.1.5, ::1", nil)
	if err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]bool{
		"10.1.2.3": true, "192.168.1.5": true, "192.168.1.6": false,
		"::1": true, "::ffff:10.0.0.9": true, "11.0.0.1": false,
	} {
		if got := tp.Contains(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Contains(%s) = %v, want %v", addr, got, want)
		}
	}
}

// A hostname entry (the frontend's service name) trusts exactly the addresses it
// resolves to, follows a restart's new address on Refresh, and trusts nobody while
// it does not resolve.
func TestTrustedProxiesHostnameResolution(t *testing.T) {
	addrs := []string{"172.20.0.9"}
	var lookupErr error
	tp, err := ParseTrustedProxies("team-frontend-svc", func(host string) ([]string, error) {
		if host != "team-frontend-svc" {
			t.Fatalf("unexpected lookup %q", host)
		}
		return addrs, lookupErr
	})
	if err != nil {
		t.Fatal(err)
	}
	fe, other := netip.MustParseAddr("172.20.0.9"), netip.MustParseAddr("172.20.0.77")
	if !tp.Contains(fe) || tp.Contains(other) {
		t.Fatal("only the frontend's address may be trusted, not its whole network")
	}

	addrs = []string{"172.20.0.10"} // container restarted with a new address
	tp.Refresh()
	if tp.Contains(fe) || !tp.Contains(netip.MustParseAddr("172.20.0.10")) {
		t.Fatal("Refresh must follow the new address")
	}

	lookupErr = errors.New("nxdomain")
	tp.Refresh()
	if tp.Contains(netip.MustParseAddr("172.20.0.10")) {
		t.Fatal("an unresolvable hostname must fail closed")
	}
}
