package rpc

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/team-search/internal/index"
)

const (
	filterStatus    = "status"
	filterSellerID  = "seller_id"
	filterInStock   = "in_stock"
	statusPublished = "published"
	statusDraft     = "draft"
)

func effectiveFilters(p auth.Principal, filters map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(filters))
	for k, v := range filters {
		out[k] = v
	}
	if v, has := out[filterInStock]; has && v != "true" {
		return nil, status.Errorf(codes.InvalidArgument, "filters.in_stock must be %q", "true")
	}
	for k, v := range out {
		_, _, ok, err := index.ParseAttrFilterKey(k)
		if !ok {
			continue
		}
		if err == nil {
			_, err = index.ParseAttrFilterValues(k, v)
		}
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
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

func requireDraftOwner(p auth.Principal, sellerID string) error {
	if p.Anonymous() {
		return status.Error(codes.Unauthenticated, "authentication required to view drafts")
	}
	if p.Type == auth.TypeService {
		return status.Error(codes.PermissionDenied, "drafts are visible to their owner only")
	}
	if p.Type != auth.TypeUser {
		return status.Error(codes.Unauthenticated, "authentication required to view drafts")
	}
	if sellerID == "" || sellerID != p.ID {
		return status.Error(codes.PermissionDenied, "drafts are visible to their owner only: filters.seller_id must be your own id")
	}
	return nil
}
