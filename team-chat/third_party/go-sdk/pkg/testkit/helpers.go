package testkit

import (
	"net"
	"testing"
	"time"
)

// FreePort returns a TCP port on 127.0.0.1 that was free a moment ago. The port is
// released before returning, so another process can in theory take it first.
func FreePort(t testing.TB) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// Eventually polls fn every 10ms until it returns true and fails the test if
// timeout elapses first. fn runs on the calling goroutine.
func Eventually(t testing.TB, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v", timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
