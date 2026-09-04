package repository

import (
	"crypto/rand"
	"encoding/hex"
)

// newID returns a random 128-bit identifier as a 32-char hex string. It avoids a
// uuid dependency (crypto/rand is stdlib) while giving collision-free surrogate
// keys for vouchers, reservations, and campaigns. On the (astronomically
// unlikely) rand failure it panics — an unusable CSPRNG is a fatal boot problem,
// not something a caller can recover from.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("promotion: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
