// Package mockdata selects, per adapter, between a real implementation and a
// fixture implementation according to the MOCK_DATA setting.
package mockdata

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

// Set is the validated set of adapters that serve mock data.
type Set struct {
	active []string
	once   sync.Once
}

// New parses cfg against the adapter names the service knows. Names are
// trimmed, lower-cased, de-duplicated; an unknown name is an error.
func New(cfg config.MockData, known ...string) (*Set, error) {
	s := &Set{}
	for raw := range strings.SplitSeq(cfg.Adapters, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" || slices.Contains(s.active, name) {
			continue
		}
		if !slices.Contains(known, name) {
			return nil, fmt.Errorf("MOCK_DATA names unknown adapter %q; known adapters: %s", name, strings.Join(known, ", "))
		}
		s.active = append(s.active, name)
	}
	slices.Sort(s.active)
	return s, nil
}

// Active reports whether the named adapter serves mock data.
func (s *Set) Active(name string) bool { return slices.Contains(s.active, name) }

// Adapters returns the active adapter names, sorted.
func (s *Set) Adapters() []string { return slices.Clone(s.active) }

// LogActive logs the active adapters once per Set; it logs nothing when none are active.
func (s *Set) LogActive(l *slog.Logger) {
	if len(s.active) == 0 {
		return
	}
	s.once.Do(func() {
		l.Warn("MOCK_DATA active: serving fixture data", "adapters", strings.Join(s.active, ","))
	})
}

// Select returns mock when the named adapter is active, otherwise real.
func Select[T any](s *Set, name string, real, mock T) T {
	if s.Active(name) {
		return mock
	}
	return real
}
