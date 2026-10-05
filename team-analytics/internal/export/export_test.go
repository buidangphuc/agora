package export_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/export"
)

// fileExporter writes body to dst, or fails with err.
type fileExporter struct {
	body  string
	err   error
	calls atomic.Int32
	seen  []string
}

func (f *fileExporter) ExportParquet(_ context.Context, dst string) error {
	f.calls.Add(1)
	f.seen = append(f.seen, dst)
	if f.err != nil {
		_ = os.WriteFile(dst, []byte("partial"), 0o600)
		return f.err
	}
	return os.WriteFile(dst, []byte(f.body), 0o600)
}

func TestOnce_ReplacesAtomicallyViaTempFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &fileExporter{body: "new"}
	if err := export.Once(context.Background(), e, path); err != nil {
		t.Fatalf("Once: %v", err)
	}
	if e.seen[0] != path+".tmp" {
		t.Errorf("exported to %q, want the temp file %q", e.seen[0], path+".tmp")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("target = %q, want new", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind: %v", err)
	}
}

func TestOnce_FailureKeepsThePreviousFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &fileExporter{err: errors.New("disk full")}
	if err := export.Once(context.Background(), e, path); err == nil {
		t.Fatal("want an error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "old" {
		t.Errorf("target = %q, want the previous export kept", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("partial temp file left behind: %v", err)
	}
}

func TestEnabled_OffByDefault(t *testing.T) {
	for _, c := range []struct {
		path     string
		interval int
		want     bool
	}{{"", 0, false}, {"/data/x.parquet", 0, false}, {"", 60, false}, {"/data/x.parquet", 60, true}} {
		if got := export.Enabled(c.path, c.interval); got != c.want {
			t.Errorf("Enabled(%q, %d) = %v, want %v", c.path, c.interval, got, c.want)
		}
	}
}

func TestRun_FailuresDoNotStopTheLoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracking_events.parquet")
	e := &fileExporter{err: errors.New("boom")}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		export.Run(ctx, e, path, 5*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for e.calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if n := e.calls.Load(); n < 3 {
		t.Fatalf("export ran %d times; a failure must not stop the loop", n)
	}
}
