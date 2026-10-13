package testkit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kgo"
)

// keepTB lets the shared container outlive the test that first asks for it:
// its cleanups are dropped, and testcontainers' reaper removes the container
// when the test process exits.
type keepTB struct{ testing.TB }

func (keepTB) Cleanup(func()) {}

var (
	sharedOnce sync.Once
	sharedURL  string
)

// SharedPostgres returns the connection URL of one Postgres container shared by the
// whole test binary, started on first use. Isolate tests with Schema. Docker handling
// matches Postgres.
func SharedPostgres(t testing.TB) string {
	t.Helper()
	sharedOnce.Do(func() { sharedURL = Postgres(keepTB{t}) })
	if sharedURL == "" {
		t.Skip("shared postgres did not start")
	}
	return sharedURL
}

// Schema creates a fresh schema on the server at dsn and returns a DSN whose
// search_path is that schema. The schema is dropped, with CASCADE, when the test ends.
func Schema(t testing.TB, dsn string) string {
	t.Helper()
	ctx := context.Background()

	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("schema name: %v", err)
	}
	name := "testkit_" + hex.EncodeToString(b[:])

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect for schema: %v", err)
	}
	defer admin.Close(ctx)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), dsn)
		if err != nil {
			t.Logf("drop schema %s: %v", name, err)
			return
		}
		defer c.Close(context.Background())
		if _, err := c.Exec(context.Background(), "DROP SCHEMA "+name+" CASCADE"); err != nil {
			t.Logf("drop schema %s: %v", name, err)
		}
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	return u.String()
}

// Produce writes one record to topic on brokers and waits for the acknowledgement.
func Produce(t testing.TB, brokers, topic, key string, value []byte) {
	t.Helper()
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("produce client: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: []byte(key), Value: value}).FirstErr(); err != nil {
		t.Fatalf("produce to %s: %v", topic, err)
	}
}

// ReadAll reads topic from the beginning with a group-less client and returns the
// first n records. It fails the test if fewer than n arrive within timeout.
func ReadAll(t testing.TB, brokers, topic string, n int, timeout time.Duration) []*kgo.Record {
	t.Helper()
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		t.Fatalf("read client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var got []*kgo.Record
	for len(got) < n {
		fetches := client.PollRecords(ctx, n-len(got))
		if ctx.Err() != nil {
			t.Fatalf("read %s: got %d of %d records within %v", topic, len(got), n, timeout)
		}
		fetches.EachRecord(func(r *kgo.Record) { got = append(got, r) })
	}
	return got
}
