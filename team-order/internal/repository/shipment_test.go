package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-order/internal/repository"
)

func TestInMemoryShipmentRepository(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryShipmentRepository()

	// 1. Create Shipment
	s := repository.Shipment{
		OrderID:      "order-abc",
		Carrier:      "SPX",
		TrackingCode: "SPX-VN-123456",
		Checkpoints: []repository.ShipmentCheckpoint{
			{
				Timestamp:   time.Now(),
				Location:    "Kho Hub Tân Bình, TP.HCM",
				Description: "Đã tiếp nhận kiện hàng từ người bán",
			},
		},
	}

	created, err := repo.CreateShipment(ctx, s)
	if err != nil {
		t.Fatalf("CreateShipment failed: %v", err)
	}
	if created.ID == "" {
		t.Errorf("expected generated shipment ID")
	}
	if created.Status != repository.ShipmentStatusPending {
		t.Errorf("expected status PENDING, got %d", created.Status)
	}
	if len(created.Checkpoints) != 1 {
		t.Errorf("expected 1 checkpoint, got %d", len(created.Checkpoints))
	}

	// 2. Get Shipment by ID
	got, err := repo.GetShipment(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetShipment failed: %v", err)
	}
	if got.TrackingCode != "SPX-VN-123456" {
		t.Errorf("expected tracking code SPX-VN-123456, got %s", got.TrackingCode)
	}
	if len(got.Checkpoints) != 1 {
		t.Errorf("expected 1 checkpoint loaded, got %d", len(got.Checkpoints))
	}

	// 3. Get Shipment by Tracking Code
	byTracking, err := repo.GetShipmentByTrackingCode(ctx, "SPX-VN-123456")
	if err != nil {
		t.Fatalf("GetShipmentByTrackingCode failed: %v", err)
	}
	if byTracking.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, byTracking.ID)
	}

	// 4. Get Shipment by Order ID
	byOrder, err := repo.GetShipmentByOrderID(ctx, "order-abc")
	if err != nil {
		t.Fatalf("GetShipmentByOrderID failed: %v", err)
	}
	if byOrder.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, byOrder.ID)
	}

	// 5. Add Checkpoint
	newCp := repository.ShipmentCheckpoint{
		ShipmentID:  created.ID,
		Timestamp:   time.Now(),
		Location:    "Kho Trung Chuyển Củ Chi",
		Description: "Kiện hàng đang trung chuyển đến bưu cục phát",
	}
	addedCp, err := repo.AddShipmentCheckpoint(ctx, newCp)
	if err != nil {
		t.Fatalf("AddShipmentCheckpoint failed: %v", err)
	}
	if addedCp.ID == "" {
		t.Errorf("expected generated checkpoint ID")
	}

	// Verify loaded checkpoints
	updatedShipment, err := repo.GetShipment(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetShipment after checkpoint add failed: %v", err)
	}
	if len(updatedShipment.Checkpoints) != 2 {
		t.Errorf("expected 2 checkpoints, got %d", len(updatedShipment.Checkpoints))
	}

	// 6. Update Status
	updated, err := repo.UpdateShipmentStatus(ctx, created.ID, repository.ShipmentStatusInTransit)
	if err != nil {
		t.Fatalf("UpdateShipmentStatus failed: %v", err)
	}
	if updated.Status != repository.ShipmentStatusInTransit {
		t.Errorf("expected status IN_TRANSIT, got %d", updated.Status)
	}

	// 7. Not Found checks
	if _, err := repo.GetShipment(ctx, "non-existent"); err != repository.ErrShipmentNotFound {
		t.Errorf("expected ErrShipmentNotFound, got %v", err)
	}
	if _, err := repo.GetShipmentByTrackingCode(ctx, "non-existent"); err != repository.ErrShipmentNotFound {
		t.Errorf("expected ErrShipmentNotFound, got %v", err)
	}
	if _, err := repo.GetShipmentByOrderID(ctx, "non-existent"); err != repository.ErrShipmentNotFound {
		t.Errorf("expected ErrShipmentNotFound, got %v", err)
	}
}
