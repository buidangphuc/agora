package config_test

import (
	"testing"

	"github.com/buidangphuc/team-promotion/internal/config"
)

func TestMinAdBidDefaultsToFiveHundred(t *testing.T) {
	localEnv(t)
	t.Setenv("MIN_AD_BID", "")
	unset(t, "MIN_AD_BID")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := asInt64(t, settingByEnvKey(t, s, "MIN_AD_BID")); got != 500 {
		t.Fatalf("MIN_AD_BID default = %d, want 500", got)
	}
}

func TestMinAdBidMustBeAtLeastOne(t *testing.T) {
	for _, v := range []string{"0", "-5", "abc"} {
		t.Run("MIN_AD_BID="+v, func(t *testing.T) {
			localEnv(t)
			t.Setenv("MIN_AD_BID", v)
			_, err := config.Load()
			mustErrNaming(t, err, "MIN_AD_BID")
		})
	}
	localEnv(t)
	t.Setenv("MIN_AD_BID", "1")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("MIN_AD_BID=1 must load: %v", err)
	}
	if got := asInt64(t, settingByEnvKey(t, s, "MIN_AD_BID")); got != 1 {
		t.Fatalf("MIN_AD_BID = %d, want 1", got)
	}
}
