package app_test

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-engagement/internal/app"
	"github.com/buidangphuc/team-engagement/internal/config"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/migrations"
)

const unreachableDB = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func settings(t *testing.T, env, databaseURL string) config.Settings {
	t.Helper()
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: freePort(t), ShutdownGrace: 10 * time.Second},
		Postgres: sdkconfig.Postgres{URL: databaseURL, MaxConns: 4},
		Kafka:    sdkconfig.Kafka{Brokers: "127.0.0.1:1"},
		Outbox:   sdkconfig.Outbox{PollInterval: 20 * time.Millisecond, BatchSize: 10, ClaimLock: time.Minute},
		Events: config.Events{
			ConsumerGroup: "team-engagement-feed",
			ListingTopic:  "listing.events",
			EventsTopic:   "engagement.events",
		},
		Upstream: config.Upstream{CallTimeoutSeconds: 2},
	}
}

// Boot never opens the port before the database is known to be reachable, in any
// ENV: team-engagement has no in-memory fallback.
func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	for _, env := range []string{"staging", "stage", "prod", "production", "local"} {
		t.Run(env, func(t *testing.T) {
			s := settings(t, env, unreachableDB)
			if err := app.Run(context.Background(), s); err == nil {
				t.Fatalf("Run succeeded in ENV=%s with an unreachable database", env)
			}
			l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port)))
			if err != nil {
				t.Fatalf("port opened before the boot check failed: %v", err)
			}
			_ = l.Close()
		})
	}
}

// fakeProducer records every publish except those for the key in failKey, which
// fail with an error the relayer treats as permanent for the record. It is the
// service's Producer: the outbox publisher and the consumer's DLQ producer.
type fakeProducer struct {
	*testkit.Recorder
	failKey string
}

func newFakeProducer(failKey string) *fakeProducer {
	return &fakeProducer{Recorder: testkit.NewRecorder(), failKey: failKey}
}

func (p *fakeProducer) PublishRecord(ctx context.Context, rec *kgo.Record) error {
	if string(rec.Key) == p.failKey {
		return outbox.Permanent(errors.New("record rejected"))
	}
	return p.Recorder.PublishRecord(ctx, rec)
}

func (p *fakeProducer) Close() {}

func (p *fakeProducer) published() ([]string, []string) {
	var topics, keys []string
	for _, r := range p.Records() {
		topics = append(topics, r.Topic)
		keys = append(keys, string(r.Key))
	}
	return topics, keys
}

func enqueue(t *testing.T, pool *pgxpool.Pool, id, key string) {
	t.Helper()
	ctx := context.Background()
	ob, err := outbox.NewStore(pool, store.OutboxTable)
	if err != nil {
		t.Fatal(err)
	}
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return ob.EnqueueTx(ctx, tx, outbox.Row{
			EventID: id, AggregateType: "Listing", AggregateID: key,
			EventType: "platform.engagement.v1.FavoriteAdded", Payload: []byte("envelope-" + id),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func rowState(t *testing.T, pool *pgxpool.Pool, id string) (status string, attempts int, lastErr string) {
	t.Helper()
	var e *string
	if err := pool.QueryRow(context.Background(),
		`SELECT status, attempts, error FROM `+store.OutboxTable+` WHERE event_id = $1`, id).Scan(&status, &attempts, &e); err != nil {
		t.Fatal(err)
	}
	if e != nil {
		lastErr = *e
	}
	return status, attempts, lastErr
}

// Two rows are enqueued and publishing the first fails with an error that is
// permanent for the record: the second (another aggregate) is published, and the
// first is parked at once with its error kept. go-sdk v1 has no attempt limit
// (OUTBOX_MAX_ATTEMPTS is gone): only a permanent record error parks a row,
// everything else retries.
func TestOutboxRelayedViaSDK(t *testing.T) {
	ctx := context.Background()
	url := testkit.Postgres(t)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := migrate.Up(ctx, url, migrations.Files); err != nil {
		t.Fatal(err)
	}

	enqueue(t, pool, "evt-a", "A")
	enqueue(t, pool, "evt-b", "B")

	s := settings(t, "local", url)
	prod := newFakeProducer("A")
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- app.RunWithOptions(runCtx, s, app.Options{
			NewProducer: func(sdkconfig.Kafka, ...kgo.Opt) (app.Producer, error) { return prod, nil },
			LogOut:      io.Discard,
		})
	}()

	deadline := time.Now().Add(30 * time.Second)
	for {
		if st, _, _ := rowState(t, pool, "evt-b"); st == "published" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second row was not published while the first kept failing")
		}
		time.Sleep(50 * time.Millisecond)
	}
	topics, keys := prod.published()
	if len(keys) != 1 || keys[0] != "B" || topics[0] != "engagement.events" {
		t.Fatalf("published topics=%v keys=%v, want [engagement.events] [B]", topics, keys)
	}

	for {
		st, attempts, msg := rowState(t, pool, "evt-a")
		if st == "failed" {
			if attempts != 1 || msg == "" {
				t.Fatalf("parked with attempts=%d error=%q, want one attempt and the error kept", attempts, msg)
			}
			break
		}
		if st != "pending" {
			t.Fatalf("first row status = %q while retrying", st)
		}
		if time.Now().After(deadline) {
			t.Fatalf("first row not parked in time (status=%s attempts=%d)", st, attempts)
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("app did not stop")
	}
}
