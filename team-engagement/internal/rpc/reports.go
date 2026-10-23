package rpc

import (
	"context"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/store"
)

const (
	defaultReportPageSize = 20
	maxReportPageSize     = 100
)

func (h *Handler) CreateReport(
	ctx context.Context, req *engagementv1.CreateReportRequest,
) (*engagementv1.CreateReportResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetTargetType() != engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING {
		return nil, status.Error(codes.InvalidArgument, "target_type must be LISTING")
	}
	rep, err := h.reportSvc.CreateReport(ctx, userID(ctx), req.GetTargetId(), req.GetReasonCode(), req.GetNote())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.CreateReportResponse{Report: toWireReport(rep)}, nil
}

func (h *Handler) ListReports(
	ctx context.Context, req *engagementv1.ListReportsRequest,
) (*engagementv1.ListReportsResponse, error) {
	if err := requireScopes(ctx, "admin"); err != nil {
		return nil, err
	}
	var st store.ReportStatus
	if req.GetStatus() != engagementv1.ReportStatus_REPORT_STATUS_UNSPECIFIED {
		st = toDomainReportStatus(req.GetStatus())
		if st == "" {
			return nil, status.Error(codes.InvalidArgument, "invalid status")
		}
	}
	limit, offset := pagination.Clamp(int(req.GetPage().GetPageSize()), defaultReportPageSize, maxReportPageSize), 0
	if p := req.GetPage(); p != nil {
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
		return nil, h.fail(ctx, err)
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

func (h *Handler) ResolveReport(
	ctx context.Context, req *engagementv1.ResolveReportRequest,
) (*engagementv1.ResolveReportResponse, error) {
	if err := requireScopes(ctx, "admin"); err != nil {
		return nil, err
	}
	rep, err := h.reportSvc.ResolveReport(ctx, userID(ctx), req.GetReportId(),
		toDomainReportStatus(req.GetStatus()), req.GetResolutionNote())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.ResolveReportResponse{Report: toWireReport(rep)}, nil
}

func toDomainReportStatus(s engagementv1.ReportStatus) store.ReportStatus {
	switch s {
	case engagementv1.ReportStatus_REPORT_STATUS_OPEN:
		return store.ReportStatusOpen
	case engagementv1.ReportStatus_REPORT_STATUS_ACTIONED:
		return store.ReportStatusActioned
	case engagementv1.ReportStatus_REPORT_STATUS_DISMISSED:
		return store.ReportStatusDismissed
	}
	return ""
}

func toWireReportStatus(s store.ReportStatus) engagementv1.ReportStatus {
	switch s {
	case store.ReportStatusOpen:
		return engagementv1.ReportStatus_REPORT_STATUS_OPEN
	case store.ReportStatusActioned:
		return engagementv1.ReportStatus_REPORT_STATUS_ACTIONED
	case store.ReportStatusDismissed:
		return engagementv1.ReportStatus_REPORT_STATUS_DISMISSED
	}
	return engagementv1.ReportStatus_REPORT_STATUS_UNSPECIFIED
}

func toWireReport(r store.Report) *engagementv1.Report {
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
