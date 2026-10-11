// Package handler adapts the AuditService gRPC contract to the application
// service. It translates the request/response wire types, resolves pagination
// cursors, and maps validation errors to gRPC status codes.
package handler

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	commonv1 "github.com/buidangphuc/team-audit/generated/platform/common/v1"
	"github.com/buidangphuc/team-audit/internal/interceptor"
	"github.com/buidangphuc/team-audit/internal/service"
)

type AuditHandler struct {
	auditv1.UnimplementedAuditServiceServer
	svc *service.AuditService
}

func NewAuditHandler(svc *service.AuditService) *AuditHandler {
	return &AuditHandler{svc: svc}
}

// mapAuditErr turns a service validation error into the right gRPC status.
func mapAuditErr(err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyAction):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		// Log the cause server-side; never leak storage error text to the caller.
		slog.Error("audit: internal error", "err", err)
		return status.Error(codes.Internal, "internal error")
	}
}

// WriteAuditEvent appends an immutable event. It is fire-and-forget: the event
// is persisted durably, but the response carries no payload. Service-only: the
// caller must be a service principal holding audit.write; users (admins
// included), anonymous callers and scopeless services are refused so the trail
// cannot be forged. The service may supply actor_id to record on behalf of a
// user; otherwise its own id is stored.
func (h *AuditHandler) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	p, err := interceptor.RequireService(ctx, interceptor.ScopeAuditWrite)
	if err != nil {
		return nil, err
	}
	actorID := p.ID
	if req.GetActorId() != "" {
		actorID = req.GetActorId()
	}
	if _, err := h.svc.Write(ctx, actorID, req.GetAction(), req.GetTargetType(), req.GetTargetId(), req.GetMetadata()); err != nil {
		return nil, mapAuditErr(err)
	}
	return &auditv1.WriteAuditEventResponse{}, nil
}

// QueryAuditLog returns the audit trail filtered by actor and/or target type,
// newest first, paginated. The opaque page cursor encodes the row offset; the
// response's next_cursor is set only while further pages remain. Admin only.
func (h *AuditHandler) QueryAuditLog(ctx context.Context, req *auditv1.QueryAuditLogRequest) (*auditv1.QueryAuditLogResponse, error) {
	if _, err := interceptor.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	pageSize := int(req.GetPage().GetPageSize())
	offset := decodeCursor(req.GetPage().GetCursor())

	events, total, err := h.svc.Query(ctx, req.GetActorId(), req.GetTargetType(), pageSize, offset)
	if err != nil {
		return nil, mapAuditErr(err)
	}

	next := ""
	if int64(offset+len(events)) < total {
		next = encodeCursor(offset + len(events))
	}

	return &auditv1.QueryAuditLogResponse{
		Events: events,
		Page: &commonv1.PageResponse{
			NextCursor: next,
			Total:      total,
		},
	}, nil
}

// decodeCursor reads the opaque offset cursor; an empty or malformed cursor
// starts from the first page (offset 0).
func decodeCursor(cursor string) int {
	if cursor == "" {
		return 0
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func encodeCursor(offset int) string { return strconv.Itoa(offset) }
