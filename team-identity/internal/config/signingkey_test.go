package config_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"strings"
	"testing"

	"github.com/buidangphuc/team-identity/internal/config"
)

// keyed is a kid plus the public key the guard inspects.
type keyed struct {
	kid string
	pub *rsa.PublicKey
}

// devKey loads the public half of the development signing key. Only the public
// key is kept in testdata; the guard matches by public-key fingerprint.
func devKey(t *testing.T, kid string) keyed {
	t.Helper()
	pemBytes, err := os.ReadFile("testdata/dev_signing_key.pub.pem")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(pemBytes[strings.Index(string(pemBytes), "-----BEGIN"):])
	if block == nil {
		t.Fatal("no PEM block in testdata/dev_signing_key.pub.pem")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pub, ok := k.(*rsa.PublicKey)
	if !ok {
		t.Fatal("dev key is not RSA")
	}
	return keyed{kid: kid, pub: pub}
}

func freshKey(t *testing.T, kid string) keyed {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return keyed{kid: kid, pub: &key.PublicKey}
}

func settingsFor(env string) *config.Settings {
	s := &config.Settings{}
	s.Runtime.Env = env
	return s
}

func TestRequireNonDevSigningKey(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		key     keyed
		wantErr string // substring; "" = accepted
	}{
		{"strict + dev key (dev kid)", "production", devKey(t, "dev-2026"), "JWT_KID"},
		{"strict + dev key under another kid", "staging", devKey(t, "renamed-kid"), "development signing key"},
		{"strict + fresh key with dev kid", "staging", freshKey(t, "dev-2026"), "JWT_KID"},
		{"strict + fresh key", "production", freshKey(t, "prod-2026-10"), ""},
		{"local + dev key", "local", devKey(t, "dev-2026"), ""},
		{"empty env + dev key", "", devKey(t, "dev-2026"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := settingsFor(tc.env).RequireNonDevSigningKey(tc.key.kid, tc.key.pub)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("want accepted, got %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
			if err != nil && strings.Contains(err.Error(), "PRIVATE KEY") {
				t.Fatalf("error must not contain key material: %v", err)
			}
		})
	}
}

func TestDevKeyFingerprintMatchesCommittedTestdata(t *testing.T) {
	k := devKey(t, "x")
	if err := settingsFor("production").RequireNonDevSigningKey(k.kid, k.pub); err == nil {
		t.Fatal("devKeyFingerprint constant does not match the committed dev key")
	}
}
