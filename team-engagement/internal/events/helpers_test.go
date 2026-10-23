package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-engagement/generated/platform/listing/v1"
	"github.com/buidangphuc/team-engagement/migrations"
)

const (
	factsTable  = "engagement_outbox_events"
	factsTopic  = "engagement.events"
	listingType = "platform.listing.v1.ListingChanged"
	statusType  = "platform.listing.v1.ListingStatusChanged"
)

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

const (
	published = listingv1.ListingStatus_LISTING_STATUS_PUBLISHED
	draft     = listingv1.ListingStatus_LISTING_STATUS_DRAFT
	created   = listingv1.ChangeType_CHANGE_TYPE_CREATED
	updated   = listingv1.ChangeType_CHANGE_TYPE_UPDATED
	deleted   = listingv1.ChangeType_CHANGE_TYPE_DELETED
)

func envelopeOf(t *testing.T, eventID, typ string, msg proto.Message, at time.Time) []byte {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: eventID, Type: typ, OccurredAt: timestamppb.New(at), Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func changed(t *testing.T, id, seller string, st listingv1.ListingStatus, ct listingv1.ChangeType, at time.Time) []byte {
	t.Helper()
	return envelopeOf(t, "evt-"+id, listingType, &listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: id, SellerId: seller, Status: st},
		ChangeType: ct,
	}, at)
}

func userCtx(id string) context.Context {
	return auth.WithPrincipal(context.Background(),
		auth.Principal{ID: id, Type: auth.TypeUser, Scopes: []string{"engagement:write"}})
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) records(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if line == "" {
			continue
		}
		m := map[string]any{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("bad log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func (b *syncBuffer) find(t *testing.T, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range b.records(t) {
		if r["msg"] == msg {
			out = append(out, r)
		}
	}
	return out
}

func factsDB(t *testing.T) (*pgxpool.Pool, *outbox.Store) {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.FS); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ob, err := outbox.NewStore(pool, factsTable)
	if err != nil {
		t.Fatal(err)
	}
	return pool, ob
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func envelopeOfRecord(t *testing.T, raw []byte) *eventsv1.EventEnvelope {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(raw, &env); err != nil {
		t.Fatalf("record value is not an EventEnvelope: %v", err)
	}
	return &env
}
