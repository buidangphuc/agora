package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	commonv1 "github.com/buidangphuc/team-audit/generated/platform/common/v1"
	"github.com/buidangphuc/team-audit/internal/audit"
)

const (
	scopeAdmin      = "admin"
	scopeAuditWrite = "audit.write"
)

type Handler struct {
	auditv1.UnimplementedAuditServiceServer
	svc    *audit.Service
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

func NewHandler(svc *audit.Service, logger *slog.Logger) (*Handler, error) {
	if svc == nil {
		return nil, errors.New("rpc: service is required")
	}
	if logger == nil {
		return nil, errors.New("rpc: logger is required")
	}
	errs, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: audit.ErrEmptyAction, Code: codes.InvalidArgument},
		grpcx.Rule{Target: audit.ErrInvalidTimeRange, Code: codes.InvalidArgument},
	)
	if err != nil {
		return nil, err
	}
	return &Handler{svc: svc, logger: logger, errs: errs}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	auditv1.RegisterAuditServiceServer(srv, h)
}

func (h *Handler) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	p, err := auth.RequireService(ctx, scopeAuditWrite)
	if err != nil {
		return nil, err
	}
	actorID := p.ID
	if req.GetActorId() != "" {
		actorID = req.GetActorId()
	}
	if _, err := h.svc.Write(ctx, actorID, req.GetAction(), req.GetTargetType(), req.GetTargetId(), req.GetMetadata()); err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, h.errs)
	}
	return &auditv1.WriteAuditEventResponse{}, nil
}

func (h *Handler) QueryAuditLog(ctx context.Context, req *auditv1.QueryAuditLogRequest) (*auditv1.QueryAuditLogResponse, error) {
	if _, err := auth.RequireScopes(ctx, scopeAdmin); err != nil {
		return nil, err
	}
	offset := decodeCursor(req.GetPage().GetCursor())
	var after, before time.Time
	if req.GetCreatedAfter() != nil {
		after = req.GetCreatedAfter().AsTime()
	}
	if req.GetCreatedBefore() != nil {
		before = req.GetCreatedBefore().AsTime()
	}
	events, total, err := h.svc.Query(ctx, audit.Filter{
		ActorID:       req.GetActorId(),
		TargetType:    req.GetTargetType(),
		TargetID:      req.GetTargetId(),
		Action:        req.GetAction(),
		CreatedAfter:  after,
		CreatedBefore: before,
		Limit:         int(req.GetPage().GetPageSize()),
		Offset:        offset,
	})
	if err != nil {
		return nil, grpcx.Status(ctx, h.logger, err, h.errs)
	}
	next := ""
	if int64(offset+len(events)) < total {
		next = strconv.Itoa(offset + len(events))
	}
	return &auditv1.QueryAuditLogResponse{
		Events: toProto(events),
		Page:   &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

func decodeCursor(cursor string) int {
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func toProto(events []audit.Event) []*auditv1.AuditEvent {
	out := make([]*auditv1.AuditEvent, 0, len(events))
	for _, e := range events {
		out = append(out, &auditv1.AuditEvent{
			Id:         e.ID,
			ActorId:    e.ActorID,
			Action:     e.Action,
			TargetType: e.TargetType,
			TargetId:   e.TargetID,
			Metadata:   e.Metadata,
			CreatedAt:  timestamppb.New(e.CreatedAt),
		})
	}
	return out
}
