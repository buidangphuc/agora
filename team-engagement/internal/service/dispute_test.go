package service_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

func TestDisputeService(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryDisputeRepository()
	svc := service.NewDisputeService(repo, nil)

	t.Run("CreateDispute Validations", func(t *testing.T) {
		// Empty order ID
		_, err := svc.CreateDispute(ctx, "", "buyer-1", "seller-1", "Hỏng hàng", nil)
		if err != service.ErrEmptyOrderID {
			t.Fatalf("expected ErrEmptyOrderID, got %v", err)
		}

		// Empty claimant ID
		_, err = svc.CreateDispute(ctx, "order-1", "", "seller-1", "Hỏng hàng", nil)
		if err != service.ErrEmptyClaimantID {
			t.Fatalf("expected ErrEmptyClaimantID, got %v", err)
		}

		// Empty defendant ID
		_, err = svc.CreateDispute(ctx, "order-1", "buyer-1", "", "Hỏng hàng", nil)
		if err != service.ErrEmptyDefendantID {
			t.Fatalf("expected ErrEmptyDefendantID, got %v", err)
		}

		// Claimant and Defendant same
		_, err = svc.CreateDispute(ctx, "order-1", "user-1", "user-1", "Hỏng hàng", nil)
		if err != service.ErrSameClaimantAndDef {
			t.Fatalf("expected ErrSameClaimantAndDef, got %v", err)
		}

		// Empty reason
		_, err = svc.CreateDispute(ctx, "order-1", "buyer-1", "seller-1", "   ", nil)
		if err != service.ErrEmptyReason {
			t.Fatalf("expected ErrEmptyReason, got %v", err)
		}
	})

	t.Run("Lifecycle: Create -> Get -> Resolve", func(t *testing.T) {
		disp, err := svc.CreateDispute(ctx, "order-101", "buyer-101", "seller-101", "Sản phẩm bị trầy xước nặng", []string{"https://img.com/scratch.png"})
		if err != nil {
			t.Fatalf("unexpected error creating dispute: %v", err)
		}
		if disp.ID == "" {
			t.Fatal("expected dispute ID")
		}
		if disp.Status != repository.DisputeStatusOpen {
			t.Fatalf("expected OPEN status, got %s", disp.Status)
		}

		// Get dispute validation
		_, err = svc.GetDispute(ctx, "")
		if err != service.ErrEmptyDisputeID {
			t.Fatalf("expected ErrEmptyDisputeID, got %v", err)
		}

		// Get dispute
		fetched, err := svc.GetDispute(ctx, disp.ID)
		if err != nil {
			t.Fatalf("unexpected error getting dispute: %v", err)
		}
		if fetched.Reason != "Sản phẩm bị trầy xước nặng" {
			t.Fatalf("unexpected reason: %s", fetched.Reason)
		}

		// Resolve dispute validation
		_, err = svc.ResolveDispute(ctx, "", repository.DisputeStatusResolved, "Xong")
		if err != service.ErrEmptyDisputeID {
			t.Fatalf("expected ErrEmptyDisputeID, got %v", err)
		}

		_, err = svc.ResolveDispute(ctx, disp.ID, repository.DisputeStatus("INVALID_STATUS"), "Xong")
		if err != service.ErrInvalidDisputeStatus {
			t.Fatalf("expected ErrInvalidDisputeStatus, got %v", err)
		}

		// Resolve dispute successfully
		resolved, err := svc.ResolveDispute(ctx, disp.ID, repository.DisputeStatusResolved, "Hoàn tiền 50% cho người mua")
		if err != nil {
			t.Fatalf("unexpected error resolving dispute: %v", err)
		}
		if resolved.Status != repository.DisputeStatusResolved {
			t.Fatalf("expected RESOLVED status, got %s", resolved.Status)
		}
		if resolved.Resolution != "Hoàn tiền 50% cho người mua" {
			t.Fatalf("unexpected resolution: %s", resolved.Resolution)
		}

		// Try resolving already closed dispute -> ErrDisputeClosed
		_, err = svc.ResolveDispute(ctx, disp.ID, repository.DisputeStatusRejected, "Từ chối sau khi đã đóng")
		if err != service.ErrDisputeClosed {
			t.Fatalf("expected ErrDisputeClosed, got %v", err)
		}
	})
}
