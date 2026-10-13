// Package migrations embeds the SQL migrations so tests (and an optional
// `migrate up` subcommand) can pass them to migrate.Up. The deploy migrate job
// applies the same files; the service never applies them at boot.
package migrations

import "embed"

// Files holds the NNN_name.up/down.sql files at its root, the shape migrate.Up
// expects.
//
//go:embed *.sql
var Files embed.FS
