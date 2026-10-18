package handler

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
	"github.com/buidangphuc/team-identity/internal/service"
)

const (
	scopeAdmin      = "admin"
	maxQueryBytes   = 64
	maxReasonBytes  = 64
	maxNoteChars    = 500
	auditDownMsg    = "audit unavailable"
	statusInputMsg  = "invalid input"
	statusPrecondMs = "status change not allowed"
)

// reasonCodeRE is lower_snake_case: starts with a letter, no leading/trailing/double underscore.
var reasonCodeRE = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// UserLister is the read surface for admin listing.
type UserLister interface {
	ListUsers(ctx context.Context, query, status, cursor string, limit int) ([]repository.User, string, error)
}

// AccountAdminHandler implements identityv1.AccountAdminServiceServer.
type AccountAdminHandler struct {
	identityv1.UnimplementedAccountAdminServiceServer
	svc    *service.AuthService
	users  UserLister
	logger *slog.Logger
}

func NewAccountAdminHandler(svc *service.AuthService, users UserLister, logger *slog.Logger) *AccountAdminHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AccountAdminHandler{svc: svc, users: users, logger: logger}
}

func requireAdmin(ctx context.Context) (*commonv1.Principal, error) {
	p, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if !hasScope(p.GetScopes(), scopeAdmin) {
		return nil, status.Error(codes.PermissionDenied, "admin scope required")
	}
	return p, nil
}

func repoStatus(s identityv1.UserStatus) string {
	switch s {
	case identityv1.UserStatus_USER_STATUS_ACTIVE:
		return repository.StatusActive
	case identityv1.UserStatus_USER_STATUS_RESTRICTED:
		return repository.StatusRestricted
	case identityv1.UserStatus_USER_STATUS_SUSPENDED:
		return repository.StatusSuspended
	case identityv1.UserStatus_USER_STATUS_BANNED:
		return repository.StatusBanned
	}
	return ""
}

func toAdminUser(u repository.User) *identityv1.AdminUser {
	st := u.Status
	if st == "" {
		st = repository.StatusActive
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
	case repository.StatusActive:
		return identityv1.UserStatus_USER_STATUS_ACTIVE
	case repository.StatusRestricted:
		return identityv1.UserStatus_USER_STATUS_RESTRICTED
	case repository.StatusSuspended:
		return identityv1.UserStatus_USER_STATUS_SUSPENDED
	case repository.StatusBanned:
		return identityv1.UserStatus_USER_STATUS_BANNED
	}
	return identityv1.UserStatus_USER_STATUS_UNSPECIFIED
}

func (h *AccountAdminHandler) ListUsers(ctx context.Context, req *identityv1.ListUsersRequest) (*identityv1.ListUsersResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if len(req.GetQuery()) > maxQueryBytes {
		return nil, status.Error(codes.InvalidArgument, "query too long")
	}
	users, next, err := h.users.ListUsers(ctx, req.GetQuery(), repoStatus(req.GetStatus()),
		req.GetPage().GetCursor(), int(req.GetPage().GetPageSize()))
	if err != nil {
		return nil, internalErr(h.logger, "list users", err)
	}
	out := make([]*identityv1.AdminUser, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUser(u))
	}
	return &identityv1.ListUsersResponse{Users: out, Page: &commonv1.PageResponse{NextCursor: next, Total: -1}}, nil
}

func (h *AccountAdminHandler) SetUserStatus(ctx context.Context, req *identityv1.SetUserStatusRequest) (*identityv1.SetUserStatusResponse, error) {
	p, err := requireAdmin(ctx)
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
	res, err := h.svc.SetUserStatus(ctx, p.GetId(), req.GetUserId(), repoStatus(req.GetStatus()), reason, req.GetNote(), until)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidInput):
			return nil, status.Error(codes.InvalidArgument, statusInputMsg)
		case errors.Is(err, repository.ErrNotFound):
			return nil, status.Error(codes.NotFound, "user not found")
		case errors.Is(err, service.ErrStatusPrecondition):
			return nil, status.Error(codes.FailedPrecondition, statusPrecondMs)
		case errors.Is(err, service.ErrAuditUnavailable):
			h.logger.WarnContext(ctx, "set user status: audit unavailable")
			return nil, status.Error(codes.Unavailable, auditDownMsg)
		default:
			return nil, internalErr(h.logger, "set user status", err)
		}
	}
	return &identityv1.SetUserStatusResponse{User: toAdminUser(res.User), SessionsRevoked: int32(res.SessionsRevoked)}, nil
}
