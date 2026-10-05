package edge

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Trusted client-context metadata the edge stamps on every upstream call. Services
// use them for audit fields only (a session's device and IP), never authorization.
const (
	mdClientIP        = "x-client-ip"
	mdClientUserAgent = "x-client-user-agent"

	maxClientIPLen = 64
	maxClientUALen = 256
)

// clientInfo is the caller's network identity as the edge sees it.
type clientInfo struct {
	ip        string
	userAgent string
}

const clientKey ctxKey = 100

func withClient(ctx context.Context, c clientInfo) context.Context {
	return context.WithValue(ctx, clientKey, c)
}

func clientFrom(ctx context.Context) (clientInfo, bool) {
	c, ok := ctx.Value(clientKey).(clientInfo)
	return c, ok
}

// hostnameRe accepts DNS-ish names; anything with a "/" is parsed as a CIDR instead.
var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// TrustedProxies is the set of peers whose X-Forwarded-For the edge believes
// (TRUSTED_PROXIES). Entries are CIDRs, bare IPs or hostnames. Hostnames (e.g. the
// frontend's compose service name) are re-resolved periodically so a restarted
// container's new address is picked up; a name that does not resolve trusts nobody
// (fail closed). The zero value and an empty list trust no peer.
type TrustedProxies struct {
	mu       sync.RWMutex
	prefixes []netip.Prefix
	hosts    []string
	resolved []netip.Addr
	lookup   func(host string) ([]string, error)
}

// ParseTrustedProxies parses a comma-separated TRUSTED_PROXIES value. A malformed
// CIDR is an error (a typo must not silently widen or narrow trust). lookup resolves
// hostnames; nil uses the system resolver.
func ParseTrustedProxies(csv string, lookup func(string) ([]string, error)) (*TrustedProxies, error) {
	if lookup == nil {
		lookup = net.LookupHost
	}
	t := &TrustedProxies{lookup: lookup}
	for _, raw := range strings.Split(csv, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		switch {
		case strings.Contains(entry, "/"):
			p, err := netip.ParsePrefix(entry)
			if err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES: bad CIDR %q: %w", entry, err)
			}
			t.prefixes = append(t.prefixes, p.Masked())
		default:
			if a, err := netip.ParseAddr(entry); err == nil {
				t.prefixes = append(t.prefixes, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
			} else if hostnameRe.MatchString(entry) {
				t.hosts = append(t.hosts, entry)
			} else {
				return nil, fmt.Errorf("TRUSTED_PROXIES: %q is not a CIDR, IP or hostname", entry)
			}
		}
	}
	t.Refresh()
	return t, nil
}

// Refresh re-resolves the hostname entries.
func (t *TrustedProxies) Refresh() {
	if t == nil || len(t.hosts) == 0 {
		return
	}
	var out []netip.Addr
	for _, h := range t.hosts {
		addrs, err := t.lookup(h)
		if err != nil {
			continue
		}
		for _, s := range addrs {
			if a, err := netip.ParseAddr(s); err == nil {
				out = append(out, a.Unmap())
			}
		}
	}
	t.mu.Lock()
	t.resolved = out
	t.mu.Unlock()
}

// Run refreshes hostname entries until ctx is done (no-op without hostnames).
func (t *TrustedProxies) Run(ctx context.Context, every time.Duration) {
	if t == nil || len(t.hosts) == 0 {
		return
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.Refresh()
		}
	}
}

// Contains reports whether addr is a trusted proxy.
func (t *TrustedProxies) Contains(addr netip.Addr) bool {
	if t == nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range t.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, a := range t.resolved {
		if a == addr {
			return true
		}
	}
	return false
}

// WithTrustedProxies sets the peers whose X-Forwarded-For is honoured. Set once at
// startup, before the edge serves. Without it no forwarded header is ever trusted.
func (e *Edge) WithTrustedProxies(t *TrustedProxies) *Edge {
	e.trusted = t
	return e
}

// clientInfoFor computes the client IP and user agent for a request. The IP is the
// socket peer; X-Forwarded-For is consulted only when that peer is a trusted proxy,
// and then the rightmost entry that is NOT itself a trusted proxy is the client
// (entries to its left are client-controlled and ignored). Anything unparseable
// falls back to the peer. The user agent is the request's User-Agent (clipped): it
// is audit data a client can already set freely, so it needs no trust decision.
func (e *Edge) clientInfoFor(peerAddr string, header http.Header) clientInfo {
	c := clientInfo{userAgent: clip(header.Get("User-Agent"), maxClientUALen)}
	peer, ok := parseHostAddr(peerAddr)
	if !ok {
		return c
	}
	ip := peer
	if e.trusted.Contains(peer) {
		if xff := strings.Join(header.Values("X-Forwarded-For"), ","); xff != "" {
			ip = clientFromForwarded(xff, peer, e.trusted)
		}
	}
	c.ip = clip(ip.String(), maxClientIPLen)
	return c
}

// clientFromForwarded walks X-Forwarded-For right to left, skipping trusted proxies,
// and returns the first untrusted address. An unparseable entry stops the walk at
// the previous good value (never trusts garbage); an all-trusted chain yields its
// leftmost entry.
func clientFromForwarded(xff string, peer netip.Addr, trusted *TrustedProxies) netip.Addr {
	parts := strings.Split(xff, ",")
	result := peer
	for i := len(parts) - 1; i >= 0; i-- {
		a, ok := parseHostAddr(strings.TrimSpace(parts[i]))
		if !ok {
			return result
		}
		result = a
		if !trusted.Contains(a) {
			return a
		}
	}
	return result
}

// parseHostAddr parses "ip", "ip:port" or "[ip]:port" into an address.
func parseHostAddr(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.Trim(s, "[]")
	if i := strings.IndexByte(s, '%'); i >= 0 { // drop an IPv6 zone
		s = s[:i]
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

func clip(s string, max int) string {
	// gRPC metadata values must be printable ASCII; replace anything else so a
	// hostile User-Agent cannot make the upstream call fail.
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e {
			return '?'
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) <= max {
		return s
	}
	// Cut on a rune boundary so the metadata stays valid UTF-8.
	cut := max
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}
