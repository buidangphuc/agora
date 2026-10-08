package config

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"
)

// DevJWTKID is the key id of the development signing key committed to the repo.
const DevJWTKID = "dev-2026"

// devKeyFingerprint is the hex SHA-256 of the DER (PKIX) public key of the
// development signing key committed to the repo (local compose / kind vault
// config). Only the public-key hash lives here, so no extra copy of the private
// key is committed. Matching by fingerprint (not only kid) also catches the dev
// key re-labelled under another kid.
const devKeyFingerprint = "dfc8b00fc06a698bf1236d331e87ed80143197ebfcaff4f88213d9f8399f18f3"

// PublicKeyFingerprint returns the hex SHA-256 of the key's PKIX DER encoding.
func PublicKeyFingerprint(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// RequireNonDevSigningKey is the boot guard for the signing key: in a strict ENV
// it refuses JWT_KID=dev-2026 and the committed development key (by public-key
// fingerprint). Other environments always pass. The key material is never
// logged or included in the error.
func (s *Settings) RequireNonDevSigningKey(kid string, pub *rsa.PublicKey) error {
	if !s.RequiresHardenedSecrets() {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(kid), DevJWTKID) {
		return fmt.Errorf("refusing to start: JWT_KID=%q is the committed development key id in a strict ENV (ENV=%q); provision a real signing key and kid (Vault svc/shared/jwt)", kid, s.Runtime.Env)
	}
	fp, err := PublicKeyFingerprint(pub)
	if err != nil {
		return err
	}
	if fp == devKeyFingerprint {
		return fmt.Errorf("refusing to start: JWT_PRIVATE_KEY is the development signing key committed to the repository, which is not allowed in a strict ENV (ENV=%q); provision a real signing key (Vault svc/shared/jwt)", s.Runtime.Env)
	}
	return nil
}
