package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

const (
	maxPublicProfileIDs = 100
	scopeIdentityRead   = "identity.read"
)

type ProfileHandler struct {
	identityv1.UnimplementedPublicProfileServiceServer

	users  store.ProfileReader
	logger *slog.Logger
}

func NewProfileHandler(users store.ProfileReader, logger *slog.Logger) *ProfileHandler {
	return &ProfileHandler{users: users, logger: orDefault(logger)}
}

func (h *ProfileHandler) GetPublicProfiles(ctx context.Context, req *identityv1.GetPublicProfilesRequest) (*identityv1.GetPublicProfilesResponse, error) {
	if _, err := auth.RequireService(ctx, scopeIdentityRead); err != nil {
		return nil, err
	}
	ids := dedupeNonEmpty(req.GetUserIds())
	if len(ids) > maxPublicProfileIDs {
		return nil, status.Errorf(codes.InvalidArgument, "too many user_ids (max %d)", maxPublicProfileIDs)
	}
	if len(ids) == 0 {
		return &identityv1.GetPublicProfilesResponse{}, nil
	}
	users, err := h.users.GetByIDs(ctx, ids)
	if err != nil {
		return nil, statusErr(ctx, h.logger, err)
	}
	out := make([]*identityv1.PublicProfile, 0, len(users))
	for _, u := range users {
		out = append(out, &identityv1.PublicProfile{UserId: u.ID, DisplayName: u.Username})
	}
	return &identityv1.GetPublicProfilesResponse{Profiles: out}, nil
}

func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
