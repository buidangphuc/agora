package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-referral/internal/config"
)

func TestReferralSettingsDefaults(t *testing.T) {
	baseEnv(t, "local")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := s.Referral
	if r.RewardXU != 10000 || r.DailyCap != 5 || r.LifetimeCap != 50 {
		t.Errorf("reward/caps = %d/%d/%d, want 10000/5/50", r.RewardXU, r.DailyCap, r.LifetimeCap)
	}
	if r.RedeemMaxAccountAge != 168*time.Hour || r.SweepInterval != time.Minute {
		t.Errorf("age/interval = %v/%v, want 168h/1m", r.RedeemMaxAccountAge, r.SweepInterval)
	}
	if r.CapTimezone != "Asia/Ho_Chi_Minh" {
		t.Errorf("CapTimezone = %q", r.CapTimezone)
	}
	want := map[string]string{
		r.IdentityTopic:    "identity.events",
		r.OrderTopic:       "order.events",
		r.EventsTopic:      "referral.events",
		r.IdentityDLQTopic: "referral.identity.dlq",
		r.OrderDLQTopic:    "referral.order.dlq",
	}
	for got, w := range want {
		if got != w {
			t.Errorf("topic default = %q, want %q", got, w)
		}
	}
	if r.IdentityGroup == "" || r.OrderGroup == "" || r.IdentityGroup == r.OrderGroup {
		t.Errorf("consumer groups must be set and distinct: %q %q", r.IdentityGroup, r.OrderGroup)
	}
	if s.Kafka.Brokers == "" || s.Outbox.BatchSize <= 0 {
		t.Errorf("Kafka/Outbox groups not loaded: %+v %+v", s.Kafka, s.Outbox)
	}
}

func TestReferralSettingsRejectInvalid(t *testing.T) {
	cases := []struct{ key, val string }{
		{"REFERRAL_DAILY_CAP", "0"},
		{"REFERRAL_DAILY_CAP", "-2"},
		{"REFERRAL_DAILY_CAP", "abc"},
		{"REFERRAL_LIFETIME_CAP", "0"},
		{"REFERRAL_REWARD_XU", "0"},
		{"REFERRAL_REWARD_XU", "-1"},
		{"REFERRAL_REDEEM_MAX_ACCOUNT_AGE", "0s"},
		{"REFERRAL_REDEEM_MAX_ACCOUNT_AGE", "soon"},
		{"REFERRAL_RELEASE_SWEEP_INTERVAL", "-1s"},
		{"REFERRAL_RELEASE_SWEEP_INTERVAL", "0"},
		{"REFERRAL_CAP_TIMEZONE", "Mars/Base"},
	}
	for _, c := range cases {
		t.Run(c.key+"="+c.val, func(t *testing.T) {
			baseEnv(t, "local")
			t.Setenv(c.key, c.val)
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), c.key) {
				t.Fatalf("err = %v, want an error naming %s", err, c.key)
			}
		})
	}
}

func TestReferralSettingsOverride(t *testing.T) {
	baseEnv(t, "local")
	t.Setenv("REFERRAL_DAILY_CAP", "2")
	t.Setenv("REFERRAL_LIFETIME_CAP", "3")
	t.Setenv("REFERRAL_REWARD_XU", "500")
	t.Setenv("REFERRAL_RELEASE_SWEEP_INTERVAL", "1s")
	t.Setenv("REFERRAL_CAP_TIMEZONE", "UTC")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := s.Referral
	if r.DailyCap != 2 || r.LifetimeCap != 3 || r.RewardXU != 500 || r.SweepInterval != time.Second || r.CapTimezone != "UTC" {
		t.Fatalf("overrides not applied: %+v", r)
	}
}
