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
	"github.com/buidangphuc/team-identity/internal/token"
)

func devSigner(t *testing.T, kid string) *token.Signer {
	t.Helper()
	pemBytes, err := os.ReadFile("testdata/dev_signing_key.pem") // labelled: the committed, public dev key
	if err != nil {
		t.Fatal(err)
	}
	s, err := token.NewSigner(string(pemBytes), kid)
	if err != nil {
		t.Fatalf("NewSigner(dev key): %v", err)
	}
	return s
}

func freshSigner(t *testing.T, kid string) *token.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	s, err := token.NewSigner(string(p), kid)
	if err != nil {
		t.Fatal(err)
	}
	return s
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
		signer  *token.Signer
		wantErr string // substring; "" = accepted
	}{
		{"strict + dev key (dev kid)", "production", devSigner(t, "dev-2026"), "JWT_KID"},
		{"strict + dev key under another kid", "staging", devSigner(t, "renamed-kid"), "development signing key"},
		{"strict + fresh key with dev kid", "staging", freshSigner(t, "dev-2026"), "JWT_KID"},
		{"strict + fresh key", "production", freshSigner(t, "prod-2026-10"), ""},
		{"local + dev key", "local", devSigner(t, "dev-2026"), ""},
		{"empty env + dev key", "", devSigner(t, "dev-2026"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := settingsFor(tc.env).RequireNonDevSigningKey(tc.signer.KID(), tc.signer.PublicKey())
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
	s := devSigner(t, "x")
	if err := settingsFor("production").RequireNonDevSigningKey("x", s.PublicKey()); err == nil {
		t.Fatal("devKeyFingerprint constant does not match the committed dev key")
	}
}
