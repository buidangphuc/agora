package rpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

// maxPublicProfileIDs bounds one GetPublicProfiles call.
const maxPublicProfileIDs = 100

// scopeIdentityRead is the scope a calling service must carry.
const scopeIdentityRead = "identity.read"

// ProfileHandler serves PublicProfileService: an internal, service-to-service
// lookup of public display names.
type ProfileHandler struct {
	identityv1.UnimplementedPublicProfileServiceServer

	users  store.ProfileReader
	logger *slog.Logger
}

// NewProfileHandler builds the handler over a profile reader; a nil logger means slog.Default().
func NewProfileHandler(users store.ProfileReader, logger *slog.Logger) *ProfileHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProfileHandler{users: users, logger: logger}
}

// GetPublicProfiles returns {user_id, display_name} for known ids. Only service
// principals holding identity.read may call it; a user principal (including one
// that claims the scope) gets PERMISSION_DENIED, so it cannot be used to
// enumerate other users. Display name is the username (no separate display-name
// field exists yet).
func (h *ProfileHandler) GetPublicProfiles(
	ctx context.Context,
	req *identityv1.GetPublicProfilesRequest,
) (*identityv1.GetPublicProfilesResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if principal.Type != principalTypeService {
		return nil, status.Error(codes.PermissionDenied, "service principal required")
	}
	if !principal.HasScope(scopeIdentityRead) {
		return nil, status.Error(codes.PermissionDenied, "missing scope "+scopeIdentityRead)
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
		h.logger.ErrorContext(ctx, "get public profiles", "err", err)
		return nil, status.Error(codes.Internal, "internal error")
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
