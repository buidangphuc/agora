package x_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc"
)

type fakeOrderClient struct {
	orderv1.OrderServiceClient
}

type stubPay struct{}

func (s *stubPay) Charge(ctx context.Context, in *Req, opts ...grpc.CallOption) (*Resp, error) { return nil, nil }

func TestX(t *testing.T) { _ = gomock.NewController(t) }
