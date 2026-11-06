package consumer_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/consumer"
)

func TestLogsContainNoRawIdentifiers(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	pool := newPool(t)
	clock := t0
	cfg := testConfig(func() time.Time { return clock })
	cfg.Logger = logger
	e := &env{
		pool: pool, cfg: cfg, clock: &clock,
		identity: consumer.NewIdentityHandler(pool, cfg),
		referral: consumer.NewReferralHandler(pool, cfg),
		sweeper:  consumer.NewSweeper(pool, cfg),
	}

	waiting, timedOut, plain := "user-wait-7f3a91", "user-slow-c04d22", "user-plain-9c2d5e"
	secretCode := "REFSECRETCODE77"
	e.mustRegister(t, waiting, secretCode)
	e.mustVerdict(t, typeAttributed, waiting)
	e.mustRegister(t, timedOut, secretCode)
	e.mustRegister(t, plain, "")
	clock = t0.Add(verdictTimeout + time.Minute)
	if n, err := e.sweeper.Sweep(context.Background()); err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v", n, err)
	}
	_ = e.registered(t, "bad-1", "", "")
	_ = e.identity(context.Background(), registeredRecord(t, "bad-2", "", secretCode))

	raw := []string{waiting, timedOut, plain, secretCode}
	for _, u := range []string{waiting, timedOut, plain} {
		for _, v := range newUserVouchers(t, pool, u) {
			raw = append(raw, v.Code)
		}
	}
	logs := buf.String()
	for _, r := range raw {
		if strings.Contains(logs, r) {
			t.Errorf("logs contain the raw value %q", r)
		}
	}
	if !strings.Contains(logs, "principal_fp") && !strings.Contains(logs, "key_fp") {
		t.Errorf("logs contain neither principal_fp nor key_fp:\n%s", logs)
	}
}
