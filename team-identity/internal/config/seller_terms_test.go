package config_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/buidangphuc/team-identity/internal/config"
)

const sellerTermsKey = "SELLER_TERMS_VERSIONS"

// Scenario: Seller onboarding: the terms version must match exactly
func TestSellerTermsVersionsKeepOrderAndBytes(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want []string
	}{
		{"V1", []string{"V1"}},
		{"V2,V1", []string{"V2", "V1"}},
		{"v1,V1", []string{"v1", "V1"}},
	} {
		localEnv(t)
		t.Setenv(sellerTermsKey, tc.raw)
		got := valueByEnv(t, mustLoad(t), sellerTermsKey)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s=%q parsed to %#v, want %#v", sellerTermsKey, tc.raw, got, tc.want)
		}
	}
}

// Scenario: Seller onboarding: an unknown terms version is refused
func TestSellerTermsVersionsEmptyFailsFast(t *testing.T) {
	for _, env := range []string{"local", "test", "staging", "prod"} {
		for _, raw := range []string{"", ","} {
			strictEnv(t, env)
			t.Setenv(sellerTermsKey, raw)
			_, err := config.Load()
			mustContain(t, err, sellerTermsKey)
		}
	}
}

// Scenario: Seller onboarding: an unknown terms version is refused
func TestSellerTermsVersionsUnsetFailsFast(t *testing.T) {
	for _, env := range []string{"local", "test", "prod"} {
		strictEnv(t, env)
		t.Setenv(sellerTermsKey, "")
		unsetEnv(t, sellerTermsKey)
		_, err := config.Load()
		mustContain(t, err, sellerTermsKey)
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}
