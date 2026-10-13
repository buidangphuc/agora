// Package rpc is the transport layer: it decodes requests, enforces
// authentication, calls the domain and maps domain errors to gRPC status. It
// holds no business logic.
//
// The service is described by a hand-built grpc.ServiceDesc over well-known
// protobuf messages so the example needs no code generation. A real service
// uses the generated registration function instead.
package rpc

import (
	"context"
	"fmt"
	"log/slog"

	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/examples/hello/internal/greeter"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
)

// Handler implements the hello.v1.Hello methods.
type Handler struct {
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

// NewHandler returns a Handler that maps domain errors with its error table.
func NewHandler(logger *slog.Logger) *Handler {
	return &Handler{
		logger: logger,
		errs:   grpcx.ErrorTable{greeter.ErrEmptyName: grpcx.Code(codes.InvalidArgument)},
	}
}

// Greet greets the caller-supplied name. It requires an authenticated principal.
func (h *Handler) Greet(ctx context.Context, in *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, h.errs)
	}
	msg, err := greeter.Greet(in.GetValue())
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, h.errs)
	}
	return wrapperspb.String(fmt.Sprintf("%s (caller %s)", msg, p.ID)), nil
}

// Boom always panics. It exists to show that the server survives a panic.
func (h *Handler) Boom(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	panic("boom")
}

// Register adds the Hello service to s.
func Register(s grpc.ServiceRegistrar, h *Handler) {
	s.RegisterService(&serviceDesc, h)
}

type helloServer interface {
	Greet(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
	Boom(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}

var serviceDesc = grpc.ServiceDesc{
	ServiceName: "hello.v1.Hello",
	HandlerType: (*helloServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Greet", Handler: greetHandler},
		{MethodName: "Boom", Handler: boomHandler},
	},
}

// greetHandler has the grpc.MethodHandler signature, which puts ctx second.
//
//nolint:revive // signature fixed by grpc.MethodHandler
func greetHandler(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
	in := new(wrapperspb.StringValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	if ic == nil {
		return srv.(helloServer).Greet(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/hello.v1.Hello/Greet"}
	return ic(ctx, in, info, func(ctx context.Context, req any) (any, error) {
		return srv.(helloServer).Greet(ctx, req.(*wrapperspb.StringValue))
	})
}

// boomHandler has the grpc.MethodHandler signature, which puts ctx second.
//
//nolint:revive // signature fixed by grpc.MethodHandler
func boomHandler(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
	in := new(emptypb.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if ic == nil {
		return srv.(helloServer).Boom(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/hello.v1.Hello/Boom"}
	return ic(ctx, in, info, func(ctx context.Context, req any) (any, error) {
		return srv.(helloServer).Boom(ctx, req.(*emptypb.Empty))
	})
}
