package repository

import "errors"

// STUB (test author): placeholder so the round-2 tests compile. The implementer
// replaces this with the real sentinel and wraps RotateRefresh/RevokeSession
// ev(), outbox-enqueue and refresh_hash-conflict errors with it, then deletes this file.
var ErrRotateFault = errors.New("stub: rotate fault not implemented")
