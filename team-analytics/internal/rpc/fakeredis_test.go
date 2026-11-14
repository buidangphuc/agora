package rpc_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/forecaststore"
)

type fakeRedis struct {
	ln   net.Listener
	mu   sync.Mutex
	data map[string]string
	gets []string
}

func startFakeRedis(t *testing.T) *fakeRedis {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRedis{ln: ln, data: map[string]string{}}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeRedis) set(k, v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[k] = v
}

func (f *fakeRedis) getCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.gets...)
}

func (f *fakeRedis) store(t *testing.T) *forecaststore.Store {
	t.Helper()
	s, err := forecaststore.New(fmt.Sprintf("redis://%s/3", f.ln.Addr().String()), 500*time.Millisecond)
	if err != nil {
		t.Fatalf("forecaststore.New: %v", err)
	}
	return s
}

func readCmd(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		l, _ := strconv.Atoi(strings.TrimSpace(hdr[1:]))
		buf := make([]byte, l+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:l]))
	}
	return args, nil
}

func (f *fakeRedis) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	for {
		args, err := readCmd(r)
		if err != nil {
			return
		}
		switch strings.ToUpper(args[0]) {
		case "GET":
			f.mu.Lock()
			f.gets = append(f.gets, args[1])
			v, ok := f.data[args[1]]
			f.mu.Unlock()
			if !ok {
				_, _ = c.Write([]byte("$-1\r\n"))
			} else {
				_, _ = fmt.Fprintf(c, "$%d\r\n%s\r\n", len(v), v)
			}
		case "SELECT", "CLIENT", "PING":
			_, _ = c.Write([]byte("+OK\r\n"))
		default:
			_, _ = c.Write([]byte("-ERR unknown command\r\n"))
		}
	}
}
