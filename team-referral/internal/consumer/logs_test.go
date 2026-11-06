package consumer_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-referral/internal/consumer"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/store/fake"
)

func TestLogsContainNoRawIdentifiers(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	repo := fake.NewInMemoryReferralRepo()
	cfg := consumer.Config{RewardXU: 10000, LifetimeCap: 50, Now: func() time.Time { return t0 }, Logger: logger}
	e := &env{repo: repo, identity: consumer.NewIdentityHandler(repo, cfg), order: consumer.NewOrderHandler(repo, cfg), cfg: cfg}

	inviter, invitee, order := "user-alice-7f3a91", "user-bob-c04d22", "order-o1-9c2d5e"
	code, err := repo.CreateCode(context.Background(), inviter, "REFLOGCODE1")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.registered(t, "ev-1", invitee, code); err != nil {
		t.Fatal(err)
	}
	if err := e.completed(t, "oc-1", order, invitee, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	sweeper := referral.NewSweeper(repo,
		referral.WithConfig(referral.Config{RewardXU: 10000, DailyCap: 5, LifetimeCap: 50, RedeemMaxAccountAge: 168 * time.Hour, SweepInterval: time.Minute, CapLocation: time.UTC}),
		referral.WithClock(func() time.Time { return t0.Add(2 * time.Hour) }),
		referral.WithLogger(logger))
	if n, err := sweeper.Sweep(context.Background()); err != nil || n != 1 {
		t.Fatalf("sweep = %d, %v", n, err)
	}

	logs := buf.String()
	for _, raw := range []string{inviter, invitee, order, code} {
		if strings.Contains(logs, raw) {
			t.Errorf("logs contain the raw identifier %q", raw)
		}
	}
	if !strings.Contains(logs, "principal_fp") && !strings.Contains(logs, "key_fp") {
		t.Errorf("logs contain neither principal_fp nor key_fp:\n%s", logs)
	}
}
