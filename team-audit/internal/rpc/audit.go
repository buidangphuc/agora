// Package rpc is the transport layer: it decodes requests, authorizes the
// caller, calls the audit domain, resolves pagination cursors and maps errors
// to gRPC status. It holds no business logic.
package rpc

import (
	"context"
	"log/slog"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"

	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	commonv1 "github.com/buidangphuc/team-audit/generated/platform/common/v1"
	"github.com/buidangphuc/team-audit/internal/audit"
)

// Handler implements platform.audit.v1.AuditService.
type Handler struct {
	auditv1.UnimplementedAuditServiceServer
	svc    *audit.Service
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

// NewHandler returns a Handler that maps domain errors with its error table.
func NewHandler(svc *audit.Service, logger *slog.Logger) *Handler {
	return &Handler{
		svc:    svc,
		logger: logger,
		errs:   grpcx.ErrorTable{audit.ErrEmptyAction: codes.InvalidArgument},
	}
}

// Register adds the audit service to s.
func Register(s grpc.ServiceRegistrar, h *Handler) {
	auditv1.RegisterAuditServiceServer(s, h)
}

// mapErr turns a domain or store error into the right gRPC status; unknown
// errors are logged and become Internal with a fixed message, so storage error
// text never reaches the caller.
func (h *Handler) mapErr(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

// WriteAuditEvent appends an immutable event. It is fire-and-forget: the event
// is persisted durably, but the response carries no payload. Service-only: the
// caller must be a service principal holding audit.write; users (admins
// included), anonymous callers and scopeless services are refused so the trail
// cannot be forged. The service may supply actor_id to record on behalf of a
// user; otherwise its own id is stored.
func (h *Handler) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	p, err := requireService(ctx, scopeAuditWrite)
	if err != nil {
		return nil, err
	}
	actorID := p.ID
	if req.GetActorId() != "" {
		actorID = req.GetActorId()
	}
	if _, err := h.svc.Write(ctx, actorID, req.GetAction(), req.GetTargetType(), req.GetTargetId(), req.GetMetadata()); err != nil {
		return nil, h.mapErr(ctx, err)
	}
	return &auditv1.WriteAuditEventResponse{}, nil
}

// QueryAuditLog returns the audit trail filtered by actor and/or target type,
// newest first, paginated. The opaque page cursor encodes the row offset; the
// response's next_cursor is set only while further pages remain. Admin only.
func (h *Handler) QueryAuditLog(ctx context.Context, req *auditv1.QueryAuditLogRequest) (*auditv1.QueryAuditLogResponse, error) {
	if _, err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	pageSize := int(req.GetPage().GetPageSize())
	offset := decodeCursor(req.GetPage().GetCursor())

	events, total, err := h.svc.Query(ctx, req.GetActorId(), req.GetTargetType(), pageSize, offset)
	if err != nil {
		return nil, h.mapErr(ctx, err)
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
