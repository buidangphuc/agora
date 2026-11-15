package config_test

import (
	"reflect"
	"testing"

	"github.com/buidangphuc/team-identity/internal/config"
)

const (
	legalTermsKey    = "LEGAL_TERMS_VERSIONS"
	privacyPolicyKey = "PRIVACY_POLICY_VERSIONS"
)

// Scenario: First entry is the current version
func TestLegalVersionsKeepOrderAndFirstEntryIsCurrent(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want []string
	}{
		{"2026-11", []string{"2026-11"}},
		{"2026-11,2026-05", []string{"2026-11", "2026-05"}},
		{"2026-05,2026-11", []string{"2026-05", "2026-11"}},
	} {
		for _, key := range []string{legalTermsKey, privacyPolicyKey} {
			localEnv(t)
			t.Setenv(key, tc.raw)
			got := valueByEnv(t, mustLoad(t), key)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%s=%q parsed to %#v, want %#v", key, tc.raw, got, tc.want)
			}
		}
	}
}

// Scenario: Boot fails without a configured version
func TestLegalVersionsEmptyFailsFast(t *testing.T) {
	for _, env := range []string{"local", "test", "staging", "prod"} {
		for _, key := range []string{legalTermsKey, privacyPolicyKey} {
			for _, raw := range []string{"", ","} {
				strictEnv(t, env)
				t.Setenv(key, raw)
				_, err := config.Load()
				mustContain(t, err, key)
			}
		}
	}
}

// Scenario: Boot fails without a configured version
func TestLegalVersionsUnsetFailsFast(t *testing.T) {
	for _, env := range []string{"local", "test", "prod"} {
		for _, key := range []string{legalTermsKey, privacyPolicyKey} {
			strictEnv(t, env)
			unsetEnv(t, key)
			_, err := config.Load()
			mustContain(t, err, key)
		}
	}
}
