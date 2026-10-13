// Package migrations embeds the golang-migrate files so tests (and an optional migrate
// subcommand) can pass Files to the SDK migrate.Up. The deploy migrate job reads the
// directory itself; the service does not apply migrations at boot.
package migrations

import "embed"

// Files holds every NNNN_name.up.sql / .down.sql at its root.
//
//go:embed *.sql
var Files embed.FS
