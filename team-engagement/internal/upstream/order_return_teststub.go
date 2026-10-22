package upstream

import (
	"context"
	"errors"

	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
)

// TEST-STUB: compile-only; the implementation in order.go replaces this file.
var ErrReturnNotFound = errors.New("return not found")

func (c *OrderClient) GetReturn(_ context.Context, _ string) (string, orderv1.ReturnStatus, bool, error) {
	return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, nil
}
