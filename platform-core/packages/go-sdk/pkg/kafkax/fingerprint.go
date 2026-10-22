package kafkax

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// Fingerprinter turns a record key into a short keyed fingerprint that is safe
// to log: 12 lowercase hex characters of HMAC-SHA256 under a secret.
type Fingerprinter struct{ secret []byte }

// NewFingerprinter fails on an empty secret.
func NewFingerprinter(secret string) (*Fingerprinter, error) {
	if secret == "" {
		return nil, errors.New("kafkax: fingerprint secret is empty")
	}
	return &Fingerprinter{secret: []byte(secret)}, nil
}

// Of returns the fingerprint of key.
func (f *Fingerprinter) Of(key []byte) string {
	m := hmac.New(sha256.New, f.secret)
	m.Write(key)
	return hex.EncodeToString(m.Sum(nil))[:12]
}
