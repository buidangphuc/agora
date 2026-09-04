// Package bootstrap owns the resource lifecycle: it opens the infrastructure the
// service needs (its Postgres), runs enable-gated optional addons, installs a
// health service with a DB dependency check, and tears everything down in
// reverse order. It mirrors team-ai's app/bootstrap (ApplicationResources +
// BootstrapAddon + open/close_application_resources).
package bootstrap

import (
	"context"

	"github.com/buidangphuc/team-domain/internal/config"
)

// Addon is an optional capability with an enable-gated lifecycle — the Go
// analogue of team-ai's BootstrapAddon (is_enabled / open / close). Core infra
// (Postgres) is opened directly by the lifecycle; addons are the extension point
// for optional capabilities (Redis cache, quota, ...) added later.
type Addon interface {
	Name() string
	Enabled(s *config.Settings) bool
	Open(ctx context.Context, s *config.Settings, res *Resources) error
	Close(ctx context.Context, res *Resources) error
}

// defaultAddons is the ordered set of optional addons. Empty today; new
// capabilities register here and are opened in order / closed in reverse — the
// same shape as team-ai's default_resource_addons().
func defaultAddons() []Addon { return nil }
