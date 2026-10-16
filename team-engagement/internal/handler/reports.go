package handler

import (
	"context"
	"errors"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

const (
	defaultReportPageSize = 20
	maxReportPageSize     = 100
)

// WithReportService wires the listing-report service. Without it the report
// RPCs return UNIMPLEMENTED.
func WithReportService(s *service.ReportService) Option {
	return func(h *EngagementHandler) { h.reportSvc = s }
}

// CreateReport files a report against a listing as the verified caller.
func (h *EngagementHandler) CreateReport(
	ctx context.Context, req *engagementv1.CreateReportRequest,
) (*engagementv1.CreateReportResponse, error) {
	if _, err := interceptor.RequireAuthenticated(ctx); err != nil {
		return nil, err
	}
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if h.reportSvc == nil {
		return nil, status.Error(codes.Unimplemented, "report service unavailable")
	}
	if req.GetTargetType() != engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING {
		return nil, status.Error(codes.InvalidArgument, "target_type must be LISTING")
	}
	rep, err := h.reportSvc.CreateReport(ctx, userID(ctx), req.GetTargetId(), req.GetReasonCode(), req.GetNote())
	switch {
	case err == nil:
		return &engagementv1.CreateReportResponse{Report: toWireReport(rep)}, nil
	case errors.Is(err, service.ErrInvalidReportTarget), errors.Is(err, service.ErrInvalidReportReason),
		errors.Is(err, service.ErrReportNoteTooLong):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, service.ErrOwnListing):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, service.ErrReportCapReached):
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	return nil, internalError(ctx, "create report", err)
}

// ListReports lists reports oldest first (admin only).
func (h *EngagementHandler) ListReports(
	ctx context.Context, req *engagementv1.ListReportsRequest,
) (*engagementv1.ListReportsResponse, error) {
	if err := interceptor.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if h.reportSvc == nil {
		return nil, status.Error(codes.Unimplemented, "report service unavailable")
	}
	var st repository.ReportStatus
	if req.GetStatus() != engagementv1.ReportStatus_REPORT_STATUS_UNSPECIFIED {
		st = toDomainReportStatus(req.GetStatus())
		if st == "" {
			return nil, status.Error(codes.InvalidArgument, "invalid status")
		}
	}
	limit, offset := defaultReportPageSize, 0
	if p := req.GetPage(); p != nil {
		if n := int(p.GetPageSize()); n > 0 {
			limit = min(n, maxReportPageSize)
		}
		if c := p.GetCursor(); c != "" {
			o, err := strconv.Atoi(c)
			if err != nil || o < 0 {
				return nil, status.Error(codes.InvalidArgument, "invalid cursor")
			}
			offset = o
		}
	}
	rows, total, err := h.reportSvc.ListReports(ctx, st, req.GetTargetId(), offset, limit)
	if err != nil {
		return nil, internalError(ctx, "list reports", err)
	}
	out := make([]*engagementv1.Report, 0, len(rows))
	for _, r := range rows {
		out = append(out, toWireReport(r))
	}
	page := &commonv1.PageResponse{Total: total}
	if int64(offset+len(rows)) < total {
		page.NextCursor = strconv.Itoa(offset + len(rows))
	}
	return &engagementv1.ListReportsResponse{Reports: out, Page: page}, nil
}

// ResolveReport closes an open report as ACTIONED or DISMISSED (admin only).
func (h *EngagementHandler) ResolveReport(
	ctx context.Context, req *engagementv1.ResolveReportRequest,
) (*engagementv1.ResolveReportResponse, error) {
	if err := interceptor.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if h.reportSvc == nil {
		return nil, status.Error(codes.Unimplemented, "report service unavailable")
	}
	rep, err := h.reportSvc.ResolveReport(ctx, userID(ctx), req.GetReportId(),
		toDomainReportStatus(req.GetStatus()), req.GetResolutionNote())
	switch {
	case err == nil:
		return &engagementv1.ResolveReportResponse{Report: toWireReport(rep)}, nil
	case errors.Is(err, service.ErrEmptyReportID), errors.Is(err, service.ErrInvalidReportStatus),
		errors.Is(err, service.ErrResolutionNoteRequired), errors.Is(err, service.ErrResolutionNoteTooLong):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, repository.ErrReportNotFound):
		return nil, status.Error(codes.NotFound, "report not found")
	case errors.Is(err, service.ErrReportClosed):
		return nil, status.Error(codes.FailedPrecondition, "report is already closed")
	case errors.Is(err, service.ErrAuditUnavailable):
		return nil, status.Error(codes.Unavailable, "audit unavailable")
	}
	return nil, internalError(ctx, "resolve report", err)
}

func toDomainReportStatus(s engagementv1.ReportStatus) repository.ReportStatus {
	switch s {
	case engagementv1.ReportStatus_REPORT_STATUS_OPEN:
		return repository.ReportStatusOpen
	case engagementv1.ReportStatus_REPORT_STATUS_ACTIONED:
		return repository.ReportStatusActioned
	case engagementv1.ReportStatus_REPORT_STATUS_DISMISSED:
		return repository.ReportStatusDismissed
	}
	return ""
}

func toWireReportStatus(s repository.ReportStatus) engagementv1.ReportStatus {
	switch s {
	case repository.ReportStatusOpen:
		return engagementv1.ReportStatus_REPORT_STATUS_OPEN
	case repository.ReportStatusActioned:
		return engagementv1.ReportStatus_REPORT_STATUS_ACTIONED
	case repository.ReportStatusDismissed:
		return engagementv1.ReportStatus_REPORT_STATUS_DISMISSED
	}
	return engagementv1.ReportStatus_REPORT_STATUS_UNSPECIFIED
}

func toWireReport(r repository.Report) *engagementv1.Report {
	out := &engagementv1.Report{
		Id:             r.ID,
		TargetType:     engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING,
		TargetId:       r.TargetID,
		ReasonCode:     r.ReasonCode,
		Note:           r.Note,
		ReporterId:     r.ReporterID,
		Status:         toWireReportStatus(r.Status),
		CreatedAt:      timestamppb.New(r.CreatedAt),
		ResolutionNote: r.ResolutionNote,
	}
	if r.ResolvedAt != nil {
		out.ResolvedAt = timestamppb.New(*r.ResolvedAt)
	}
	return out
}
