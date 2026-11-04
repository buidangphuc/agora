package flashsale

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
)

const idemTTL = 24 * time.Hour

type idemRec struct {
	hash       string
	campaignID string
	at         time.Time
}

type idemRepo struct {
	*storefake.InMemoryFlashSaleRepository
	log       *callLog
	mu        sync.Mutex
	recs      map[string]idemRec
	cancelled map[string]time.Time
	created   int
	lookups   int
}

func newIdemRepo(mem *storefake.InMemoryFlashSaleRepository, log *callLog) *idemRepo {
	return &idemRepo{InMemoryFlashSaleRepository: mem, log: log, recs: map[string]idemRec{}, cancelled: map[string]time.Time{}}
}

func (r *idemRepo) Create(ctx context.Context, c store.FlashSaleCampaign, event func(store.FlashSaleCampaign) (outbox.Row, error)) (store.FlashSaleCampaign, error) {
	r.log.add("insert")
	out, err := r.InMemoryFlashSaleRepository.Create(ctx, c, event)
	if err == nil {
		r.mu.Lock()
		r.created++
		r.mu.Unlock()
	}
	return out, err
}

func (r *idemRepo) Cancel(ctx context.Context, id string, now time.Time) (time.Time, error) {
	r.log.add("store")
	if _, err := r.GetByID(ctx, id); err != nil {
		return time.Time{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if at, ok := r.cancelled[id]; ok {
		return at, nil
	}
	r.cancelled[id] = now
	return now, nil
}

func (r *idemRepo) LookupIdempotent(ctx context.Context, actorID, key string, now time.Time) (store.IdempotentCampaign, error) {
	r.mu.Lock()
	r.lookups++
	rec, ok := r.recs[actorID+"|"+key]
	r.mu.Unlock()
	if !ok || now.Sub(rec.at) > idemTTL {
		return store.IdempotentCampaign{}, store.ErrIdempotencyNotFound
	}
	c, err := r.GetByID(ctx, rec.campaignID)
	if err != nil {
		return store.IdempotentCampaign{}, err
	}
	return store.IdempotentCampaign{Campaign: c, PayloadHash: rec.hash}, nil
}

func (r *idemRepo) CreateIdempotent(ctx context.Context, actorID, key, payloadHash string, c store.FlashSaleCampaign, now time.Time, event func(store.FlashSaleCampaign) (outbox.Row, error)) (store.FlashSaleCampaign, bool, error) {
	r.log.add("insert")
	r.mu.Lock()
	rec, ok := r.recs[actorID+"|"+key]
	r.mu.Unlock()
	if ok && now.Sub(rec.at) <= idemTTL {
		if rec.hash != payloadHash {
			return store.FlashSaleCampaign{}, false, store.ErrIdempotencyMismatch
		}
		out, err := r.InMemoryFlashSaleRepository.GetByID(ctx, rec.campaignID)
		return out, false, err
	}
	out, err := r.InMemoryFlashSaleRepository.Create(ctx, c, event)
	if err != nil {
		return store.FlashSaleCampaign{}, false, err
	}
	r.mu.Lock()
	r.recs[actorID+"|"+key] = idemRec{hash: payloadHash, campaignID: out.ID, at: now}
	r.created++
	r.mu.Unlock()
	return out, true, nil
}

func (r *idemRepo) createdCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created
}

func (r *idemRepo) recCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.recs)
}

func (r *idemRepo) lookupCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lookups
}

type createEnv struct {
	svc      *Service
	repo     *idemRepo
	audit    *recordingAuditWriter
	listings *fakeListings
	events   *eventLog
	clk      *fakeClock
}

func newCreateEnv(t *testing.T) createEnv {
	t.Helper()
	log := &callLog{}
	repo := newIdemRepo(storefake.NewInMemoryFlashSaleRepository(), log)
	aw := &recordingAuditWriter{log: log}
	ls := &fakeListings{owners: map[string]string{"l1": "seller-1", "theirs": "seller-2"}, price: 100000}
	events := &eventLog{}
	clk := &fakeClock{t: svcT0}
	svc := New(repo, nil, newFlags(t, flagProvider(true)), ls, auditx.NewGate(aw, nil), events.build, nil)
	svc.nowFn = clk.now
	return createEnv{svc: svc, repo: repo, audit: aw, listings: ls, events: events, clk: clk}
}

