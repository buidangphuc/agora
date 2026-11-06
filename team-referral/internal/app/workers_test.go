package app_test

import (
	"bytes"
	"context"
	"net"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-referral/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-referral/generated/platform/identity/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/app"
	"github.com/buidangphuc/team-referral/migrations"
)

var workerFrames = []string{
	"team-referral/internal/consumer",
	"team-referral/internal/referral.(*Sweeper)",
	"go-sdk/pkg/outbox",
}

func workerGoroutines() []string {
	var buf bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&buf, 1)
	var found []string
	for _, block := range strings.Split(buf.String(), "\n\n") {
		for _, f := range workerFrames {
			if strings.Contains(block, f) {
				found = append(found, block)
				break
			}
		}
	}
	return found
}

func TestShutdownStopsWorkers(t *testing.T) {
	url := testkit.Postgres(t)
	s := withBrokers(settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}), testkit.Redpanda(t))
	s.ShutdownGrace = 2 * time.Second
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := startApp(t, s)
	waitServing(t, r)
	if len(workerGoroutines()) == 0 {
		t.Fatal("no consumer, relayer or sweeper goroutine is running while the app serves")
	}

	r.cancel()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s of cancellation with SHUTDOWN_GRACE=2s")
	}
	testkit.Eventually(t, 2*time.Second, func() bool { return len(workerGoroutines()) == 0 })
	if left := workerGoroutines(); len(left) != 0 {
		t.Fatalf("worker goroutines remain after Run returned:\n%s", strings.Join(left, "\n\n"))
	}
}

func TestBadKafkaConfigFailsBeforeTheGRPCListener(t *testing.T) {
	url := testkit.Postgres(t)
	port := testkit.FreePort(t)
	s := withBrokers(settings(port, "production", sdkconfig.Postgres{URL: url}), "")

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil with no Kafka brokers configured")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail with no Kafka brokers; it serves without Kafka")
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("gRPC port was bound by the failed boot: %v", err)
	}
	_ = l.Close()
}

func envelopeBytes(t *testing.T, id, typ string, payload proto.Message) []byte {
	t.Helper()
	body, err := proto.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: id, Type: typ, Payload: body})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAppAttributesFromIdentityEventsAndDeadLettersMalformedOnes(t *testing.T) {
	url := testkit.Postgres(t)
	brokers := testkit.Redpanda(t)
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := startApp(t, withBrokers(settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}), brokers))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	created, err := referralv1.NewReferralServiceClient(r.conn).CreateReferralCode(principal(ctx, "alice"), &referralv1.CreateReferralCodeRequest{})
	if err != nil {
		t.Fatalf("CreateReferralCode: %v", err)
	}

	bad, _ := proto.Marshal(&eventsv1.EventEnvelope{EventId: "ev-bad", Type: "platform.identity.v1.UserRegistered", Payload: []byte{0xff, 0xff, 0xff}})
	testkit.Produce(t, brokers, "identity.events", "mallory", bad)
	testkit.Produce(t, brokers, "identity.events", "bob", envelopeBytes(t, "ev-ok", "platform.identity.v1.UserRegistered",
		&identityv1.UserRegistered{UserId: "bob", ReferralCode: created.GetCode(), RegisteredAt: timestamppb.Now()}))

	dlq := testkit.ReadAll(t, brokers, "referral.identity.dlq", 1, 30*time.Second)
	if len(dlq) != 1 {
		t.Fatalf("dead-letter records = %d, want 1", len(dlq))
	}
	out := testkit.ReadAll(t, brokers, "referral.events", 1, 10*time.Second)
	if len(out) != 1 || string(out[0].Key) != "bob" {
		t.Fatalf("referral.events records = %v, want one keyed by bob", out)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(out[0].Value, &env); err != nil || env.GetType() != "platform.referral.v1.ReferralAttributed" {
		t.Fatalf("envelope = %v, %v", &env, err)
	}
}
