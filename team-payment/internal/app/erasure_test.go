package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-payment/generated/platform/identity/v1"
	"github.com/buidangphuc/team-payment/internal/config"
)

const (
	identityTopic = "identity.events"
	erasureDLQ    = "identity.events.payment.dlq"
	maskText      = "Đã xóa"
)

func erasureSettings(port int, url, brokers string) config.Settings {
	s := paySettings(port, url, brokers)
	s.Events.ErasureGroup = "team-payment.erasure"
	s.Events.ErasureDLQ = erasureDLQ
	return s
}

func identityEvent(t *testing.T, eventID, typ string, payload proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, Payload: b})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func erased(t *testing.T, eventID, user string) []byte {
	return identityEvent(t, eventID, "platform.identity.v1.UserErased", &identityv1.UserErased{UserId: user, ErasedAt: timestamppb.Now()})
}

func seedPayout(t *testing.T, url, id, seller string) {
	t.Helper()
	if _, err := dial(t, url).Exec(context.Background(),
		`INSERT INTO payout_requests (id, seller_id, amount, bank_code, account_number, account_name, status)
		 VALUES ($1,$2,1000,'VCB','0123456789','NGUYEN VAN A',1)`, id, seller); err != nil {
		t.Fatalf("seed payout: %v", err)
	}
}

func isMasked(t *testing.T, url, id string) bool {
	t.Helper()
	var n int
	if err := dial(t, url).QueryRow(context.Background(),
		`SELECT count(*) FROM payout_requests WHERE id=$1 AND account_number=$2 AND account_name=$2`, id, maskText).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func TestErasureConsumerMasksPayoutsFromIdentityTopic(t *testing.T) {
	// PER-1, PER-2: the consumer is wired beside the settlement consumer
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	seedPayout(t, url, "p-s", "S")
	seedPayout(t, url, "p-t", "T")
	r := start(t, erasureSettings(testkit.FreePort(t), url, brokers))
	waitServing(t, r)

	testkit.Eventually(t, 90*time.Second, func() bool {
		testkit.Produce(t, brokers, identityTopic, "k", erased(t, "evt-erase-s", "S"))
		return isMasked(t, url, "p-s")
	})
	if isMasked(t, url, "p-t") {
		t.Fatal("PER-2 another seller's payout was masked")
	}
	if text := dlqText(t, brokers, erasureDLQ); strings.Contains(text, "evt-erase-s") {
		t.Fatalf("PER-3 a valid event reached the DLQ: %s", text)
	}
}

func TestErasureConsumerParksMalformed(t *testing.T) {
	// PER-5
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	seedPayout(t, url, "p-s", "S")
	r := start(t, erasureSettings(testkit.FreePort(t), url, brokers))
	waitServing(t, r)

	testkit.Eventually(t, 90*time.Second, func() bool {
		testkit.Produce(t, brokers, identityTopic, "k", erased(t, "evt-empty-user", ""))
		return strings.Contains(dlqText(t, brokers, erasureDLQ), "evt-empty-user")
	})
	if isMasked(t, url, "p-s") {
		t.Fatal("PER-5 a malformed event masked a payout")
	}
}

func TestErasureConsumerIgnoresOtherEvents(t *testing.T) {
	// PER-7: UserRegistered for S precedes UserErased for T on the same key; once T is
	// masked the earlier record has been handled
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	seedPayout(t, url, "p-s", "S")
	seedPayout(t, url, "p-t", "T")
	r := start(t, erasureSettings(testkit.FreePort(t), url, brokers))
	waitServing(t, r)

	testkit.Eventually(t, 90*time.Second, func() bool {
		testkit.Produce(t, brokers, identityTopic, "k", identityEvent(t, "evt-registered", "platform.identity.v1.UserRegistered", &identityv1.UserRegistered{UserId: "S"}))
		testkit.Produce(t, brokers, identityTopic, "k", identityEvent(t, "evt-status", "platform.identity.v1.UserStatusChanged",
			&identityv1.UserStatusChanged{UserId: "S", Status: identityv1.UserStatus_USER_STATUS_BANNED, ChangedAt: timestamppb.Now()}))
		testkit.Produce(t, brokers, identityTopic, "k", erased(t, "evt-erase-t", "T"))
		return isMasked(t, url, "p-t")
	})
	if isMasked(t, url, "p-s") {
		t.Fatal("PER-7 an unrelated identity event masked a payout")
	}
	if text := dlqText(t, brokers, erasureDLQ); strings.Contains(text, "evt-registered") || strings.Contains(text, "evt-status") {
		t.Fatalf("PER-7 an ignored event reached the DLQ: %s", text)
	}
}