func adminParams() CreateCampaignParams {
	return CreateCampaignParams{
		ListingID: "l1", SalePrice: 80000, StockCap: 10,
		StartsAt: svcT0.Add(time.Hour), EndsAt: svcT0.Add(2 * time.Hour),
		CallerID: "admin-1", IsAdmin: true, IdempotencyKey: "retry-key-0001",
	}
}

func sellerParams() CreateCampaignParams {
	p := adminParams()
	p.CallerID, p.IsAdmin, p.IdempotencyKey = "seller-1", false, ""
	return p
}

func assertInvalidField(t *testing.T, err error, field string) {
	t.Helper()
	if !errors.Is(err, ErrInvalidCampaign) {
		t.Fatalf("err = %v, want ErrInvalidCampaign naming %s", err, field)
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("err = %q does not name %s", err.Error(), field)
	}
}

func (e createEnv) assertNothingCreated(t *testing.T) {
	t.Helper()
	if n := e.repo.createdCount(); n != 0 {
		t.Fatalf("created %d campaigns, want 0", n)
	}
	if n := len(e.audit.got()); n != 0 {
		t.Fatalf("audit events = %d, want 0", n)
	}
	if n := e.repo.recCount(); n != 0 {
		t.Fatalf("idempotency records = %d, want 0", n)
	}
	if n := e.events.count(); n != 0 {
		t.Fatalf("events built = %d, want 0", n)
	}
}

func TestCreateCampaignPriceNotBelowListingPrice(t *testing.T) {
	for name, price := range map[string]int64{"equal to listing price": 100000, "above listing price": 100001, "zero": 0, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			e := newCreateEnv(t)
			p := adminParams()
			p.SalePrice = price
			_, err := e.svc.CreateCampaign(t.Context(), p)
			assertInvalidField(t, err, "sale_price")
			e.assertNothingCreated(t)
		})
	}
}

func TestCreateCampaignSellerPathValidatesPrice(t *testing.T) {
	for name, price := range map[string]int64{"equal": 100000, "above": 100001} {
		t.Run(name, func(t *testing.T) {
			e := newCreateEnv(t)
			p := sellerParams()
			p.SalePrice = price
			_, err := e.svc.CreateCampaign(t.Context(), p)
			assertInvalidField(t, err, "sale_price")
			e.assertNothingCreated(t)
		})
	}
}

func TestCreateCampaignFieldValidation(t *testing.T) {
	now := svcT0
	cases := []struct {
		name   string
		mutate func(*CreateCampaignParams)
		field  string
	}{
		{"stock cap zero", func(p *CreateCampaignParams) { p.StockCap = 0 }, "stock_cap"},
		{"stock cap negative", func(p *CreateCampaignParams) { p.StockCap = -5 }, "stock_cap"},
		{"ends equal starts", func(p *CreateCampaignParams) { p.EndsAt = p.StartsAt }, "ends_at"},
		{"ends before starts", func(p *CreateCampaignParams) { p.EndsAt = p.StartsAt.Add(-time.Second) }, "ends_at"},
		{"ends in the past", func(p *CreateCampaignParams) {
			p.StartsAt, p.EndsAt = now.Add(-2*time.Hour), now.Add(-time.Hour)
		}, "ends_at"},
		{"ends exactly now", func(p *CreateCampaignParams) {
			p.StartsAt, p.EndsAt = now.Add(-time.Second), now
		}, "ends_at"},
	}
	for _, path := range []string{"admin", "seller"} {
		for _, tc := range cases {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				e := newCreateEnv(t)
				p := adminParams()
				if path == "seller" {
					p = sellerParams()
				}
				tc.mutate(&p)
				_, err := e.svc.CreateCampaign(t.Context(), p)
				assertInvalidField(t, err, tc.field)
				e.assertNothingCreated(t)
			})
		}
	}
}

