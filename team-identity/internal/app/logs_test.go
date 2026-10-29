package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
)

const noteMarker = "NOTE-MARKER-7f3a9c"

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func logSettings(t *testing.T, url string) config.Settings {
	t.Helper()
	s := newSettings(t, url, deadBroker)
	s.LogLevel = "info"
	return s
}

func registerBuyer(t *testing.T, r *running, username string) (userID string, res *identityv1.AuthResult) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	reg, err := identityv1.NewAuthServiceClient(r.conn).Register(ctx, &identityv1.RegisterRequest{Username: username, Password: testPassword, Role: "buyer"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return reg.GetResult().GetPrincipal().GetId(), reg.GetResult()
}

func TestCallLogsCarryRequestAndPrincipalID(t *testing.T) {
	// scenario: Call logs carry request id and principal id
	url := migratedURL(t)
	logs := captureLogs(t)
	r := start(t, logSettings(t, url))
	waitServing(t, r)
	userID, _ := registerBuyer(t, r, "erin")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = asUser(metadataRequestID(ctx, "abc"), userID)
	if _, err := identityv1.NewSessionServiceClient(r.conn).ListSessions(ctx, &identityv1.ListSessionsRequest{}); err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	testkit.Eventually(t, 10*time.Second, func() bool {
		for _, l := range logs.Lines() {
			if l["request_id"] == "abc" && l["principal_id"] == userID && l["service"] == "team-identity" {
				return true
			}
		}
		return false
	})
}

func TestDeniedCallLogsCarryNoRequestBody(t *testing.T) {
	// scenario: A refused call logs the principal and no request body
	url := migratedURL(t)
	logs := captureLogs(t)
	spans := captureSpans(t)
	r := start(t, logSettings(t, url))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := identityv1.NewAccountAdminServiceClient(r.conn).SetUserStatus(asUser(ctx, "seller-9", "listing.read", "listing.write"), &identityv1.SetUserStatusRequest{
		UserId: "someone", Status: identityv1.UserStatus_USER_STATUS_BANNED, ReasonCode: "abuse", Note: noteMarker,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetUserStatus by a seller = %v, want PermissionDenied", err)
	}

	testkit.Eventually(t, 10*time.Second, func() bool {
		for _, l := range logs.Lines() {
			if l["principal_id"] == "seller-9" && strings.Contains(toJSON(l), "PermissionDenied") {
				return true
			}
		}
		return false
	})
	if strings.Contains(logs.String(), noteMarker) {
		t.Error("a captured log line carries the request note")
	}
	if strings.Contains(spanText(spans), noteMarker) {
		t.Error("a span attribute carries the request note")
	}
}

func TestSecretsNeverReachLogsOrSpans(t *testing.T) {
	// scenario: Secrets never reach logs or spans
	url := migratedURL(t)
	logs := captureLogs(t)
	spans := captureSpans(t)
	audit := &fakeAudit{}
	s := logSettings(t, url)
	s.LogLevel = "debug"
	s.Upstream.AuditAddr = startAudit(t, audit)
	s.PasswordReset.ExposeToken = true
	r := start(t, s)
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	auth := identityv1.NewAuthServiceClient(r.conn)
	secrets := map[string]string{"password": testPassword}

	userID, reg := registerBuyer(t, r, "frank")
	secrets["register access token"] = reg.GetToken()
	r1 := reg.GetRefreshToken()
	secrets["refresh token 1"] = r1

	login, err := auth.Login(ctx, &identityv1.LoginRequest{Username: "frank", Password: testPassword})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	secrets["login access token"] = login.GetResult().GetToken()
	secrets["login refresh token"] = login.GetResult().GetRefreshToken()

	rot1, err := auth.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: login.GetResult().GetRefreshToken()})
	if err != nil {
		t.Fatalf("RefreshSession rotation: %v", err)
	}
	r2 := rot1.GetResult().GetRefreshToken()
	secrets["rotated access token"] = rot1.GetResult().GetToken()
	secrets["refresh token 2"] = r2
	if _, err := auth.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: login.GetResult().GetRefreshToken()}); err != nil {
		t.Logf("grace replay: %v", err)
	}
	rot2, err := auth.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: r2})
	if err != nil {
		t.Fatalf("second RefreshSession rotation: %v", err)
	}
	secrets["refresh token 3"] = rot2.GetResult().GetRefreshToken()
	if _, err := auth.RefreshSession(ctx, &identityv1.RefreshSessionRequest{RefreshToken: login.GetResult().GetRefreshToken()}); err == nil {
		t.Error("presenting a refresh token two rotations old succeeded; reuse must be refused")
	}

	reset, err := auth.RequestPasswordReset(ctx, &identityv1.RequestPasswordResetRequest{Username: "frank"})
	if err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if reset.GetResetToken() == "" {
		t.Fatal("no reset token returned with PASSWORD_RESET_EXPOSE_TOKEN=true")
	}
	secrets["reset token"] = reset.GetResetToken()
	secrets["reset token hash"] = sha256Hex(reset.GetResetToken())

	for _, name := range []string{"refresh token 1", "refresh token 2", "refresh token 3", "login refresh token"} {
		secrets[name+" hash"] = sha256Hex(secrets[name])
	}

	if _, err := identityv1.NewAccountAdminServiceClient(r.conn).SetUserStatus(asUser(ctx, "admin-1", "admin"), &identityv1.SetUserStatusRequest{
		UserId: userID, Status: identityv1.UserStatus_USER_STATUS_SUSPENDED, ReasonCode: "abuse", Note: noteMarker,
	}); err != nil {
		t.Fatalf("SetUserStatus: %v", err)
	}
	secrets["note marker"] = noteMarker

	r.stop(t)
	logText, spanDump := logs.String(), spanText(spans)
	if len(logs.Lines()) == 0 {
		t.Fatal("no log lines captured; the capture is not working")
	}
	if len(spans.GetSpans()) == 0 {
		t.Fatal("no spans captured; the capture is not working")
	}
	for name, secret := range secrets {
		if secret == "" {
			t.Errorf("secret %q is empty; the scenario did not produce it", name)
			continue
		}
		if strings.Contains(logText, secret) {
			t.Errorf("a captured log line carries the %s", name)
		}
		if strings.Contains(spanDump, secret) {
			t.Errorf("a span attribute carries the %s", name)
		}
	}
}
