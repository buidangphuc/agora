// Package upstreamtest holds test doubles for team-order's upstreams. It is
// imported only by tests.
package upstreamtest

import (
	"context"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-order/generated/platform/listing/v1"
)

// Reservation states, mirroring team-domain's reservations.status.
const (
	StateActive    = "active"
	StateCommitted = "committed"
	StateReleased  = "released"
)

type reservation struct {
	key   string // stock key: variant id if set, else listing id
	qty   int32
	state string
}

// Domain is an in-memory model of team-domain's stock RPCs with the reservation
// lifecycle of change port-order-inventory-correctness:
//
//   - ReserveStock: empty id -> INVALID_ARGUMENT; an existing active/committed id is
//     a successful no-op; a released id -> FAILED_PRECONDITION (stock unchanged);
//     short stock -> Success=false with no error (what team-domain answers).
//   - CommitReservation: active -> committed; committed -> OK; released ->
//     FAILED_PRECONDITION; unknown -> NOT_FOUND; empty -> INVALID_ARGUMENT.
//   - ReleaseStock: keyed on reservation_id only; restores the STORED quantity once
//     (active|committed -> released); unknown/released ids are a successful no-op;
//     empty id -> INVALID_ARGUMENT. The request's quantity is ignored.
//   - Sweep: restores every active reservation (the TTL sweep), never committed ones.
//
// Fault hooks let a test make a call fail before it has any effect.
type Domain struct {
	mu           sync.Mutex
	stock        map[string]int32
	reservations map[string]*reservation

	// ReserveErr, CommitErr and ReleaseErr, when set, are called first; a non-nil
	// result is returned and the call has no effect.
	ReserveErr func(req *listingv1.ReserveStockRequest) error
	CommitErr  func(id string) error
	ReleaseErr func(id string) error
	// ReleaseHang makes ReleaseStock block until its context is done, like a
	// gRPC call to a stopped team-domain, then fail with DeadlineExceeded.
	ReleaseHang bool

	Calls struct {
		Reserve, Commit, Release int
	}
	// ReleaseCtxErrs counts ReleaseStock calls made on an already-done context.
	ReleaseCtxErrs int
}

// NewDomain builds a fake with the given stock per listing (or variant) id.
func NewDomain(stock map[string]int32) *Domain {
	d := &Domain{stock: map[string]int32{}, reservations: map[string]*reservation{}}
	for k, v := range stock {
		d.stock[k] = v
	}
	return d
}

func stockKey(listingID, variantID string) string {
	if variantID != "" {
		return variantID
	}
	return listingID
}

// SetStock overwrites one listing's (or variant's) stock, e.g. a seller restock.
func (d *Domain) SetStock(key string, n int32) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stock[key] = n
}

// Stock reports one listing's (or variant's) current stock.
func (d *Domain) Stock(key string) int32 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stock[key]
}

// State reports a reservation's state ("" when unknown).
func (d *Domain) State(id string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r, ok := d.reservations[id]; ok {
		return r.state
	}
	return ""
}

// Reservations returns the number of reservations ever created.
func (d *Domain) Reservations() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.reservations)
}

// Sweep is team-domain's TTL sweep: every active reservation is released and its
// stock restored. It returns how many were released.
func (d *Domain) Sweep() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, r := range d.reservations {
		if r.state == StateActive {
			r.state = StateReleased
			d.stock[r.key] += r.qty
			n++
		}
	}
	return n
}

func (d *Domain) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), Stock: d.stock[req.GetId()]}}, nil
}

func (d *Domain) ReserveStock(_ context.Context, req *listingv1.ReserveStockRequest, _ ...grpc.CallOption) (*listingv1.ReserveStockResponse, error) {
	if d.ReserveErr != nil {
		if err := d.ReserveErr(req); err != nil {
			d.mu.Lock()
			d.Calls.Reserve++
			d.mu.Unlock()
			return nil, err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Calls.Reserve++
	id := req.GetReservationId()
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if r, ok := d.reservations[id]; ok {
		if r.state == StateReleased {
			return nil, status.Error(codes.FailedPrecondition, "reservation was released; use a new reservation_id")
		}
		return &listingv1.ReserveStockResponse{Success: true}, nil
	}
	key := stockKey(req.GetListingId(), req.GetVariantId())
	if d.stock[key] < req.GetQuantity() {
		return &listingv1.ReserveStockResponse{Success: false, Message: "insufficient stock"}, nil
	}
	d.stock[key] -= req.GetQuantity()
	d.reservations[id] = &reservation{key: key, qty: req.GetQuantity(), state: StateActive}
	return &listingv1.ReserveStockResponse{Success: true}, nil
}

func (d *Domain) CommitReservation(_ context.Context, req *listingv1.CommitReservationRequest, _ ...grpc.CallOption) (*listingv1.CommitReservationResponse, error) {
	id := req.GetReservationId()
	if d.CommitErr != nil {
		if err := d.CommitErr(id); err != nil {
			d.mu.Lock()
			d.Calls.Commit++
			d.mu.Unlock()
			return nil, err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Calls.Commit++
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	r, ok := d.reservations[id]
	switch {
	case !ok:
		return nil, status.Error(codes.NotFound, "reservation not found")
	case r.state == StateReleased:
		return nil, status.Error(codes.FailedPrecondition, "reservation already released")
	}
	r.state = StateCommitted
	return &listingv1.CommitReservationResponse{}, nil
}

func (d *Domain) ReleaseStock(ctx context.Context, req *listingv1.ReleaseStockRequest, _ ...grpc.CallOption) (*listingv1.ReleaseStockResponse, error) {
	id := req.GetReservationId()
	d.mu.Lock()
	d.Calls.Release++
	if ctx.Err() != nil {
		d.ReleaseCtxErrs++
	}
	hang := d.ReleaseHang
	d.mu.Unlock()
	if hang {
		<-ctx.Done()
		return nil, status.Error(codes.DeadlineExceeded, ctx.Err().Error())
	}
	if d.ReleaseErr != nil {
		if err := d.ReleaseErr(id); err != nil {
			return nil, err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "reservation_id is required")
	}
	if r, ok := d.reservations[id]; ok && r.state != StateReleased {
		r.state = StateReleased
		d.stock[r.key] += r.qty
	}
	return &listingv1.ReleaseStockResponse{Success: true}, nil
}
