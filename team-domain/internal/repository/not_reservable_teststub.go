package repository

import "errors"

// TEST-STUB (task 4 red): compile-only declaration of the sentinel the tests
// reference. The implementer MOVES this into listing.go next to the other
// sentinels, wires it into ReserveStockIdempotent (in-memory and Postgres) and
// DELETES this file.
var ErrListingNotReservable = errors.New("listing not reservable")
