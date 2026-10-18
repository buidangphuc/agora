package testkit

import (
	"runtime"
	"sync"
	"testing"
)

// fakeTB records how a call ended. Fatal and Skip stop the calling goroutine as
// the real ones do.
type fakeTB struct {
	testing.TB
	mu      sync.Mutex
	fatal   bool
	skipped bool
}

func (*fakeTB) Helper() {}

func (f *fakeTB) Fatalf(string, ...any) {
	f.mu.Lock()
	f.fatal = true
	f.mu.Unlock()
	runtime.Goexit()
}

func (f *fakeTB) Skip(...any) {
	f.mu.Lock()
	f.skipped = true
	f.mu.Unlock()
	runtime.Goexit()
}

// call runs one SharedPostgres-style call to completion on its own goroutine.
func call(s *sharedPostgres, start func(testing.TB) string) *fakeTB {
	f := &fakeTB{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.get(f, start)
	}()
	<-done
	return f
}

func failingStart(tb testing.TB) string {
	tb.Fatalf("docker down")
	return ""
}

func TestSharedPostgresFailsEveryCallerWhenDockerRequired(t *testing.T) {
	t.Setenv(requireDockerEnv, "1")
	var s sharedPostgres
	for i := range 3 {
		if f := call(&s, failingStart); !f.fatal || f.skipped {
			t.Errorf("caller %d: fatal=%v skipped=%v, want a failure", i+1, f.fatal, f.skipped)
		}
	}
}

func TestSharedPostgresSkipsLaterCallersWithoutRequirement(t *testing.T) {
	t.Setenv(requireDockerEnv, "")
	var s sharedPostgres
	call(&s, failingStart)
	if f := call(&s, failingStart); !f.skipped || f.fatal {
		t.Errorf("later caller: fatal=%v skipped=%v, want a skip", f.fatal, f.skipped)
	}
}
