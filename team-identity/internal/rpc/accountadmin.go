package rpc

import (
	"context"
	"log/slog"
	"math"
	"regexp"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	scopeAdmin     = "admin"
	maxQueryBytes  = 64
	maxReasonBytes = 64
	maxNoteChars   = 500
	statusInputMsg = "invalid input"
)

// reasonCodeRE is lower_snake_case: starts with a letter, no leading/trailing/double underscore.
var reasonCodeRE = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// UserLister is the read surface for admin listing.
type UserLister interface {
	ListUsers(ctx context.Context, query, status, cursor string, limit int) ([]store.User, string, error)
}

// AccountAdminHandler implements identityv1.AccountAdminServiceServer.
type AccountAdminHandler struct {
	identityv1.UnimplementedAccountAdminServiceServer
	svc    *identity.AuthService
	users  UserLister
	logger *slog.Logger
}

func NewAccountAdminHandler(svc *identity.AuthService, users UserLister, logger *slog.Logger) *AccountAdminHandler {
	return &AccountAdminHandler{svc: svc, users: users, logger: orDefault(logger)}
}

func repoStatus(s identityv1.UserStatus) string {
	switch s {
	case identityv1.UserStatus_USER_STATUS_ACTIVE:
		return store.StatusActive
	case identityv1.UserStatus_USER_STATUS_RESTRICTED:
		return store.StatusRestricted
	case identityv1.UserStatus_USER_STATUS_SUSPENDED:
		return store.StatusSuspended
	case identityv1.UserStatus_USER_STATUS_BANNED:
		return store.StatusBanned
	}
	return ""
}

func toAdminUser(u store.User) *identityv1.AdminUser {
	st := u.Status
	if st == "" {
		st = store.StatusActive
	}
	out := &identityv1.AdminUser{
		Id:           u.ID,
		Username:     u.Username,
		Roles:        u.Roles,
		Status:       protoUserStatus(st),
		StatusReason: u.StatusReason,
	}
	if !u.StatusUntil.IsZero() {
		out.StatusUntil = timestamppb.New(u.StatusUntil)
	}
	if !u.CreatedAt.IsZero() {
		out.CreatedAt = timestamppb.New(u.CreatedAt)
	}
	return out
}

func protoUserStatus(s string) identityv1.UserStatus {
	switch s {
	case store.StatusActive:
		return identityv1.UserStatus_USER_STATUS_ACTIVE
	case store.StatusRestricted:
		return identityv1.UserStatus_USER_STATUS_RESTRICTED
	case store.StatusSuspended:
		return identityv1.UserStatus_USER_STATUS_SUSPENDED
	case store.StatusBanned:
		return identityv1.UserStatus_USER_STATUS_BANNED
	}
	return identityv1.UserStatus_USER_STATUS_UNSPECIFIED
}

func (h *AccountAdminHandler) ListUsers(ctx context.Context, req *identityv1.ListUsersRequest) (*identityv1.ListUsersResponse, error) {
	if _, err := auth.RequireScopes(ctx, scopeAdmin); err != nil {
		return nil, err
	}
	if len(req.GetQuery()) > maxQueryBytes {
		return nil, status.Error(codes.InvalidArgument, "query too long")
	}
	users, next, err := h.users.ListUsers(ctx, req.GetQuery(), repoStatus(req.GetStatus()),
		req.GetPage().GetCursor(), int(req.GetPage().GetPageSize()))
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	out := make([]*identityv1.AdminUser, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUser(u))
	}
	return &identityv1.ListUsersResponse{Users: out, Page: &commonv1.PageResponse{NextCursor: next, Total: -1}}, nil
}

func (h *AccountAdminHandler) SetUserStatus(ctx context.Context, req *identityv1.SetUserStatusRequest) (*identityv1.SetUserStatusResponse, error) {
	p, err := auth.RequireScopes(ctx, scopeAdmin)
	if err != nil {
		return nil, err
	}
	reason := req.GetReasonCode()
	if len(reason) > maxReasonBytes || !reasonCodeRE.MatchString(reason) ||
		utf8.RuneCountInString(req.GetNote()) > maxNoteChars {
		return nil, status.Error(codes.InvalidArgument, statusInputMsg)
	}
	var until *time.Time
	if req.GetUntil() != nil {
		t := req.GetUntil().AsTime()
		until = &t
	}
	res, err := h.svc.SetUserStatus(ctx, p.ID, req.GetUserId(), repoStatus(req.GetStatus()), reason, req.GetNote(), until)
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.SetUserStatusResponse{User: toAdminUser(res.User), SessionsRevoked: int32(min(res.SessionsRevoked, math.MaxInt32))}, nil
}
