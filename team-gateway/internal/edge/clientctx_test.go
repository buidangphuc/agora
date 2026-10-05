package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
)

// clientMD sends one anonymous SearchListings call through a real edge whose
// trusted-proxy set is `trusted`, with the given request headers, and returns the
// x-client-ip / x-client-user-agent metadata the upstream service received.
func clientMD(t *testing.T, trusted string, hdr map[string]string) (ip, ua []string) {
	t.Helper()
	tp, err := edge.ParseTrustedProxies(trusted, nil)
	if err != nil {
		t.Fatal(err)
	}
	spy := &upstreamSpy{}
	e := edge.NewEdge(nil, []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000).WithTrustedProxies(tp)
	opts := connect.WithInterceptors(e.Interceptors(slog.New(slog.NewTextHandler(io.Discard, nil)))...)
	mux := http.NewServeMux()
	p, h := searchv1connect.NewSearchServiceHandler(edge.NewSearchForwarder(spySearch{spy: spy}, e), opts)
	mux.Handle(p, h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := searchv1connect.NewSearchServiceClient(srv.Client(), srv.URL)
	req := connect.NewRequest(&searchv1.SearchListingsRequest{})
	for k, v := range hdr {
		req.Header().Set(k, v)
	}
	if _, err := c.SearchListings(context.Background(), req); err != nil {
		t.Fatalf("call: %v", err)
	}
	return spy.md.Get("x-client-ip"), spy.md.Get("x-client-user-agent")
}

// The test client connects from 127.0.0.1, which is the "socket peer".
func TestClientContextDirectIgnoresSpoofedHeaders(t *testing.T) {
	ip, ua := clientMD(t, "", map[string]string{
		"X-Forwarded-For":     "1.2.3.4",
		"X-Real-Ip":           "1.2.3.4",
		"X-Client-Ip":         "1.2.3.4",
		"X-Client-User-Agent": "spoofed-ua",
		"User-Agent":          "Mozilla/5.0 test",
	})
	if len(ip) != 1 || ip[0] != "127.0.0.1" {
		t.Fatalf("x-client-ip = %v, want the socket peer 127.0.0.1", ip)
	}
	if len(ua) != 1 || ua[0] != "Mozilla/5.0 test" {
		t.Fatalf("x-client-user-agent = %v, want the request User-Agent (inbound x-client-user-agent is ignored)", ua)
	}
}

func TestClientContextTrustedProxyHonoursForwardedFor(t *testing.T) {
	cases := []struct {
		name, trusted, xff, want string
	}{
		{"single forwarded client", "127.0.0.1", "9.9.9.9", "9.9.9.9"},
		{"client-prepended junk is ignored", "127.0.0.1", "6.6.6.6, 9.9.9.9", "9.9.9.9"},
		{"trusted hops are skipped", "127.0.0.0/8", "9.9.9.9, 127.0.0.2", "9.9.9.9"},
		{"unparseable entry falls back to peer", "127.0.0.1", "not-an-ip", "127.0.0.1"},
		{"untrusted peer ignores the header", "10.0.0.0/8", "9.9.9.9", "127.0.0.1"},
		{"no header uses the peer", "127.0.0.1", "", "127.0.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hdr := map[string]string{"X-Client-Ip": "1.2.3.4"}
			if tc.xff != "" {
				hdr["X-Forwarded-For"] = tc.xff
			}
			ip, _ := clientMD(t, tc.trusted, hdr)
			if len(ip) != 1 || ip[0] != tc.want {
				t.Fatalf("x-client-ip = %v, want %s", ip, tc.want)
			}
		})
	}
}

func TestClientContextClipsUserAgent(t *testing.T) {
	_, ua := clientMD(t, "", map[string]string{"User-Agent": strings.Repeat("a", 1000)})
	if len(ua) != 1 || len(ua[0]) != 256 {
		t.Fatalf("user agent length = %d, want clipped to 256", len(ua[0]))
	}
}
