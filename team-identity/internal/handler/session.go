package handler

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/events"
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

	repo     repository.SessionRepository
	logger   *slog.Logger
	tokenTTL time.Duration // 0: revokes record no outbox event
}

func NewSessionHandler(repo repository.SessionRepository, logger *slog.Logger) *SessionHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SessionHandler{repo: repo, logger: logger}
}

// WithTokenTTL makes RevokeSession publish a SessionRevoked event (in the same
// transaction as the revoke) whose expiry is the session's creation time plus ttl —
// the latest instant a token bound to that session can still be valid.
func (h *SessionHandler) WithTokenTTL(ttl time.Duration) *SessionHandler {
	h.tokenTTL = ttl
	return h
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
		return nil, internalErr(h.logger, "list sessions", err)
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
	var enqueue repository.EnqueueFn
	if h.tokenTTL > 0 {
		requestID := requestIDFrom(ctx)
		enqueue = func(revoked repository.Session) (repository.OutboxRow, error) {
			eventID := uuid.NewString()
			payload, err := events.BuildSessionRevokedEnvelope(eventID, revoked.ID, revoked.UserID,
				revoked.CreatedAt.Add(h.tokenTTL), principal, requestID)
			if err != nil {
				return repository.OutboxRow{}, err
			}
			return repository.OutboxRow{
				EventID:       eventID,
				AggregateType: "Session",
				AggregateID:   revoked.UserID,
				EventType:     events.SessionRevokedEventType,
				Payload:       payload,
				RequestID:     requestID,
			}, nil
		}
	}
	if err := h.repo.RevokeSession(ctx, req.GetSessionId(), principal.GetId(), enqueue); err != nil {
		if errors.Is(err, repository.ErrSessionNotFound) {
			return nil, status.Error(codes.NotFound, "session not found")
		}
		return nil, internalErr(h.logger, "revoke session", err)
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
		return nil, internalErr(h.logger, "list login history", err)
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

// requestIDFrom reads the gateway-forwarded x-request-id (trace continuity only).
func requestIDFrom(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-request-id"); len(v) > 0 {
			return v[0]
		}
	}
	return ""
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
