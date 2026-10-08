package edge

import (
	"errors"
	"slices"

	"connectrpc.com/connect"

	"github.com/buidangphuc/team-gateway/generated/platform/audit/v1/auditv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/engagement/v1/engagementv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/verification/v1/verificationv1connect"
)

// adminProcedures is the edge procedure policy: exactly these RPCs need the
// `admin` scope before they are forwarded (defence in depth; the owning service
// stays the authoritative check). Adding or removing an entry is a spec change
// (openspec service-authz-hardening, edge-route-policy); a test pins the set.
var adminProcedures = map[string]string{
	verificationv1connect.VerificationServiceReviewKycProcedure:  adminScope,
	engagementv1connect.EngagementServiceResolveDisputeProcedure: adminScope,
	auditv1connect.AuditServiceQueryAuditLogProcedure:            adminScope,
	orderv1connect.OrderServiceForceFailSagaProcedure:            adminScope,
}

// requireProcedureScope enforces adminProcedures for a resolved principal:
// anonymous -> unauthenticated, verified without the scope -> permission_denied,
// nil otherwise (including for every procedure not in the map).
func requireProcedureScope(procedure string, p resolvedPrincipal) error {
	scope, gated := adminProcedures[procedure]
	if !gated {
		return nil
	}
	if p.ptype == "anonymous" || p.id == "" || p.id == "anonymous" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !slices.Contains(p.scopes, scope) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("insufficient scope"))
	}
	return nil
}
