package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
)

const invalidRefreshMessage = "invalid refresh token"

var errTable = mustTable(
	grpcx.Rule{Target: identity.ErrAccountLocked, Code: codes.PermissionDenied, Message: "tài khoản bị khóa"},
	grpcx.Rule{Target: identity.ErrInvalidCredentials, Code: codes.Unauthenticated, Message: "invalid credentials"},
	grpcx.Rule{Target: store.ErrConflict, Code: codes.AlreadyExists, Message: "username already exists"},
	grpcx.Rule{Target: identity.ErrInvalidInput, Code: codes.InvalidArgument, Message: "invalid input"},
	grpcx.Rule{Target: identity.ErrInvalidToken, Code: codes.InvalidArgument, Message: "invalid reset token"},
	grpcx.Rule{Target: identity.ErrTokenExpired, Code: codes.InvalidArgument, Message: "reset token expired"},
	grpcx.Rule{Target: identity.ErrTokenAlreadyUsed, Code: codes.InvalidArgument, Message: "reset token already used"},
	grpcx.Rule{Target: store.ErrNotFound, Code: codes.NotFound, Message: "user not found"},
	grpcx.Rule{Target: store.ErrTokenNotFound, Code: codes.NotFound, Message: "reset token not found"},
	grpcx.Rule{Target: store.ErrAddressNotFound, Code: codes.NotFound, Message: "address not found"},
	grpcx.Rule{Target: store.ErrSessionNotFound, Code: codes.NotFound, Message: "session not found"},
	grpcx.Rule{Target: identity.ErrStatusPrecondition, Code: codes.FailedPrecondition, Message: "status change not allowed"},
	grpcx.Rule{Target: identity.ErrInvalidRefreshToken, Code: codes.Unauthenticated, Message: invalidRefreshMessage},
	grpcx.Rule{Target: identity.ErrRefreshUnavailable, Code: codes.Unavailable, Message: "service unavailable"},
)

func mustTable(rules ...grpcx.Rule) grpcx.ErrorTable {
	t, err := grpcx.NewErrorTable(rules...)
	if err != nil {
		panic(err)
	}
	return t
}

func statusErr(ctx context.Context, logger *slog.Logger, err error) error {
	return grpcx.Status(ctx, logger, err, errTable)
}

func orDefault(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l
}
