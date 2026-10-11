package service

import (
	"context"
	"fmt"
	"time"

	"github.com/buidangphuc/team-order/internal/repository"
)

// Saga step statuses. The wire field is a free string; SKIPPED renders neutrally
// in the storefront timeline like any unknown value.
const (
	SagaStepSuccess     = "SUCCESS"
	SagaStepPending     = "PENDING"
	SagaStepSkipped     = "SKIPPED"
	SagaStepCompensated = "COMPENSATED"
)

// CompensationReasonCancelled is the neutral compensation reason of a cancelled
// order (the view does not guess why it was cancelled).
const CompensationReasonCancelled = "order cancelled"

// SagaStepView is one step; At is nil when the time was not recorded (a time is
// never invented).
type SagaStepView struct {
	Name   string
	Status string
	Detail string
	At     *time.Time
}

// SagaView is what GetSagaState reports, built only from persisted facts: the
// order row, its reservations and orders.paid_at.
type SagaView struct {
	CurrentStep        string
	Steps              []SagaStepView
	IsCompensated      bool
	CompensationReason string
	// ReleasePending: the order is Cancelled and a reservation is not released yet.
	ReleasePending bool
}

const (
	stepNameCreated      = "1. Khởi tạo Đơn Hàng (Order Created)"
	stepNameReserved     = "2. Khóa Tồn Kho Sản Phẩm (Stock Reserved)"
	stepNamePayment      = "3. Thanh Toán (Payment Charged)"
	stepNameConfirmed    = "4. Xác Nhận & Giao Vận (Order Confirmed)"
	stepNameCompensation = "4. Hoàn Tác & Trả Tồn Kho (Compensation Executed)"
	paymentMethodCOD     = 1
)

// SagaView derives an order's saga view (spec order-read-access):
//
//	Order created   SUCCESS, orders.created_at
//	Stock reserved  SUCCESS, time of the earliest reservation (none recorded: no time)
//	Payment         Pending -> PENDING; paid_at set -> SUCCESS at paid_at;
//	                Cancelled without paid_at -> SKIPPED; a cash-on-delivery order
//	                shipped but not completed -> PENDING (collected on delivery);
//	                otherwise (paid before paid_at was recorded, COD completed) ->
//	                SUCCESS without a time
//	Confirmation    (not cancelled) Pending -> PENDING, otherwise SUCCESS
//	Compensation    (cancelled) every reservation RELEASED -> COMPENSATED and
//	                is_compensated; any still held -> PENDING; none recorded -> SKIPPED
func (s *OrderService) SagaView(ctx context.Context, order repository.Order) (SagaView, error) {
	reservations, err := s.sagaRepo.ListReservationsByOrder(ctx, order.ID)
	if err != nil {
		return SagaView{}, fmt.Errorf("load reservations of order %s: %w", order.ID, err)
	}

	created := order.CreatedAt
	view := SagaView{Steps: []SagaStepView{{
		Name: stepNameCreated, Status: SagaStepSuccess, At: &created,
		Detail: "Đơn hàng được ghi trên Order DB",
	}}}

	reserved := SagaStepView{Name: stepNameReserved, Status: SagaStepSuccess}
	if len(reservations) == 0 {
		reserved.Detail = "Không có bản ghi giữ tồn kho (đơn có từ trước khi theo dõi saga)"
	} else {
		first := reservations[0].CreatedAt
		for _, r := range reservations {
			if r.CreatedAt.Before(first) {
				first = r.CreatedAt
			}
		}
		reserved.At = &first
		reserved.Detail = "Đã giữ tồn kho ở team-domain"
	}
	view.Steps = append(view.Steps, reserved)

	cancelled := order.Status == repository.OrderStatusCancelled
	payment := SagaStepView{Name: stepNamePayment}
	switch {
	case order.PaidAt != nil:
		paidAt := *order.PaidAt
		payment.Status, payment.At, payment.Detail = SagaStepSuccess, &paidAt, "Thanh toán đã được xác nhận"
	case order.Status == repository.OrderStatusPending:
		payment.Status, payment.Detail = SagaStepPending, "Đang chờ thanh toán"
	case cancelled:
		payment.Status, payment.Detail = SagaStepSkipped, "Đơn bị hủy trước khi có thanh toán"
	case order.Status == repository.OrderStatusShipped && order.PaymentMethod == paymentMethodCOD:
		payment.Status, payment.Detail = SagaStepPending, "Thu tiền khi giao hàng (COD)"
	default:
		payment.Status, payment.Detail = SagaStepSuccess, "Thanh toán đã hoàn tất (không ghi nhận thời điểm)"
	}
	view.Steps = append(view.Steps, payment)

	if !cancelled {
		confirmation := SagaStepView{Name: stepNameConfirmed, Status: SagaStepSuccess, Detail: "Đơn hàng đã được xác nhận"}
		if order.Status == repository.OrderStatusPending {
			confirmation.Status, confirmation.Detail = SagaStepPending, "Chờ xác nhận"
		}
		view.Steps = append(view.Steps, confirmation)
		view.CurrentStep = currentStepFor(order.Status)
		return view, nil
	}

	view.CompensationReason = CompensationReasonCancelled
	compensation := SagaStepView{Name: stepNameCompensation}
	if len(reservations) == 0 {
		compensation.Status = SagaStepSkipped
		compensation.Detail = "Không có bản ghi giữ tồn kho để hoàn trả"
		view.CurrentStep = "Đã Hủy (Cancelled)"
		view.Steps = append(view.Steps, compensation)
		return view, nil
	}
	var lastRelease time.Time
	allReleased := true
	for _, r := range reservations {
		if r.Status != repository.ReservationStatusReleased {
			allReleased = false
			continue
		}
		if r.UpdatedAt.After(lastRelease) {
			lastRelease = r.UpdatedAt
		}
	}
	if allReleased {
		compensation.Status = SagaStepCompensated
		compensation.Detail = "Đã hoàn trả toàn bộ tồn kho (ReleaseStock)"
		compensation.At = &lastRelease
		view.IsCompensated = true
		view.CurrentStep = "Đã Hoàn Tác (Compensated)"
	} else {
		compensation.Status = SagaStepPending
		compensation.Detail = "Đơn đã hủy; hoàn trả tồn kho đang chờ thử lại (stock release pending retry)"
		view.ReleasePending = true
		view.CurrentStep = "Đang Hoàn Tác (Compensation pending)"
	}
	view.Steps = append(view.Steps, compensation)
	return view, nil
}

func currentStepFor(st repository.OrderStatus) string {
	switch st {
	case repository.OrderStatusPending:
		return "3. Chờ Thanh Toán (Awaiting Payment)"
	case repository.OrderStatusPaid:
		return "4. Đã Thanh Toán (Paid)"
	case repository.OrderStatusShipped:
		return "4. Đang Giao Hàng (Shipped)"
	case repository.OrderStatusCompleted:
		return "4. Đơn Hàng Hoàn Tất (Completed)"
	}
	return ""
}
