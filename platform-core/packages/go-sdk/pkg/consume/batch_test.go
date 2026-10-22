package consume

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
)

type batchDLQ struct {
	mu   sync.Mutex
	recs []*kgo.Record
}

func (d *batchDLQ) PublishRecord(_ context.Context, r *kgo.Record) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recs = append(d.recs, r)
	return nil
}

func scriptedBatch(t *testing.T, h BatchHandler, dlq DLQProducer) (*BatchConsumer, *fakeClient) {
	t.Helper()
	fc := newFakeClient(fetchOf("orders", span{
		partition: 0, from: 0, count: 2,
		headers: []kgo.RecordHeader{{Key: "x-h", Value: []byte("hv")}},
	}))
	c, err := newBatchConsumer(fc, "g", h, dlq, fastBatchOpts())
	if err != nil {
		t.Fatal(err)
	}
	return c, fc
}

func TestBatchRetriesWithoutCommit(t *testing.T) {
	var fcRef *fakeClient
	var mu sync.Mutex
	var seen [][]string
	h := func(_ context.Context, rs []Record) error {
		mu.Lock()
		defer mu.Unlock()
		var vs []string
		for _, r := range rs {
			vs = append(vs, string(r.Value))
		}
		seen = append(seen, vs)
		if len(seen) == 1 {
			if _, commits := fcRef.snapshot(); len(commits) != 0 {
				t.Error("committed before the handler succeeded")
			}
			return errors.New("transient")
		}
		return nil
	}
	c, fc := scriptedBatch(t, h, &recordingDLQ{})
	fcRef = fc
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || len(seen[0]) != 2 || seen[0][0] != "p0-o0" || seen[1][1] != "p0-o1" || seen[1][0] != "p0-o0" {
		t.Fatalf("handler saw %v, want the same two records twice", seen)
	}
	if _, commits := fc.snapshot(); len(commits) != 1 {
		t.Fatalf("commits = %d, want exactly 1 after success", len(commits))
	}
}

func TestBatchPermanentGoesToDLQ(t *testing.T) {
	dlq := &recordingDLQ{}
	calls := 0
	h := func(context.Context, []Record) error { calls++; return Permanent(errors.New("poison")) }
	c, fc := scriptedBatch(t, h, dlq)
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (permanent is not retried)", calls)
	}
	recs := dlq.records()
	if len(recs) != 2 {
		t.Fatalf("dlq records = %d, want 2", len(recs))
	}
	for i, r := range recs {
		hs := map[string]string{}
		for _, h := range r.Headers {
			hs[h.Key] = string(h.Value)
		}
		if r.Topic != "orders.dlq" || hs[kafkax.HeaderDLQError] != "poison" || hs[kafkax.HeaderDLQSourceTopic] != "orders" || hs["x-h"] != "hv" {
			t.Fatalf("dlq record %d = topic %q headers %v", i, r.Topic, hs)
		}
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuf) lines(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(s.String()), "\n") {
		if l == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		out = append(out, m)
	}
	return out
}

func jsonLogger(buf *syncBuf) *slog.Logger {
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func wantFP(secret, key string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(key))
	return hex.EncodeToString(m.Sum(nil))[:12]
}

func keyedFetch(keys ...string) kgo.Fetches {
	var recs []*kgo.Record
	for i, k := range keys {
		recs = append(recs, &kgo.Record{Topic: "orders", Partition: 0, Offset: int64(i), Key: []byte(k), Value: []byte("val-" + k)})
	}
	return kgo.Fetches{{Topics: []kgo.FetchTopic{{Topic: "orders", Partitions: []kgo.FetchPartition{{Partition: 0, Records: recs}}}}}}
}

func headerMap(r *kgo.Record) map[string]string {
	hs := map[string]string{}
	for _, h := range r.Headers {
		hs[h.Key] = string(h.Value)
	}
	return hs
}

