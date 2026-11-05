package app

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

// Seller onboarding: the first grant emits one event
func TestBecomeSellerEnqueuesUserRoleGranted(t *testing.T) {
	url := migratedURL(t)
	s := logSettings(t, url)
	s.SellerTerms.Versions = []string{"V1", "V0"}
	s.Upstream.AuditAddr = startAudit(t, &fakeAudit{})
	r := start(t, s)
	waitServing(t, r)
	userID, reg := registerBuyer(t, r, "sally")
	auth := identityv1.NewAuthServiceClient(r.conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	userCtx := asSessionUser(ctx, t, userID, reg.GetToken())

	count := func() int {
		return queryInt(t, url, `SELECT count(*) FROM identity_outbox_events WHERE event_type = $1 AND aggregate_id = $2`,
			"platform.identity.v1.UserRoleGranted", userID)
	}

	if _, err := auth.BecomeSeller(userCtx, &identityv1.BecomeSellerRequest{TermsVersion: "V1"}); err != nil {
		t.Fatalf("BecomeSeller: %v", err)
	}
	if n := count(); n != 1 {
		t.Fatalf("UserRoleGranted rows = %d, want 1", n)
	}

	var payload []byte
	if err := dial(t, url).QueryRow(context.Background(),
		`SELECT payload FROM identity_outbox_events WHERE event_type = $1 AND aggregate_id = $2`,
		"platform.identity.v1.UserRoleGranted", userID).Scan(&payload); err != nil {
		t.Fatalf("read outbox payload: %v", err)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		t.Fatalf("outbox payload is not an EventEnvelope: %v", err)
	}
	if env.GetType() != "platform.identity.v1.UserRoleGranted" || env.GetEventId() == "" {
		t.Fatalf("envelope: %+v", &env)
	}
	var ev identityv1.UserRoleGranted
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if ev.GetUserId() != userID || ev.GetRole() != "seller" || ev.GetGrantedAt() == nil || ev.GetGrantedAt().AsTime().IsZero() {
		t.Fatalf("payload: %+v", &ev)
	}

	if _, err := auth.BecomeSeller(userCtx, &identityv1.BecomeSellerRequest{TermsVersion: "V1"}); err != nil {
		t.Fatalf("second BecomeSeller: %v", err)
	}
	if n := count(); n != 1 {
		t.Fatalf("UserRoleGranted rows after second call = %d, want 1", n)
	}
}

// Seller onboarding: an unknown terms version is refused (wired SELLER_TERMS_VERSIONS)
func TestBecomeSellerWiredTermsRefuseOlderVersion(t *testing.T) {
	url := migratedURL(t)
	s := logSettings(t, url)
	s.SellerTerms.Versions = []string{"V1", "V0"}
	s.Upstream.AuditAddr = startAudit(t, &fakeAudit{})
	r := start(t, s)
	waitServing(t, r)
	userID, reg := registerBuyer(t, r, "tom")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := identityv1.NewAuthServiceClient(r.conn).BecomeSeller(asSessionUser(ctx, t, userID, reg.GetToken()),
		&identityv1.BecomeSellerRequest{TermsVersion: "V0"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("older version = %v, want InvalidArgument", err)
	}
	if n := queryInt(t, url, `SELECT count(*) FROM identity_outbox_events WHERE event_type = $1`, "platform.identity.v1.UserRoleGranted"); n != 0 {
		t.Fatalf("rows = %d, want 0", n)
	}
}
