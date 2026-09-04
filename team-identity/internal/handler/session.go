package handler

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
)

const (
	defaultLoginHistoryPageSize = 20
	maxLoginHistoryPageSize     = 100
)

// SessionHandler serves the SessionService: a caller's own active sessions and
// login history. Every RPC requires a valid Principal and is self-scoped to it,
// mirroring AddressService.
type SessionHandler struct {
	identityv1.UnimplementedSessionServiceServer

	repo   repository.SessionRepository
	logger *slog.Logger
}

func NewSessionHandler(repo repository.SessionRepository, logger *slog.Logger) *SessionHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionHandler{repo: repo, logger: logger}
}

func (h *SessionHandler) ListSessions(
	ctx context.Context,
	_ *identityv1.ListSessionsRequest,
) (*identityv1.ListSessionsResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.repo.ListSessions(ctx, principal.GetId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list sessions: %v", err)
	}
	wire := make([]*identityv1.Session, 0, len(items))
	for _, s := range items {
		wire = append(wire, toWireSession(s))
	}
	return &identityv1.ListSessionsResponse{Sessions: wire}, nil
}

func (h *SessionHandler) RevokeSession(
	ctx context.Context,
	req *identityv1.RevokeSessionRequest,
) (*identityv1.RevokeSessionResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSessionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if err := h.repo.RevokeSession(ctx, req.GetSessionId(), principal.GetId()); err != nil {
		if errors.Is(err, repository.ErrSessionNotFound) {
			return nil, status.Error(codes.NotFound, "session not found")
		}
		return nil, status.Errorf(codes.Internal, "revoke session: %v", err)
	}
	return &identityv1.RevokeSessionResponse{}, nil
}

func (h *SessionHandler) ListLoginHistory(
	ctx context.Context,
	req *identityv1.ListLoginHistoryRequest,
) (*identityv1.ListLoginHistoryResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	limit := pageSize(req.GetPage())
	offset := pageOffset(req.GetPage())

	events, total, err := h.repo.ListLoginHistory(ctx, principal.GetId(), limit, offset)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list login history: %v", err)
	}

	wire := make([]*identityv1.LoginEvent, 0, len(events))
	for _, e := range events {
		wire = append(wire, toWireLoginEvent(e))
	}

	var nextCursor string
	if int64(offset+len(events)) < total {
		nextCursor = strconv.Itoa(offset + len(events))
	}

	return &identityv1.ListLoginHistoryResponse{
		Events: wire,
		Page: &commonv1.PageResponse{
			NextCursor: nextCursor,
			Total:      total,
		},
	}, nil
}

// pageSize clamps the requested page size to a sane server default/maximum.
func pageSize(p *commonv1.PageRequest) int {
	size := int(p.GetPageSize())
	if size <= 0 {
		return defaultLoginHistoryPageSize
	}
	if size > maxLoginHistoryPageSize {
		return maxLoginHistoryPageSize
	}
	return size
}

// pageOffset decodes the opaque cursor as a numeric offset; anything invalid or
// empty starts from the beginning (no panic on garbage input).
func pageOffset(p *commonv1.PageRequest) int {
	cur := p.GetCursor()
	if cur == "" {
		return 0
	}
	off, err := strconv.Atoi(cur)
	if err != nil || off < 0 {
		return 0
	}
	return off
}

func toWireSession(s repository.Session) *identityv1.Session {
	return &identityv1.Session{
		Id:        s.ID,
		Device:    s.Device,
		Ip:        s.IP,
		CreatedAt: timestamppb.New(s.CreatedAt),
		LastSeen:  timestamppb.New(s.LastSeen),
		Revoked:   s.Revoked,
	}
}

func toWireLoginEvent(e repository.LoginEvent) *identityv1.LoginEvent {
	return &identityv1.LoginEvent{
		Id:        e.ID,
		Ip:        e.IP,
		UserAgent: e.UserAgent,
		Success:   e.Success,
		CreatedAt: timestamppb.New(e.CreatedAt),
	}
}
