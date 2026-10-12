// Package store owns Postgres access. Here it only embeds the migrations:
// hello keeps no data, but a real service puts its queries next to them.
package store

import (
	"embed"
	"io/fs"
)

//go:embed migrations/*.sql
var files embed.FS

// Migrations returns the migration files at the root of the returned FS, the
// shape migrate.Up expects.
func Migrations() (fs.FS, error) { return fs.Sub(files, "migrations") }
