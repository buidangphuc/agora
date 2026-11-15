package app_test

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/store"

	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-domain/generated/platform/identity/v1"
)

const erasureDLQTopic = "identity.events.team-domain.dlq"

func erasureSettings(t *testing.T, s config.Settings) config.Settings {
	t.Helper()
	f := reflect.ValueOf(&s.Events).Elem().FieldByName("ErasureGroup")
	if f.IsValid() {
		f.SetString("team-domain-erasure")
	}
	return s
}

func erasedEnvelope(t *testing.T, eventID, user string) []byte {
	t.Helper()
	payload, err := proto.Marshal(&identityv1.UserErased{UserId: user, ErasedAt: timestamppb.Now()})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: eventID, Type: "platform.identity.v1.UserErased", OccurredAt: timestamppb.Now(), Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func listingExists(t *testing.T, pool *pgxpool.Pool, id string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM listings WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("count listing %s: %v", id, err)
	}
	return n > 0
}

func TestErasureRunConsumesUserErasedEvents(t *testing.T) {
	// scenario: A poison erasure event does not block seller hold (erasure consumer is wired and running)
	url := migratedURL(t)
	pool := openPool(t, url)
	seedListing(t, pool, store.Listing{ID: "L-erase", Title: "t", Currency: "VND", Status: "published", SellerID: "seller-erase", Stock: 1})
	brokers := testkit.Redpanda(t)
	r := start(t, erasureSettings(t, domainSettings(testkit.FreePort(t), url, brokers)))
	waitServing(t, r)

	testkit.Eventually(t, 60*time.Second, func() bool {
		testkit.Produce(t, brokers, identityTopic, "seller-erase", erasedEnvelope(t, "evt-erase-1", "seller-erase"))
		return !listingExists(t, pool, "L-erase")
	})
}

func TestErasureMalformedEventGoesToTeamDomainDLQ(t *testing.T) {
	// scenario: Malformed event goes to the DLQ (identity.events.team-domain.dlq)
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	r := start(t, erasureSettings(t, domainSettings(testkit.FreePort(t), url, brokers)))
	waitServing(t, r)

	poison := []byte{0xff, 0xff, 0xff, 0xff}
	testkit.Produce(t, brokers, identityTopic, "poison-erasure", poison)
	recs := testkit.ReadAll(t, brokers, erasureDLQTopic, 1, 60*time.Second)
	if len(recs) != 1 {
		t.Fatalf("DLQ records = %d, want 1", len(recs))
	}
}

func TestErasurePoisonEventDoesNotBlockSellerHold(t *testing.T) {
	// scenario: A poison erasure event does not block seller hold (separate consumer group + DLQ)
	url := migratedURL(t)
	pool := openPool(t, url)
	seedListing(t, pool, store.Listing{ID: "L-hold-after-poison", Title: "t", Currency: "VND", Status: "published", SellerID: "seller-susp2", Stock: 1})
	brokers := testkit.Redpanda(t)
	r := start(t, erasureSettings(t, domainSettings(testkit.FreePort(t), url, brokers)))
	waitServing(t, r)

	testkit.Produce(t, brokers, identityTopic, "poison-erasure", []byte{0xff, 0xff, 0xff, 0xff})
	testkit.ReadAll(t, brokers, erasureDLQTopic, 1, 60*time.Second)
	testkit.Eventually(t, 60*time.Second, func() bool {
		testkit.Produce(t, brokers, identityTopic, "seller-susp2", statusEnvelope(t, "evt-suspend-2", "seller-susp2"))
		return listingStatus(t, pool, "L-hold-after-poison") == "held"
	})
}

func TestErasureComposeWiring(t *testing.T) {
	// scenario: Malformed event goes to the DLQ (DLQ topic created by topic init, group set in compose)
	root := serviceRoot(t)
	for _, c := range []struct {
		path string
		want []string
	}{
		{filepath.Join(root, "docker-compose.local.yaml"), []string{"KAFKA_ERASURE_GROUP"}},
		{filepath.Join(root, "..", "docker-compose.services.yaml"), []string{"KAFKA_ERASURE_GROUP", erasureDLQTopic}},
	} {
		raw, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range c.want {
			if !strings.Contains(string(raw), w) {
				t.Errorf("%s does not mention %s", c.path, w)
			}
		}
	}
}
