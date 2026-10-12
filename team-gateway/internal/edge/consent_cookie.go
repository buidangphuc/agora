package edge

import (
	"net/http"
	"strconv"
	"strings"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
)

const (
	consentCookie       = "agora_consent"
	maxConsentCookieLen = 256
	maxGaClientIDLen    = 64
)

// parseConsentCookie reads the agora_consent cookie
// (v=1&a=0|1&m=0|1&p=0|1&ts=<epoch>). Anything missing or malformed degrades
// to all-denied, version 0. Only the cookie is trusted (never body/headers).
func parseConsentCookie(r *http.Request) (analytics, marketing, personalization bool, version int32) {
	c, err := r.Cookie(consentCookie)
	if err != nil || len(c.Value) > maxConsentCookieLen {
		return false, false, false, 0
	}
	seen := map[string]string{}
	for _, part := range strings.Split(c.Value, "&") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return false, false, false, 0
		}
		if _, dup := seen[k]; dup {
			return false, false, false, 0
		}
		seen[k] = v
	}
	if seen["v"] != "1" {
		return false, false, false, 0
	}
	if ts, ok := seen["ts"]; !ok {
		return false, false, false, 0
	} else if _, err := strconv.ParseUint(ts, 10, 64); err != nil {
		return false, false, false, 0
	}
	var flags [3]bool
	for i, k := range []string{"a", "m", "p"} {
		switch seen[k] {
		case "1":
			flags[i] = true
		case "0":
		default:
			return false, false, false, 0
		}
	}
	return flags[0], flags[1], flags[2], 1
}

func newConsentState(a, m, p bool, v int32) *analyticsv1.ConsentState {
	return &analyticsv1.ConsentState{Analytics: a, Marketing: m, Personalization: p, Version: v}
}