func TestCreateCampaignBoundaryAccepted(t *testing.T) {
	for _, path := range []string{"admin", "seller"} {
		t.Run(path, func(t *testing.T) {
			e := newCreateEnv(t)
			p := adminParams()
			if path == "seller" {
				p = sellerParams()
			}
			p.SalePrice, p.StockCap = 99999, 1
			p.StartsAt = svcT0.Add(time.Hour)
			p.EndsAt = p.StartsAt.Add(time.Second)
			c, err := e.svc.CreateCampaign(t.Context(), p)
			if err != nil {
				t.Fatalf("CreateCampaign: %v", err)
			}
			if c.ID == "" || c.SalePrice != 99999 || c.StockCap != 1 || !c.EndsAt.Equal(p.EndsAt) {
				t.Fatalf("campaign = %+v", c)
			}
			if !c.StartsAt.After(e.clk.now()) {
				t.Fatalf("campaign must be scheduled, starts_at %v now %v", c.StartsAt, e.clk.now())
			}
		})
	}
}

func TestCreateCampaignUnknownListing(t *testing.T) {
	for _, path := range []string{"admin", "seller"} {
		t.Run(path, func(t *testing.T) {
			e := newCreateEnv(t)
			p := adminParams()
			if path == "seller" {
				p = sellerParams()
			}
			p.ListingID = "ghost"
			_, err := e.svc.CreateCampaign(t.Context(), p)
			assertInvalidField(t, err, "listing_id")
			if errors.Is(err, listing.ErrLookupFailed) {
				t.Fatalf("unknown listing reported as lookup failure: %v", err)
			}
			e.assertNothingCreated(t)
		})
	}
}

func TestCreateCampaignSellerUnknownListing(t *testing.T) {
	e := newCreateEnv(t)
	p := sellerParams()
	p.ListingID = "ghost"
	_, err := e.svc.CreateCampaign(t.Context(), p)
	assertInvalidField(t, err, "listing_id")
	e.assertNothingCreated(t)
}

func TestCreateCampaignListingUnavailable(t *testing.T) {
	failures := map[string]error{
		"plain error":       errors.New("dial tcp: refused"),
		"deadline exceeded": status.Error(codes.DeadlineExceeded, "slow"),
		"unavailable":       status.Error(codes.Unavailable, "team-listing down"),
		"internal":          status.Error(codes.Internal, "boom"),
	}
	for _, path := range []string{"admin", "seller"} {
		for name, cause := range failures {
			t.Run(path+"/"+name, func(t *testing.T) {
				e := newCreateEnv(t)
				e.listings.err = cause
				p := adminParams()
				if path == "seller" {
					p = sellerParams()
				}
				_, err := e.svc.CreateCampaign(t.Context(), p)
				if !errors.Is(err, listing.ErrLookupFailed) {
					t.Fatalf("err = %v, want listing.ErrLookupFailed", err)
				}
				e.assertNothingCreated(t)
			})
		}
	}
}

func TestCreateCampaignAuditDownLeavesNoIdempotencyRecord(t *testing.T) {
	for name, cause := range auditFailures {
		t.Run(name, func(t *testing.T) {
			e := newCreateEnv(t)
			e.audit.err = cause
			_, err := e.svc.CreateCampaign(t.Context(), adminParams())
			assertAuditUnavailable(t, err)
			if n := e.repo.createdCount(); n != 0 {
				t.Fatalf("created %d campaigns despite audit failure", n)
			}
			if n := e.repo.recCount(); n != 0 {
				t.Fatalf("idempotency records = %d, want 0", n)
			}
			e.audit.err = nil
			c, err := e.svc.CreateCampaign(t.Context(), adminParams())
			if err != nil || c.ID == "" {
				t.Fatalf("retry with same key after audit recovery = %+v, %v", c, err)
			}
			if n := e.repo.createdCount(); n != 1 {
				t.Fatalf("created = %d, want 1", n)
			}
		})
	}
}

func TestCreateCampaignRetryReturnsSameCampaign(t *testing.T) {
	e := newCreateEnv(t)
	first, err := e.svc.CreateCampaign(t.Context(), adminParams())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	listingCalls := e.listings.calls
	second, err := e.svc.CreateCampaign(t.Context(), adminParams())
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if first.ID == "" || second.ID != first.ID {
		t.Fatalf("ids = %q then %q, want identical", first.ID, second.ID)
	}
	if n := e.repo.createdCount(); n != 1 {
		t.Fatalf("created = %d, want 1", n)
	}
	if n := len(e.audit.got()); n != 1 {
		t.Fatalf("audit events = %d, want 1", n)
	}
	if n := e.events.count(); n != 1 {
		t.Fatalf("events built = %d, want 1", n)
	}
	if e.listings.calls != listingCalls {
		t.Fatalf("retry made %d extra listing lookups, want 0", e.listings.calls-listingCalls)
	}
}

