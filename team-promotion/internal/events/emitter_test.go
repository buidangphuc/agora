package events_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/events"
)

type failingPublisher struct{ err error }

func (f failingPublisher) Publish(context.Context, string, []byte) error { return f.err }

// A failed publish is returned to the caller, wrapped so errors.Is still finds
// the cause, and nothing is logged as published.
func TestPublishFailureIsReturned(t *testing.T) {
	var logs bytes.Buffer
	cause := errors.New("broker down")
	em := events.NewEmitter(failingPublisher{err: cause}, slog.New(slog.NewTextHandler(&logs, nil)))

	err := em.EmitVoucherChanged(context.Background(), &promotionv1.Voucher{Id: "v1"})
	if !errors.Is(err, cause) {
		t.Fatalf("EmitVoucherChanged err = %v, want the publish error", err)
	}
	err = em.EmitFlashSaleChanged(context.Background(), &promotionv1.FlashSaleCampaign{Id: "c1"})
	if !errors.Is(err, cause) {
		t.Fatalf("EmitFlashSaleChanged err = %v, want the publish error", err)
	}
	if strings.Contains(strings.ToLower(logs.String()), "published") {
		t.Fatalf("a failed publish was logged as published: %s", logs.String())
	}
}