func fingerprinter(t *testing.T, secret string) *kafkax.Fingerprinter {
	t.Helper()
	f, err := kafkax.NewFingerprinter(secret)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestHandlerFailureLogsKeyFingerprint(t *testing.T) {
	fc := newFakeClient(keyedFetch("seller-123"))
	buf := &syncBuf{}
	o := fastOpts()
	o.Logger = jsonLogger(buf)
	o.KeyFP = fingerprinter(t, "s3cret")
	c, err := newConsumer(fc, "g", func(context.Context, Record) error { return errors.New("boom") }, &recordingDLQ{}, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := wantFP("s3cret", "seller-123")
	found := false
	for _, l := range buf.lines(t) {
		if l["key_fp"] == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("no log record with key_fp=%s:\n%s", want, buf.String())
	}
	if strings.Contains(buf.String(), "seller-123") {
		t.Fatalf("log output contains the raw key:\n%s", buf.String())
	}
}

func TestDeadLetterLogsFingerprintKeepsDLQKey(t *testing.T) {
	fc := newFakeClient(keyedFetch("seller-123"))
	buf := &syncBuf{}
	dlq := &recordingDLQ{}
	o := fastOpts()
	o.Logger = jsonLogger(buf)
	o.KeyFP = fingerprinter(t, "s3cret")
	c, _ := newConsumer(fc, "g", func(context.Context, Record) error { return errors.New("boom") }, dlq, o)
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	recs := dlq.records()
	if len(recs) != 1 || string(recs[0].Key) != "seller-123" {
		t.Fatalf("dlq records = %v, want one with key seller-123", recs)
	}
	want := wantFP("s3cret", "seller-123")
	found := false
	for _, l := range buf.lines(t) {
		if msg, _ := l["msg"].(string); strings.Contains(msg, "dead-letter") {
			found = true
			if l["key_fp"] != want {
				t.Errorf("dead-letter log key_fp = %v, want %s", l["key_fp"], want)
			}
			if _, has := l["key"]; has {
				t.Errorf("dead-letter log carries a key attribute: %v", l)
			}
		}
	}
	if !found {
		t.Fatalf("no dead-letter log record:\n%s", buf.String())
	}
}

func TestDeadLetterFailureLogsFingerprintOffsetHeld(t *testing.T) {
	fc := newFakeClient(keyedFetch("seller-123"))
	buf := &syncBuf{}
	o := fastOpts()
	o.Logger = jsonLogger(buf)
	o.KeyFP = fingerprinter(t, "s3cret")
	c, _ := newConsumer(fc, "g", func(context.Context, Record) error {
		return fmt.Errorf("user seller-123: %w", errors.New("boom"))
	}, &recordingDLQ{err: errors.New("down for seller-123")}, o)
	err := c.Run(context.Background())
	if err == nil {
		t.Fatal("Run returned nil with the DLQ down")
	}
	if strings.Contains(err.Error(), "seller-123") {
		t.Errorf("returned error contains the raw key: %v", err)
	}
	if _, commits := fc.snapshot(); len(commits) != 0 {
		t.Errorf("commits = %v, want none", commitOffsets(commits))
	}
	if strings.Contains(buf.String(), "seller-123") {
		t.Fatalf("log output contains the raw key:\n%s", buf.String())
	}
	want := wantFP("s3cret", "seller-123")
	found := false
	for _, l := range buf.lines(t) {
		if l["key_fp"] == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("no log record with key_fp=%s:\n%s", want, buf.String())
	}
}

func TestErrorTextIsScrubbedOfKey(t *testing.T) {
	fc := newFakeClient(keyedFetch("seller-123"))
	buf := &syncBuf{}
	dlq := &recordingDLQ{}
	o := fastOpts()
	o.Logger = jsonLogger(buf)
	o.KeyFP = fingerprinter(t, "s3cret")
	c, _ := newConsumer(fc, "g", func(context.Context, Record) error {
		return fmt.Errorf("user seller-123: %w", errors.New("boom"))
	}, dlq, o)
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := "user [key_fp:" + wantFP("s3cret", "seller-123") + "]: boom"
	recs := dlq.records()
	if len(recs) != 1 {
		t.Fatalf("dlq records = %d, want 1", len(recs))
	}
	if got := headerMap(recs[0])[kafkax.HeaderDLQError]; got != want {
		t.Errorf("dlq-error = %q, want %q", got, want)
	}
	errAttr := false
	for _, l := range buf.lines(t) {
		if e, ok := l["err"].(string); ok {
			errAttr = true
			if e != want {
				t.Errorf("log err = %q, want %q", e, want)
			}
		}
	}
	if !errAttr {
		t.Fatalf("no log record with an err attribute:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "seller-123") {
		t.Fatalf("log output contains the raw key:\n%s", buf.String())
	}
}

func TestBatchLogsFingerprintsOnly(t *testing.T) {
	fc := newFakeClient(keyedFetch("a-1", "b-2"))
	buf := &syncBuf{}
	dlq := &recordingDLQ{}
	o := fastBatchOpts()
	o.Logger = jsonLogger(buf)
	o.KeyFP = fingerprinter(t, "s3cret")
	c, err := newBatchConsumer(fc, "g", func(context.Context, []Record) error {
		return errors.New("failed for a-1 and b-2")
	}, dlq, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	out := buf.String()
	for _, raw := range []string{"a-1", "b-2"} {
		if strings.Contains(out, raw) {
			t.Fatalf("log output contains raw key %s:\n%s", raw, out)
		}
	}
	for _, raw := range []string{"a-1", "b-2"} {
		if fp := wantFP("s3cret", raw); !strings.Contains(out, fp) {
			t.Errorf("log output lacks key_fp %s of %s:\n%s", fp, raw, out)
		}
	}
	for _, r := range dlq.records() {
		if strings.Contains(headerMap(r)[kafkax.HeaderDLQError], "-") && (strings.Contains(headerMap(r)[kafkax.HeaderDLQError], "a-1") || strings.Contains(headerMap(r)[kafkax.HeaderDLQError], "b-2")) {
			t.Errorf("dlq-error header contains a raw key: %q", headerMap(r)[kafkax.HeaderDLQError])
		}
	}
}

func TestNilFingerprinterUsesRandomSecret(t *testing.T) {
	fc := newFakeClient(keyedFetch("seller-123"))
	buf := &syncBuf{}
	o := fastOpts()
	o.Logger = jsonLogger(buf)
	c, _ := newConsumer(fc, "g", func(context.Context, Record) error { return errors.New("boom") }, &recordingDLQ{}, o)
	if err := runUntil(t, c.Run, func() { fc.waitForEvent(t, "commit", 1) }); err != nil {
		t.Fatalf("Run: %v", err)
	}
	sum := sha256.Sum256([]byte("seller-123"))
	unkeyed := hex.EncodeToString(sum[:])[:12]
	hex12 := regexp.MustCompile(`^[0-9a-f]{12}$`)
	found := false
	for _, l := range buf.lines(t) {
		fp, ok := l["key_fp"].(string)
		if !ok {
			continue
		}
		found = true
		if !hex12.MatchString(fp) {
			t.Errorf("key_fp = %q, want 12 hex", fp)
		}
		if fp == unkeyed {
			t.Errorf("key_fp %q is the unkeyed SHA-256 prefix", fp)
		}
	}
	if !found {
		t.Fatalf("no key_fp in log:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "seller-123") {
		t.Fatalf("log output contains the raw key:\n%s", buf.String())
	}
}