func TestCreateCampaignSameKeyDifferentPayload(t *testing.T) {
	changes := map[string]func(*CreateCampaignParams){
		"sale price": func(p *CreateCampaignParams) { p.SalePrice = 70000 },
		"stock cap":  func(p *CreateCampaignParams) { p.StockCap = 11 },
		"variant":    func(p *CreateCampaignParams) { p.VariantID = "v2" },
		"starts at":  func(p *CreateCampaignParams) { p.StartsAt = p.StartsAt.Add(time.Minute) },
		"ends at":    func(p *CreateCampaignParams) { p.EndsAt = p.EndsAt.Add(time.Minute) },
		"listing":    func(p *CreateCampaignParams) { p.ListingID = "theirs" },
	}
	for name, mutate := range changes {
		t.Run(name, func(t *testing.T) {
			e := newCreateEnv(t)
			first, err := e.svc.CreateCampaign(t.Context(), adminParams())
			if err != nil {
				t.Fatalf("first: %v", err)
			}
			p := adminParams()
			mutate(&p)
			_, err = e.svc.CreateCampaign(t.Context(), p)
			if !errors.Is(err, store.ErrIdempotencyMismatch) {
				t.Fatalf("err = %v, want store.ErrIdempotencyMismatch", err)
			}
			if n := e.repo.createdCount(); n != 1 {
				t.Fatalf("created = %d, want 1", n)
			}
			if n := len(e.audit.got()); n != 1 {
				t.Fatalf("audit events = %d, want 1", n)
			}
			stored, err := e.repo.GetByID(t.Context(), first.ID)
			if err != nil || stored.SalePrice != 80000 || stored.StockCap != 10 {
				t.Fatalf("original campaign = %+v, %v; want unchanged", stored, err)
			}
		})
	}
}

