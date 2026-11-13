package forecaststore_test

import (
	"bufio"
	"context"
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
	ln      net.Listener
	mu      sync.Mutex
	data    map[string]string
	gets    []string
	selects []string
	stall   chan struct{}
}

func startFake(t *testing.T) *fakeRedis {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRedis{ln: ln, data: map[string]string{}, stall: make(chan struct{})}
	t.Cleanup(func() {
		close(f.stall)
		_ = ln.Close()
	})
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

func (f *fakeRedis) url(db int) string {
	return fmt.Sprintf("redis://%s/%d", f.ln.Addr().String(), db)
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

func (f *fakeRedis) selectCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.selects...)
}

func readCmd(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("unexpected %q", line)
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
			select {
			case <-f.stall:
				return
			default:
			}
			if f.stallOn() {
				<-f.stall
				return
			}
			if !ok {
				_, _ = c.Write([]byte("$-1\r\n"))
			} else {
				_, _ = fmt.Fprintf(c, "$%d\r\n%s\r\n", len(v), v)
			}
		case "SELECT":
			f.mu.Lock()
			f.selects = append(f.selects, args[1])
			f.mu.Unlock()
			_, _ = c.Write([]byte("+OK\r\n"))
		case "PING":
			_, _ = c.Write([]byte("+PONG\r\n"))
		case "CLIENT":
			_, _ = c.Write([]byte("+OK\r\n"))
		default:
			_, _ = c.Write([]byte("-ERR unknown command\r\n"))
		}
	}
}

var stallMode sync.Map

func (f *fakeRedis) stallOn() bool { _, ok := stallMode.Load(f); return ok }
func (f *fakeRedis) enableStall()  { stallMode.Store(f, true) }

const goodKey = "fc:v1:seller:s-1:l-1"

func points(n int) string {
	var parts []string
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf(`{"date":"2026-11-%02d","p10":%d,"p50":%d,"p90":%d}`, 14+i%10, i%3, i%3+2, i%3+4))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func payload(pts string) string {
	return `{"seller_id":"s-1","listing_id":"l-1","model_version":"seasonal_naive_v1","generated_at":"2026-11-13T02:45:00Z","is_cold_start":false,"daily_forecasts":` + pts + `}`
}

