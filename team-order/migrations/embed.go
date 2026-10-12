// Package migrations embeds the SQL migrations so tests (and an optional migrate
// subcommand) can pass Files to the go-sdk migrate package. The deploy migrate
// job (compose: team-order-migrate) applies them; the service does not at boot.
package migrations

import "embed"

// Files holds every NNNN_name.up.sql / .down.sql at the root of this package.
//
//go:embed *.sql
var Files embed.FS
