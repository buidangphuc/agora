package pgtest

import (
	"os"
	"sync"
	"testing"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"
)

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
	pgOnce   sync.Once
	pgURL    string
	rpOnce   sync.Once
	rpAddr   string
}

func (s *shared) add(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanups = append(s.cleanups, f)
}

var fixtures shared

// Run is for TestMain: it runs the tests, then stops the shared containers.
func Run(m *testing.M) int {
	code := m.Run()
	fixtures.mu.Lock()
	defer fixtures.mu.Unlock()
	for i := len(fixtures.cleanups) - 1; i >= 0; i-- {
		fixtures.cleanups[i]()
	}
	return code
}

// URL returns the Postgres server tests use: TEST_DATABASE_URL, else one shared
// container started on first use.
func URL(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	fixtures.pgOnce.Do(func() { fixtures.pgURL = testkit.Postgres(keepTB{T: t, owner: &fixtures}) })
	if fixtures.pgURL == "" {
		t.Skip("shared Postgres did not start")
	}
	return fixtures.pgURL
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