func TestCreateCampaignPayloadHashIsSHA256Hex(t *testing.T) {
	e := newCreateEnv(t)
	if _, err := e.svc.CreateCampaign(t.Context(), adminParams()); err != nil {
		t.Fatal(err)
	}
	p := adminParams()
	p.IdempotencyKey = "retry-key-0002"
	p.SalePrice = 70000
	if _, err := e.svc.CreateCampaign(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	e.repo.mu.Lock()
	defer e.repo.mu.Unlock()
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	a, b := e.repo.recs["admin-1|retry-key-0001"], e.repo.recs["admin-1|retry-key-0002"]
	if !hex64.MatchString(a.hash) || !hex64.MatchString(b.hash) {
		t.Fatalf("hashes = %q, %q; want lowercase sha256 hex", a.hash, b.hash)
	}
	if a.hash == b.hash {
		t.Fatal("different payloads produced the same hash")
	}
}

func TestCreateCampaignIdempotencyScopeAndExpiry(t *testing.T) {
	e := newCreateEnv(t)
	mk := func(actor string) CreateCampaignParams {
		p := adminParams()
		p.CallerID = actor
		p.StartsAt, p.EndsAt = svcT0.Add(25*time.Hour), svcT0.Add(40*time.Hour)
		return p
	}
	a, err := e.svc.CreateCampaign(t.Context(), mk("admin-a"))
	if err != nil {
		t.Fatalf("admin a: %v", err)
	}
	b, err := e.svc.CreateCampaign(t.Context(), mk("admin-b"))
	if err != nil {
		t.Fatalf("admin b with a's key: %v", err)
	}
	if b.ID == a.ID || e.repo.createdCount() != 2 {
		t.Fatalf("admin b got %q (a %q), created %d; want an independent campaign", b.ID, a.ID, e.repo.createdCount())
	}

	e.clk.advance(idemTTL - time.Minute)
	p := mk("admin-a")
	p.SalePrice = 70000
	if _, err := e.svc.CreateCampaign(t.Context(), p); !errors.Is(err, store.ErrIdempotencyMismatch) {
		t.Fatalf("reuse just before expiry: err = %v, want store.ErrIdempotencyMismatch", err)
	}

	e.clk.advance(time.Minute + time.Second)
	again, err := e.svc.CreateCampaign(t.Context(), p)
	if err != nil {
		t.Fatalf("reuse after expiry: %v", err)
	}
	if again.ID == a.ID || again.SalePrice != 70000 {
		t.Fatalf("after expiry got %+v, want a new campaign with the new payload", again)
	}
	if n := e.repo.createdCount(); n != 3 {
		t.Fatalf("created = %d, want 3", n)
	}
}

func TestCreateCampaignReplayAfterEndsAt(t *testing.T) {
	e := newCreateEnv(t)
	first, err := e.svc.CreateCampaign(t.Context(), adminParams())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	listingCalls := e.listings.calls
	e.clk.advance(3 * time.Hour)
	replay, err := e.svc.CreateCampaign(t.Context(), adminParams())
	if err != nil {
		t.Fatalf("replay after ends_at: %v", err)
	}
	if replay.ID != first.ID {
		t.Fatalf("replay id = %q, want %q", replay.ID, first.ID)
	}
	if e.listings.calls != listingCalls {
		t.Fatalf("replay made %d listing lookups, want 0", e.listings.calls-listingCalls)
	}
	if n, a := e.repo.createdCount(), len(e.audit.got()); n != 1 || a != 1 {
		t.Fatalf("created %d, audit %d; want 1 and 1", n, a)
	}
}

func TestCreateCampaignReplayAfterCancel(t *testing.T) {
	e := newCreateEnv(t)
	first, err := e.svc.CreateCampaign(t.Context(), adminParams())
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := e.svc.CancelCampaign(t.Context(), first.ID, "admin-1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	listingCalls := e.listings.calls
	replay, err := e.svc.CreateCampaign(t.Context(), adminParams())
	if err != nil {
		t.Fatalf("replay after cancel: %v", err)
	}
	if replay.ID != first.ID {
		t.Fatalf("replay id = %q, want %q", replay.ID, first.ID)
	}
	if n := e.repo.createdCount(); n != 1 {
		t.Fatalf("created = %d, want 1", n)
	}
	var actions []string
	for _, ev := range e.audit.got() {
		actions = append(actions, ev.Action)
	}
	if fmt.Sprint(actions) != "[flash_sale.admin_create campaign.cancel]" {
		t.Fatalf("audit actions = %v, want create then cancel only", actions)
	}
	if e.listings.calls != listingCalls {
		t.Fatalf("replay made listing lookups")
	}
}

func TestCreateCampaignSellerKeyIgnoredOwnershipApplies(t *testing.T) {
	e := newCreateEnv(t)
	denied := sellerParams()
	denied.ListingID, denied.IdempotencyKey = "theirs", "seller-key-0001"
	if _, err := e.svc.CreateCampaign(t.Context(), denied); !errors.Is(err, listing.ErrNotOwner) {
		t.Fatalf("foreign listing with key: err = %v, want listing.ErrNotOwner", err)
	}
	own := sellerParams()
	own.IdempotencyKey = "seller-key-0001"
	first, err := e.svc.CreateCampaign(t.Context(), own)
	if err != nil {
		t.Fatalf("own listing with key: %v", err)
	}
	second, err := e.svc.CreateCampaign(t.Context(), own)
	if err != nil {
		t.Fatalf("own listing with key again: %v", err)
	}
	if first.ID == "" || second.ID == first.ID {
		t.Fatalf("ids = %q, %q; a seller key must be ignored so each call creates", first.ID, second.ID)
	}
	if n := e.repo.recCount(); n != 0 {
		t.Fatalf("idempotency records = %d, want 0", n)
	}
	if n := e.repo.lookupCount(); n != 0 {
		t.Fatalf("idempotency lookups = %d, want 0 on the seller path", n)
	}
	if n := len(e.audit.got()); n != 0 {
		t.Fatalf("seller path wrote %d audit events", n)
	}
}

func TestCreateCampaignIdempotencyLookupPrecedesListingLookup(t *testing.T) {
	e := newCreateEnv(t)
	first, err := e.svc.CreateCampaign(t.Context(), adminParams())
	if err != nil {
		t.Fatal(err)
	}
	e.listings.err = errors.New("team-listing down")
	replay, err := e.svc.CreateCampaign(t.Context(), adminParams())
	if err != nil {
		t.Fatalf("replay while listing is down: %v, want the stored campaign", err)
	}
	if replay.ID != first.ID || e.repo.createdCount() != 1 {
		t.Fatalf("replay id %q (want %q), created %d (want 1)", replay.ID, first.ID, e.repo.createdCount())
	}
}
