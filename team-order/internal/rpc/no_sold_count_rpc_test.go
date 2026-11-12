package rpc_test

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
)

func TestNoRPCAcceptsSoldCount(t *testing.T) {
	services := orderv1.File_platform_order_v1_order_proto.Services()
	methods := 0
	for i := 0; i < services.Len(); i++ {
		svc := services.Get(i)
		ms := svc.Methods()
		for j := 0; j < ms.Len(); j++ {
			m := ms.Get(j)
			methods++
			if found := soldCountReachable(m.Input(), map[protoreflect.FullName]bool{}); len(found) > 0 {
				t.Errorf("%s.%s accepts %v", svc.FullName(), m.Name(), found)
			}
		}
	}
	if methods < 10 {
		t.Fatalf("expected the team-order RPC methods, found %d", methods)
	}
}