func newStore(t *testing.T, url string, timeout time.Duration) *forecaststore.Store {
	t.Helper()
	s, err := forecaststore.New(url, timeout)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// Scenario: A forecast key is served as written (store-level decode, DB 3, validated key)
func TestGetDecodesStoredPayload_AForecastKeyIsServedAsWritten(t *testing.T) {
	f := startFake(t)
	f.set(goodKey, payload(points(14)))
	s := newStore(t, f.url(3), 500*time.Millisecond)
	got, found, err := s.Get(context.Background(), "s-1", "l-1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.ModelVersion != "seasonal_naive_v1" || got.IsColdStart || len(got.Points) != 14 {
		t.Fatalf("unexpected forecast: %+v", got)
	}
	want := time.Date(2026, 11, 13, 2, 45, 0, 0, time.UTC)
	if !got.GeneratedAt.Equal(want) {
		t.Fatalf("generated_at = %v want %v", got.GeneratedAt, want)
	}
	if got.Points[1].P10 != 1 || got.Points[1].P50 != 3 || got.Points[1].P90 != 5 {
		t.Fatalf("point 1 = %+v", got.Points[1])
	}
	if calls := f.getCalls(); len(calls) != 1 || calls[0] != goodKey {
		t.Fatalf("GET calls = %v, want exactly [%s]", calls, goodKey)
	}
	if sel := f.selectCalls(); len(sel) == 0 || sel[0] != "3" {
		t.Fatalf("SELECT calls = %v, want DB 3", sel)
	}
}

// Scenario: No key means cold start without points (store reports not found, no error)
func TestGetMissingKeyIsNotFound_NoKeyMeansColdStart(t *testing.T) {
	f := startFake(t)
	s := newStore(t, f.url(3), 500*time.Millisecond)
	got, found, err := s.Get(context.Background(), "s-1", "l-1")
	if err != nil || found || len(got.Points) != 0 {
		t.Fatalf("got=%+v found=%v err=%v", got, found, err)
	}
}

// Scenario: A key marked cold start carries no invented numbers (flag preserved)
func TestGetKeepsStoredColdStartFlag(t *testing.T) {
	f := startFake(t)
	f.set(goodKey, strings.Replace(payload(points(14)), `"is_cold_start":false`, `"is_cold_start":true`, 1))
	s := newStore(t, f.url(3), 500*time.Millisecond)
	got, found, err := s.Get(context.Background(), "s-1", "l-1")
	if err != nil || !found || !got.IsColdStart {
		t.Fatalf("got=%+v found=%v err=%v", got, found, err)
	}
}

// Scenario: A malformed payload returns UNAVAILABLE
func TestGetMalformedPayloadErrors_AMalformedPayloadReturnsUnavailable(t *testing.T) {
	big := payload(points(14))
	big = big[:len(big)-1] + `,"pad":"` + strings.Repeat("x", 64*1024) + `"}`
	exactlyOver := strings.Replace(payload(points(14)), `"is_cold_start":false`, `"is_cold_start":false,"pad":"`+strings.Repeat("y", 64*1024)+`"`, 1)
	cases := map[string]string{
		"invalid json":         "{not json",
		"empty string":         "",
		"json array":           "[]",
		"null":                 "null",
		"points not a list":    payload(`"oops"`),
		"p10 greater than p50": payload(`[{"date":"2026-11-14","p10":5,"p50":3,"p90":9}]`),
		"p50 greater than p90": payload(`[{"date":"2026-11-14","p10":1,"p50":9,"p90":3}]`),
		"negative p10":         payload(`[{"date":"2026-11-14","p10":-1,"p50":3,"p90":9}]`),
		"negative p50":         payload(`[{"date":"2026-11-14","p10":0,"p50":-3,"p90":9}]`),
		"non-finite literal":   payload(`[{"date":"2026-11-14","p10":0,"p50":NaN,"p90":9}]`),
		"overflow number":      payload(`[{"date":"2026-11-14","p10":0,"p50":1,"p90":1e999}]`),
		"quantity as string":   payload(`[{"date":"2026-11-14","p10":"0","p50":1,"p90":2}]`),
		"bad generated_at":     strings.Replace(payload(points(14)), "2026-11-13T02:45:00Z", "yesterday", 1),
		"over 64 KiB":          big,
		"over 64 KiB padded":   exactlyOver,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			f := startFake(t)
			f.set(goodKey, raw)
			s := newStore(t, f.url(3), 500*time.Millisecond)
			got, found, err := s.Get(context.Background(), "s-1", "l-1")
			if err == nil {
				t.Fatalf("want error, got forecast %+v found=%v", got, found)
			}
			if found || len(got.Points) != 0 {
				t.Fatalf("an error must carry no numbers: %+v found=%v", got, found)
			}
		})
	}
}

// Scenario: A malformed payload returns UNAVAILABLE (boundary: equal quantities are valid)
func TestGetAcceptsEqualQuantilesAndZero_AMalformedPayloadReturnsUnavailable(t *testing.T) {
	f := startFake(t)
	f.set(goodKey, payload(`[{"date":"2026-11-14","p10":0,"p50":0,"p90":0}]`))
	s := newStore(t, f.url(3), 500*time.Millisecond)
	got, found, err := s.Get(context.Background(), "s-1", "l-1")
	if err != nil || !found || len(got.Points) != 1 {
		t.Fatalf("got=%+v found=%v err=%v", got, found, err)
	}
}

