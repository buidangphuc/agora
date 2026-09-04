// Package handler adapts the AuditService gRPC contract to the application
// service. It translates the request/response wire types, resolves pagination
// cursors, and maps validation errors to gRPC status codes.
package handler

import (
	"context"
	"errors"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	commonv1 "github.com/buidangphuc/team-audit/generated/platform/common/v1"
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
		return status.Errorf(codes.Internal, "%v", err)
	}
}

// WriteAuditEvent appends an immutable event. It is fire-and-forget: the event
// is persisted durably, but the response carries no payload (the caller does not
// wait on a receipt). actor_id may be empty for anonymous/system actors.
func (h *AuditHandler) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	if _, err := h.svc.Write(ctx, req.GetActorId(), req.GetAction(), req.GetTargetType(), req.GetTargetId(), req.GetMetadata()); err != nil {
		return nil, mapAuditErr(err)
	}
	return &auditv1.WriteAuditEventResponse{}, nil
}

// QueryAuditLog returns the audit trail filtered by actor and/or target type,
// newest first, paginated. The opaque page cursor encodes the row offset; the
// response's next_cursor is set only while further pages remain.
func (h *AuditHandler) QueryAuditLog(ctx context.Context, req *auditv1.QueryAuditLogRequest) (*auditv1.QueryAuditLogResponse, error) {
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
