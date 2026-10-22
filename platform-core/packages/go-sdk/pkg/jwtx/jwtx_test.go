package jwtx_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/jwtx"
)

func TestClaimsJSONContract(t *testing.T) {
	in := jwtx.Claims{
		Name: "Ann", Type: "user", Scopes: []string{"a", "b"}, SessionID: "s1",
		Issuer: "identity", Subject: "u1", Audience: jwtx.Audience{"gw"},
		ExpiresAt: 200, NotBefore: 90, IssuedAt: 100, ID: "j1",
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"aud", "exp", "iat", "iss", "jti", "name", "nbf", "scopes", "sid", "sub", "typ"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	var out jwtx.Claims
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip: got %+v, want %+v", out, in)
	}

	// Without a session the sid key is omitted; scopes/name/typ stay.
	raw, _ = json.Marshal(jwtx.Claims{Name: "svc", Type: "service", Scopes: []string{}})
	m = map[string]any{}
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["sid"]; ok {
		t.Fatalf("sid should be omitted when empty: %s", raw)
	}
	for _, k := range []string{"name", "typ", "scopes"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("%s missing: %s", k, raw)
		}
	}
}

func TestAudienceAcceptsStringOrArray(t *testing.T) {
	var c jwtx.Claims
	if err := json.Unmarshal([]byte(`{"aud":"gw"}`), &c); err != nil || len(c.Audience) != 1 || c.Audience[0] != "gw" {
		t.Fatalf("string aud: %+v, %v", c.Audience, err)
	}
	if err := json.Unmarshal([]byte(`{"aud":["a","b"]}`), &c); err != nil || len(c.Audience) != 2 {
		t.Fatalf("array aud: %+v, %v", c.Audience, err)
	}
}

func TestJWKSJSONFieldNames(t *testing.T) {
	in := jwtx.JWKS{Keys: []jwtx.JWK{{Kty: "RSA", Use: "sig", Alg: "RS256", Kid: "k1", N: "n", E: "AQAB"}}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"keys":[{"kty":"RSA","use":"sig","alg":"RS256","kid":"k1","n":"n","e":"AQAB"}]}`
	if string(raw) != want {
		t.Fatalf("got %s, want %s", raw, want)
	}
}
