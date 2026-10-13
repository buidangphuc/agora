package index_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buidangphuc/team-search/internal/index"
)

// Ping backs the OpenSearch health checker: nil when the cluster answers, an
// error when it does not.
func TestPing(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	idx, err := index.New(srv.URL, "listings")
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Ping(context.Background()); err != nil {
		t.Fatalf("Ping against a healthy cluster: %v", err)
	}
	status = http.StatusServiceUnavailable
	if err := idx.Ping(context.Background()); err == nil {
		t.Fatal("Ping must fail when the cluster answers 503")
	}
	srv.Close()
	if err := idx.Ping(context.Background()); err == nil {
		t.Fatal("Ping must fail when the cluster is unreachable")
	}
}
