package config_test

import (
	"regexp"
	"testing"
	"time"
)

func TestUpstreamRequiresPaymentAddr(t *testing.T) {
	// Scenario: Blank payment address is rejected at startup
	for _, env := range []string{"local", "test", "dev"} {
		for name, val := range map[string]string{"empty": "", "blank": "   "} {
			t.Run(env+"/"+name, func(t *testing.T) {
				localEnv(t)
				t.Setenv("ENV", env)
				t.Setenv("UPSTREAM_PAYMENT_ADDR", val)
				_, err := loadSettings(t)
				mustErrNaming(t, err, "UPSTREAM_PAYMENT_ADDR")
			})
		}
	}
	t.Run("unset", func(t *testing.T) {
		localEnv(t)
		unset(t, "UPSTREAM_PAYMENT_ADDR")
		_, err := loadSettings(t)
		mustErrNaming(t, err, "UPSTREAM_PAYMENT_ADDR")
	})
	t.Run("configured", func(t *testing.T) {
		localEnv(t)
		s, err := loadSettings(t)
		if err != nil {
			t.Fatalf("fully configured local ENV must load: %v", err)
		}
		if got, _ := settingByEnvKey(t, s, "UPSTREAM_PAYMENT_ADDR").(string); got != "payment:50056" {
			t.Errorf("UPSTREAM_PAYMENT_ADDR = %q, want payment:50056", got)
		}
	})
}

func TestUpstreamRejectsNonPositivePaymentTimeout(t *testing.T) {
	// Scenario: Non-positive timeout is invalid
	for _, val := range []string{"0s", "-1s"} {
		t.Run(val, func(t *testing.T) {
			localEnv(t)
			t.Setenv("UPSTREAM_PAYMENT_TIMEOUT", val)
			_, err := loadSettings(t)
			mustErrNaming(t, err, "UPSTREAM_PAYMENT_TIMEOUT")
		})
	}
	localEnv(t)
	t.Setenv("UPSTREAM_PAYMENT_TIMEOUT", "1ms")
	if _, err := loadSettings(t); err != nil {
		t.Fatalf("smallest positive timeout must load: %v", err)
	}
}

func TestSubscriptionRenewalRetryWindowMustBePositive(t *testing.T) {
	// Scenario: Non-positive timeout is invalid
	for _, val := range []string{"0s", "-1m"} {
		t.Run(val, func(t *testing.T) {
			localEnv(t)
			t.Setenv("SUBSCRIPTION_RENEWAL_RETRY_WINDOW", val)
			_, err := loadSettings(t)
			mustErrNaming(t, err, "SUBSCRIPTION_RENEWAL_RETRY_WINDOW")
		})
	}
}

func TestPaymentSettingsDefaults(t *testing.T) {
	// Scenario: Blank payment address is rejected at startup
	localEnv(t)
	s, err := loadSettings(t)
	if err != nil {
		t.Fatal(err)
	}
	if got := asDuration(t, settingByEnvKey(t, s, "UPSTREAM_PAYMENT_TIMEOUT")); got != 2*time.Second {
		t.Errorf("UPSTREAM_PAYMENT_TIMEOUT = %v, want 2s", got)
	}
	if got := asDuration(t, settingByEnvKey(t, s, "SUBSCRIPTION_RENEWAL_RETRY_WINDOW")); got != time.Hour {
		t.Errorf("SUBSCRIPTION_RENEWAL_RETRY_WINDOW = %v, want 1h", got)
	}
}

func TestPaymentSettingsDocumented(t *testing.T) {
	// Scenario: Blank payment address is rejected at startup
	env := repoFile(t, "team-promotion", ".env.example")
	readme := repoFile(t, "team-promotion", "README.md")
	for _, k := range []string{"UPSTREAM_PAYMENT_ADDR", "UPSTREAM_PAYMENT_TIMEOUT", "SUBSCRIPTION_RENEWAL_RETRY_WINDOW"} {
		if !regexp.MustCompile(`(?m)^` + k + `=`).MatchString(env) {
			t.Errorf(".env.example lacks %s", k)
		}
		if !regexp.MustCompile("(?m)^\\|\\s*`" + k + "`").MatchString(readme) {
			t.Errorf("README settings table has no row for %s", k)
		}
	}
}
