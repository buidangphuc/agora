package rpc

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	defaultLoginHistoryPageSize = 20
	maxLoginHistoryPageSize     = 100
)

type SessionHandler struct {
	identityv1.UnimplementedSessionServiceServer

	repo     store.SessionRepository
	logger   *slog.Logger
	tokenTTL time.Duration
}

func NewSessionHandler(repo store.SessionRepository, logger *slog.Logger) *SessionHandler {
	return &SessionHandler{repo: repo, logger: orDefault(logger)}
}

func (h *SessionHandler) WithTokenTTL(ttl time.Duration) *SessionHandler {
	h.tokenTTL = ttl
	return h
}

func (h *SessionHandler) ListSessions(ctx context.Context, _ *identityv1.ListSessionsRequest) (*identityv1.ListSessionsResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	items, err := h.repo.ListSessions(ctx, p.ID)
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	wire := make([]*identityv1.Session, 0, len(items))
	for _, s := range items {
		wire = append(wire, toWireSession(s))
	}
	return &identityv1.ListSessionsResponse{Sessions: wire}, nil
}

func (h *SessionHandler) RevokeSession(ctx context.Context, req *identityv1.RevokeSessionRequest) (*identityv1.RevokeSessionResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSessionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	var enqueue store.EnqueueFn
	if h.tokenTTL > 0 {
		requestID := reqctx.RequestID(ctx)
		principal := commonPrincipal(p)
		enqueue = func(revoked store.Session) (store.OutboxRow, error) {
			eventID := uuid.NewString()
			payload, err := events.BuildSessionRevokedEnvelope(eventID, revoked.ID, revoked.UserID,
				time.Now().Add(h.tokenTTL), principal, requestID)
			if err != nil {
				return store.OutboxRow{}, err
			}
			return store.OutboxRow{
				EventID:       eventID,
				AggregateType: "Session",
				AggregateID:   revoked.UserID,
				EventType:     events.SessionRevokedEventType,
				Payload:       payload,
				RequestID:     requestID,
			}, nil
		}
	}
	if err := h.repo.RevokeSession(ctx, req.GetSessionId(), p.ID, enqueue); err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	return &identityv1.RevokeSessionResponse{}, nil
}

func (h *SessionHandler) ListLoginHistory(ctx context.Context, req *identityv1.ListLoginHistoryRequest) (*identityv1.ListLoginHistoryResponse, error) {
	p, err := auth.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	limit := pagination.Clamp(int(req.GetPage().GetPageSize()), defaultLoginHistoryPageSize, maxLoginHistoryPageSize)
	offset := pageOffset(req.GetPage())

	evs, total, err := h.repo.ListLoginHistory(ctx, p.ID, limit, offset)
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	wire := make([]*identityv1.LoginEvent, 0, len(evs))
	for _, e := range evs {
		wire = append(wire, toWireLoginEvent(e))
	}
	var nextCursor string
	if int64(offset+len(evs)) < total {
		nextCursor = strconv.Itoa(offset + len(evs))
	}
	return &identityv1.ListLoginHistoryResponse{
		Events: wire,
		Page:   &commonv1.PageResponse{NextCursor: nextCursor, Total: total},
	}, nil
}

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

func toWireSession(s store.Session) *identityv1.Session {
	return &identityv1.Session{
		Id:        s.ID,
		Device:    s.Device,
		Ip:        s.IP,
		CreatedAt: timestamppb.New(s.CreatedAt),
		LastSeen:  timestamppb.New(s.LastSeen),
		Revoked:   s.Revoked,
	}
}

func toWireLoginEvent(e store.LoginEvent) *identityv1.LoginEvent {
	return &identityv1.LoginEvent{
		Id:        e.ID,
		Ip:        e.IP,
		UserAgent: e.UserAgent,
		Success:   e.Success,
		CreatedAt: timestamppb.New(e.CreatedAt),
	}
}
