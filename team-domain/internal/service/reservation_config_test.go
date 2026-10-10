package service_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
)

// The configured TTL decides expires_at: with a 2-minute TTL a sweep at +3m
// restores the reservation, while with the default 15m the same sweep does not.
func TestConfiguredReservationTTLDrivesExpiry(t *testing.T) {
	ctx := context.Background()
	newRepo := func() *repository.InMemoryListingRepository {
		return repository.NewInMemoryListingRepository(repository.Listing{ID: "L1", Currency: "VND", Stock: 10})
	}

	repoS := newRepo()
	short := service.NewListingService(repoS).WithReservationTTL(2 * time.Minute)
	if short.ReservationTTL() != 2*time.Minute {
		t.Fatalf("ReservationTTL = %v, want 2m", short.ReservationTTL())
	}
	if err := short.ReserveStockIdempotent(ctx, "r1", "L1", "", 4); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if n, _ := short.SweepExpiredReservations(ctx, time.Now().Add(time.Minute)); n != 0 {
		t.Errorf("sweep at +1m released %d, want 0 (TTL 2m)", n)
	}
	if n, _ := short.SweepExpiredReservations(ctx, time.Now().Add(3*time.Minute)); n != 1 {
		t.Errorf("sweep at +3m released %d, want 1 (TTL 2m)", n)
	}
	if got := stockOf(t, repoS, "L1"); got != 10 {
		t.Errorf("stock = %d, want 10", got)
	}

	def := service.NewListingService(newRepo())
	if def.ReservationTTL() != service.DefaultReservationTTL {
		t.Errorf("default TTL = %v, want %v", def.ReservationTTL(), service.DefaultReservationTTL)
	}
	if err := def.ReserveStockIdempotent(ctx, "r1", "L1", "", 4); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if n, _ := def.SweepExpiredReservations(ctx, time.Now().Add(3*time.Minute)); n != 0 {
		t.Errorf("default TTL: sweep at +3m released %d, want 0", n)
	}

	if got := service.NewListingService(nil).WithReservationTTL(0).ReservationTTL(); got != service.DefaultReservationTTL {
		t.Errorf("WithReservationTTL(0) -> %v, want default", got)
	}
}

func TestSweeperUsesConfiguredInterval(t *testing.T) {
	svc := service.NewListingService(repository.NewInMemoryListingRepository())
	if got := service.NewReservationSweeper(svc, 7*time.Second, nil).Interval(); got != 7*time.Second {
		t.Errorf("interval = %v, want 7s", got)
	}
	for _, bad := range []time.Duration{0, -time.Second} {
		if got := service.NewReservationSweeper(svc, bad, nil).Interval(); got != service.DefaultSweepInterval {
			t.Errorf("interval(%v) = %v, want default %v", bad, got, service.DefaultSweepInterval)
		}
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Run logs the effective TTL and interval at start (as Go duration strings, in
// the JSON format the service runs with) and ticks at the configured interval.
func TestSweeperRunLogsEffectiveValuesAndTicks(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "L1", Currency: "VND", Stock: 10})
	svc := service.NewListingService(repo).WithReservationTTL(time.Millisecond)
	if err := svc.ReserveStockIdempotent(context.Background(), "r1", "L1", "", 4); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	var logs syncBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go service.NewReservationSweeper(svc, 10*time.Millisecond, logger).Run(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for stockOf(t, repo, "L1") != 10 {
		if time.Now().After(deadline) {
			t.Fatal("sweeper with a 10ms interval never restored the expired reservation")
		}
		time.Sleep(10 * time.Millisecond)
	}
	out := logs.String()
	for _, want := range []string{`"msg":"reservation sweeper started"`, `"reservation_ttl":"1ms"`, `"sweep_interval":"10ms"`} {
		if !strings.Contains(out, want) {
			t.Errorf("sweeper log %q lacks %q", out, want)
		}
	}
}
