package pgtest

import (
	"sync"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

// The SDK has a shared Postgres (testkit.SharedPostgres) but no shared Redpanda, so this
// file keeps one for the settlement tests: starting a broker per test costs seconds each.

// keepTB lets a testkit fixture outlive the test that first asks for it: its cleanups
// run in Run, after the whole test binary finished.
type keepTB struct {
	*testing.T
	owner *shared
}

func (k keepTB) Cleanup(f func()) { k.owner.add(f) }

type shared struct {
	mu       sync.Mutex
	cleanups []func()
	rpOnce   sync.Once
	rpAddr   string
}

func (s *shared) add(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanups = append(s.cleanups, f)
}

var fixtures shared

// Run is for TestMain: it runs the tests, then stops the shared Redpanda container.
func Run(m *testing.M) int {
	code := m.Run()
	fixtures.mu.Lock()
	defer fixtures.mu.Unlock()
	for i := len(fixtures.cleanups) - 1; i >= 0; i-- {
		fixtures.cleanups[i]()
	}
	return code
}

// Brokers returns the seed brokers of one Redpanda container shared by the test binary.
func Brokers(t *testing.T) string {
	t.Helper()
	fixtures.rpOnce.Do(func() { fixtures.rpAddr = testkit.Redpanda(keepTB{T: t, owner: &fixtures}) })
	if fixtures.rpAddr == "" {
		t.Skip("shared Redpanda did not start")
	}
	return fixtures.rpAddr
}
