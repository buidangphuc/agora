package handler

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/interceptor"
	"github.com/buidangphuc/team-identity/internal/repository"
)

// maxPublicProfileIDs bounds one GetPublicProfiles call.
const maxPublicProfileIDs = 100

// scopeIdentityRead is the scope a calling service must carry.
const scopeIdentityRead = "identity.read"

// ProfileHandler serves PublicProfileService: an internal, service-to-service
// lookup of public display names.
type ProfileHandler struct {
	identityv1.UnimplementedPublicProfileServiceServer

	users  repository.ProfileReader
	logger *slog.Logger
}

func NewProfileHandler(users repository.ProfileReader, logger *slog.Logger) *ProfileHandler {
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
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if principal.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE {
		return nil, status.Error(codes.PermissionDenied, "service principal required")
	}
	if !hasScope(principal.GetScopes(), scopeIdentityRead) {
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
		h.logger.ErrorContext(ctx, "get public profiles", slog.Any("err", err))
		return nil, status.Error(codes.Internal, "internal error")
	}
	out := make([]*identityv1.PublicProfile, 0, len(users))
	for _, u := range users {
		out = append(out, &identityv1.PublicProfile{UserId: u.ID, DisplayName: u.Username})
	}
	return &identityv1.GetPublicProfilesResponse{Profiles: out}, nil
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
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
