package handler

import "github.com/buidangphuc/team-engagement/internal/service"

// TEST-STUB (task 4): compile-only. The implementer replaces this with the real
// option in reports.go and deletes this file.
func WithReportService(_ *service.ReportService) Option { return func(*EngagementHandler) {} }
