package testkit_test

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

// Compile-time checks live in the external test package: consume's own internal
// tests import testkit, so testkit itself must not import consume.
var (
	_ outbox.Publisher    = (*testkit.Recorder)(nil)
	_ consume.DLQProducer = (*testkit.Recorder)(nil)
)

func TestRecorderFailOn(t *testing.T) {
	r := testkit.NewRecorder()
	r.FailOn("k1")
	ctx := context.Background()

	if err := r.Publish(ctx, "t", []byte("k1"), []byte("v1")); !errors.Is(err, testkit.ErrInjected) {
		t.Fatalf("k1 error = %v, want ErrInjected", err)
	}
	if err := r.Publish(ctx, "t", []byte("k2"), []byte("v2")); err != nil {
		t.Fatalf("k2: %v", err)
	}
	rec := &kgo.Record{Topic: "dlq", Key: []byte("k1")}
	if err := r.PublishRecord(ctx, rec); !errors.Is(err, testkit.ErrInjected) {
		t.Fatalf("PublishRecord k1 error = %v, want ErrInjected", err)
	}

	got := r.Records()
	if len(got) != 1 || got[0].Topic != "t" || string(got[0].Key) != "k2" || string(got[0].Value) != "v2" {
		t.Fatalf("records = %+v, want only k2", got)
	}
}

func TestRecorderKeepsHeaders(t *testing.T) {
	r := testkit.NewRecorder()
	h := []kgo.RecordHeader{{Key: "a", Value: []byte("b")}}
	if err := r.PublishRecord(context.Background(), &kgo.Record{Topic: "dlq", Key: []byte("k"), Value: []byte("v"), Headers: h}); err != nil {
		t.Fatal(err)
	}
	got := r.Records()
	if len(got) != 1 || len(got[0].Headers) != 1 || got[0].Headers[0].Key != "a" {
		t.Fatalf("records = %+v", got)
	}
}

func TestFreePortIsBindable(t *testing.T) {
	p := testkit.FreePort(t)
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		t.Fatalf("listen on %d: %v", p, err)
	}
	l.Close()
}

func TestEventually(t *testing.T) {
	start := time.Now()
	n := 0
	testkit.Eventually(t, 2*time.Second, func() bool { n++; return n >= 3 })
	if n < 3 || time.Since(start) > time.Second {
		t.Fatalf("n=%d after %v", n, time.Since(start))
	}
}