// Scenario: A malformed payload returns UNAVAILABLE (boundary: just under the 64 KiB cap is accepted)
func TestGetAcceptsPayloadUnderCap_AMalformedPayloadReturnsUnavailable(t *testing.T) {
	raw := payload(points(14))
	pad := 64*1024 - len(raw) - len(`,"pad":""`) - 16
	raw = raw[:len(raw)-1] + `,"pad":"` + strings.Repeat("z", pad) + `"}`
	if len(raw) > 64*1024 {
		t.Fatalf("test bug: %d", len(raw))
	}
	f := startFake(t)
	f.set(goodKey, raw)
	s := newStore(t, f.url(3), 500*time.Millisecond)
	if _, found, err := s.Get(context.Background(), "s-1", "l-1"); err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

// Scenario: An invalid listing or seller id is rejected (key built only from validated ids)
func TestGetRejectsUnsafeIDsWithoutRead_KeyBuiltOnlyFromValidatedIDs(t *testing.T) {
	cases := []struct{ seller, listing string }{
		{"s-1", "x:*"}, {"", "l-1"}, {"s-1", ""}, {"s:1", "l-1"}, {"s-1", "l 1"},
		{"s-1", "l\r\nDEL"}, {"*", "l-1"}, {"s-1", "../l"}, {"s-1", strings.Repeat("a", 300)},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%q/%q", c.seller, c.listing), func(t *testing.T) {
			f := startFake(t)
			s := newStore(t, f.url(3), 500*time.Millisecond)
			_, found, err := s.Get(context.Background(), c.seller, c.listing)
			if err == nil || found {
				t.Fatalf("want error and not found, got found=%v err=%v", found, err)
			}
			if calls := f.getCalls(); len(calls) != 0 {
				t.Fatalf("Redis was read with %v", calls)
			}
		})
	}
}

// Scenario: An unset store address returns UNAVAILABLE
func TestUnsetAddressAlwaysErrors_AnUnsetStoreAddressReturnsUnavailable(t *testing.T) {
	s, err := forecaststore.New("", 25*time.Millisecond)
	if err != nil {
		t.Fatalf("an unset address must build a failing store, not fail New: %v", err)
	}
	got, found, err := s.Get(context.Background(), "s-1", "l-1")
	if err == nil || found || len(got.Points) != 0 {
		t.Fatalf("got=%+v found=%v err=%v", got, found, err)
	}
}

// Scenario: Redis unreachable returns UNAVAILABLE
func TestUnreachableRedisErrors_RedisUnreachableReturnsUnavailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	s := newStore(t, "redis://"+addr+"/3", 200*time.Millisecond)
	_, found, err := s.Get(context.Background(), "s-1", "l-1")
	if err == nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

// Scenario: A read stays within the latency budget (a stalled Redis is cut off by the read timeout)
func TestGetHonoursReadTimeout_AReadStaysWithinTheLatencyBudget(t *testing.T) {
	f := startFake(t)
	f.set(goodKey, payload(points(14)))
	f.enableStall()
	s := newStore(t, f.url(3), 25*time.Millisecond)
	start := time.Now()
	_, found, err := s.Get(context.Background(), "s-1", "l-1")
	elapsed := time.Since(start)
	if err == nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("Get took %v with a 25ms read timeout", elapsed)
	}
}

// Scenario: A read stays within the latency budget (timeout applies even with a background context)
func TestGetTimeoutIsConfigurable_AReadStaysWithinTheLatencyBudget(t *testing.T) {
	f := startFake(t)
	f.set(goodKey, payload(points(14)))
	f.enableStall()
	s := newStore(t, f.url(3), 100*time.Millisecond)
	start := time.Now()
	_, _, err := s.Get(context.Background(), "s-1", "l-1")
	if err == nil {
		t.Fatal("want timeout error")
	}
	if time.Since(start) < 90*time.Millisecond {
		t.Fatalf("returned in %v, before the configured 100ms timeout", time.Since(start))
	}
}

// Scenario: Redis unreachable returns UNAVAILABLE (cancelled caller context)
func TestGetCancelledContextErrors_RedisUnreachableReturnsUnavailable(t *testing.T) {
	f := startFake(t)
	f.set(goodKey, payload(points(14)))
	s := newStore(t, f.url(3), 500*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, found, err := s.Get(ctx, "s-1", "l-1"); err == nil || found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}
