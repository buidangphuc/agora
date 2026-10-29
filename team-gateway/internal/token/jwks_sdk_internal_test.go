package token

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/jwtx"
)

// Scenario: JWKS keys decode through the SDK types.
func TestJWKSDocumentDecodesWithSDKTypes_ParseRSAJWK(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	k := jwtx.JWK{
		Kty: "RSA", Use: "sig", Alg: "RS256", Kid: "kid-1",
		N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
	pub, err := parseRSAJWK(k)
	if err != nil {
		t.Fatalf("parseRSAJWK(jwtx.JWK): %v", err)
	}
	if pub.N.Cmp(key.N) != 0 || pub.E != key.E {
		t.Error("decoded public key differs from the source key")
	}
}
