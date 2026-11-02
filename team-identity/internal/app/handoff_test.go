package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

const (
	hsSeller  = identityv1.HandoffApp_HANDOFF_APP_SELLER
	hsBadCode = "B4d-handoff-code-never-minted-zzzzzzzzzzzzz"
)

func tokenSID(t *testing.T, token string) string {
	t.Helper()
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		t.Fatalf("parse token: %v", err)
	}
	sid, _ := claims["sid"].(string)
	if sid == "" {
		t.Fatal("token carries no sid")
	}
	return sid
}

func asSessionUser(t *testing.T, ctx context.Context, id, token string) context.Context {
	t.Helper()
	return metadata.AppendToOutgoingContext(asUser(ctx, id), "x-principal-sid", tokenSID(t, token))
}

func TestHandoffRPCWiredOverTheServer(t *testing.T) {
	// scenarios: Peek needs no token; Anonymous caller cannot mint; Service principal cannot mint
	url := migratedURL(t)
	s := logSettings(t, url)
	s.Upstream.AuditAddr = startAudit(t, &fakeAudit{})
	r := start(t, s)
	waitServing(t, r)
	userID, reg := registerBuyer(t, r, "gina")
	auth := identityv1.NewAuthServiceClient(r.conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := auth.CreateHandoffCode(ctx, &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous CreateHandoffCode = %v, want Unauthenticated", err)
	}

	svcCtx := metadata.AppendToOutgoingContext(ctx, "x-principal-id", "svc-orders", "x-principal-type", "service")
	if _, err := auth.CreateHandoffCode(svcCtx, &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("service CreateHandoffCode = %v, want PermissionDenied", err)
	}

	minted, err := auth.CreateHandoffCode(asSessionUser(t, ctx, userID, reg.GetToken()), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
	if err != nil {
		t.Fatalf("user CreateHandoffCode: %v", err)
	}
	if len(minted.GetCode()) != 43 {
		t.Fatalf("code length = %d, want 43", len(minted.GetCode()))
	}

	if _, err := auth.PeekHandoffCode(ctx, &identityv1.PeekHandoffCodeRequest{Code: hsBadCode, TargetApp: hsSeller}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous Peek of an unknown code = %v, want Unauthenticated", err)
	}
	if _, err := auth.RedeemHandoffCode(ctx, &identityv1.RedeemHandoffCodeRequest{Code: hsBadCode, TargetApp: hsSeller}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous Redeem of an unknown code = %v, want Unauthenticated", err)
	}
}

func TestHandoffCodeNeverReachesLogs(t *testing.T) {
	// scenario: Code absent from logs even on failure
	url := migratedURL(t)
	logs := captureLogs(t)
	s := logSettings(t, url)
	s.LogLevel = "debug"
	s.Upstream.AuditAddr = startAudit(t, &fakeAudit{})
	r := start(t, s)
	waitServing(t, r)
	userID, reg := registerBuyer(t, r, "hank")
	auth := identityv1.NewAuthServiceClient(r.conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	minted, err := auth.CreateHandoffCode(asSessionUser(t, ctx, userID, reg.GetToken()), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
	if err != nil {
		t.Fatalf("CreateHandoffCode: %v", err)
	}
	code := minted.GetCode()
	_, _ = auth.PeekHandoffCode(ctx, &identityv1.PeekHandoffCodeRequest{Code: code, TargetApp: hsSeller})
	_, _ = auth.RedeemHandoffCode(ctx, &identityv1.RedeemHandoffCodeRequest{Code: code, TargetApp: hsSeller})
	if _, err := auth.RedeemHandoffCode(ctx, &identityv1.RedeemHandoffCodeRequest{Code: code, TargetApp: hsSeller}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("second redeem = %v, want Unauthenticated", err)
	}
	if _, err := auth.RedeemHandoffCode(ctx, &identityv1.RedeemHandoffCodeRequest{Code: hsBadCode, TargetApp: hsSeller}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bad redeem = %v, want Unauthenticated", err)
	}
	_, _ = auth.PeekHandoffCode(ctx, &identityv1.PeekHandoffCodeRequest{Code: "short", TargetApp: hsSeller})

	if _, err := identityv1.NewSessionServiceClient(r.conn).ListSessions(asUser(metadataRequestID(ctx, "handoff-sentinel"), userID), &identityv1.ListSessionsRequest{}); err != nil {
		t.Fatalf("sentinel ListSessions: %v", err)
	}
	testkit.Eventually(t, 10*time.Second, func() bool {
		for _, l := range logs.Lines() {
			if l["request_id"] == "handoff-sentinel" {
				return true
			}
		}
		return false
	})

	out := logs.String()
	secrets := map[string]string{}
	for name, c := range map[string]string{"minted code": code, "bad code": hsBadCode} {
		sum := sha256.Sum256([]byte(c))
		secrets[name] = c
		secrets[name+" sha256 hex"] = hex.EncodeToString(sum[:])
		secrets[name+" sha256 base64"] = base64.StdEncoding.EncodeToString(sum[:])
		secrets[name+" sha256 base64url"] = base64.RawURLEncoding.EncodeToString(sum[:])
	}
	for name, v := range secrets {
		if strings.Contains(out, v) {
			t.Errorf("captured logs contain the %s", name)
		}
	}
}
