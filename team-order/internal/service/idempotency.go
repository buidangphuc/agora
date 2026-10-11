package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/buidangphuc/team-order/internal/repository"
)

// IdempotencyKeyMetadata is the gRPC metadata key carrying the client's
// Idempotency-Key for CreateOrder. The gateway validates the HTTP header and
// forwards it under this key; team-order re-validates it (defence in depth).
const IdempotencyKeyMetadata = "idempotency-key"

// MaxIdempotencyKeyLen is the longest accepted key, in bytes (same rule as the
// gateway).
const MaxIdempotencyKeyLen = 255

// ErrInvalidIdempotencyKey: the key is empty, longer than MaxIdempotencyKeyLen or
// not printable ASCII. -> INVALID_ARGUMENT.
var ErrInvalidIdempotencyKey = errors.New("invalid idempotency key")

// ErrCheckoutInProgress: the first checkout with this key has not finished (it is
// running, or crashed and awaits the sweep). -> ABORTED (retryable).
var ErrCheckoutInProgress = errors.New("checkout with this idempotency key is in progress; retry")

// NormalizeIdempotencyKey trims surrounding whitespace and requires 1..255
// printable ASCII bytes (0x20-0x7E).
func NormalizeIdempotencyKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidIdempotencyKey)
	}
	if len(key) > MaxIdempotencyKeyLen {
		return "", fmt.Errorf("%w: longer than %d bytes", ErrInvalidIdempotencyKey, MaxIdempotencyKeyLen)
	}
	for i := 0; i < len(key); i++ {
		if c := key[i]; c < 0x20 || c > 0x7e {
			return "", fmt.Errorf("%w: must be printable ASCII", ErrInvalidIdempotencyKey)
		}
	}
	return key, nil
}

// CheckoutOption customises one CreateOrdersFromCart call.
type CheckoutOption func(*checkoutConfig)

type checkoutConfig struct {
	idempotencyKey string
}

// WithIdempotencyKey makes the checkout idempotent on key for this buyer. The key
// must already be normalised (NormalizeIdempotencyKey); "" means no key.
func WithIdempotencyKey(key string) CheckoutOption {
	return func(c *checkoutConfig) { c.idempotencyKey = key }
}

// replayCheckout answers a CreateOrder whose (buyer, key) already owns a saga. A
// COMPLETED saga returns the orders it placed (found through its reservations)
// and re-attempts the cart clear; anything else is still running (or crashed and
// awaits the sweep): ErrCheckoutInProgress. A compensated saga never matches,
// because compensation frees the key.
func (s *OrderService) replayCheckout(ctx context.Context, buyerID string, sg repository.Saga) ([]repository.Order, error) {
	if sg.Status != repository.SagaStatusCompleted {
		s.logger.InfoContext(ctx, "checkout with this idempotency key has not finished; asking the client to retry",
			slog.String("saga_id", sg.ID), slog.Int("saga_status", int(sg.Status)))
		return nil, ErrCheckoutInProgress
	}
	reservations, err := s.sagaRepo.ListReservationsBySaga(ctx, sg.ID)
	if err != nil {
		return nil, fmt.Errorf("replay checkout: load reservations of saga %s: %w", sg.ID, err)
	}
	seen := map[string]bool{}
	bound := map[string]bool{} // reservation ids that belong to a placed order
	var orders []repository.Order
	for _, res := range reservations {
		if res.OrderID == "" {
			continue
		}
		bound[res.ID] = true
		if seen[res.OrderID] {
			continue
		}
		seen[res.OrderID] = true
		o, gerr := s.orderRepo.GetOrder(ctx, res.OrderID)
		if gerr != nil {
			return nil, fmt.Errorf("replay checkout: load order %s of saga %s: %w", res.OrderID, sg.ID, gerr)
		}
		orders = append(orders, o)
	}
	if len(orders) == 0 {
		// Never invent new orders for a key whose orders cannot be found.
		return nil, fmt.Errorf("replay checkout: saga %s is completed but has no recorded orders", sg.ID)
	}
	sort.Slice(orders, func(i, j int) bool { return orders[i].SellerID < orders[j].SellerID })
	s.logger.InfoContext(ctx, "checkout replayed from idempotency key",
		slog.String("saga_id", sg.ID), slog.String("buyer_id", buyerID), slog.Int("order_count", len(orders)))

	// Re-attempt the cart clear: drop the cart items this saga checked out
	// (recognised by the reservation id they derive) that a failed clear left.
	cart, cerr := s.cartRepo.GetCart(ctx, buyerID)
	if cerr != nil {
		s.logger.WarnContext(ctx, "could not read the cart to clear it on replay",
			slog.String("buyer_id", buyerID), slog.Any("err", cerr))
		return orders, nil
	}
	var leftover []string
	for _, it := range cart {
		if bound[ReservationID(sg.ID, it)] {
			leftover = append(leftover, it.ID)
		}
	}
	s.clearCart(buyerID, leftover)
	return orders, nil
}
