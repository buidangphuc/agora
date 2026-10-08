package handler

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-search/generated/platform/common/v1"
)

// Listing visibility policy (search-correctness-and-privacy D1/D2).
//
// The INDEX guarantees published-only by default (a status=published clause is
// added whenever the filter set has no status); this function is the HANDLER-side
// authorization that decides who may override it, and is the only place that
// knows the caller. It is shared by SearchListings, SaveSearch and RunSavedSearch
// so a saved filter cannot be used to see drafts the runner could not see directly.
const (
	filterStatus    = "status"
	filterSellerID  = "seller_id"
	statusPublished = "published"
	statusDraft     = "draft"

	// anonymousPrincipalID is the id the gateway forwards for an unauthenticated
	// caller (team-gateway PUBLIC principal); a shared constant, never a user.
	anonymousPrincipalID = "anonymous"
)

// effectiveFilters validates the caller's structured filters against the
// visibility policy and returns the map to hand to the index. It never mutates
// its input (the request's map is shared with the gRPC layer).
//
//   - no status key: returned as is; the index then applies status=published;
//   - status other than published/draft (deleted, rejected, any, "", ...): INVALID_ARGUMENT;
//   - status=published: allowed for everyone;
//   - status=draft: only a user principal whose filters["seller_id"] equals its id
//     (the explicit owner view). No principal or an anonymous/unspecified one is
//     UNAUTHENTICATED; a service principal, a non-owner, a foreign seller_id or a
//     missing seller_id is PERMISSION_DENIED.
func effectiveFilters(p *commonv1.Principal, filters map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(filters))
	for k, v := range filters {
		out[k] = v
	}
	st, has := out[filterStatus]
	if !has {
		return out, nil
	}
	switch st {
	case statusPublished:
		return out, nil
	case statusDraft:
		if err := requireDraftOwner(p, out[filterSellerID]); err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "filters.status must be %q or %q", statusPublished, statusDraft)
	}
}

func requireDraftOwner(p *commonv1.Principal, sellerID string) error {
	if p == nil {
		return status.Error(codes.Unauthenticated, "authentication required to view drafts")
	}
	switch p.GetType() {
	case commonv1.PrincipalType_PRINCIPAL_TYPE_USER:
	case commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE:
		return status.Error(codes.PermissionDenied, "drafts are visible to their owner only")
	default:
		return status.Error(codes.Unauthenticated, "authentication required to view drafts")
	}
	if p.GetId() == "" || p.GetId() == anonymousPrincipalID {
		return status.Error(codes.Unauthenticated, "authentication required to view drafts")
	}
	if sellerID == "" || sellerID != p.GetId() {
		return status.Error(codes.PermissionDenied, "drafts are visible to their owner only: filters.seller_id must be your own id")
	}
	return nil
}
