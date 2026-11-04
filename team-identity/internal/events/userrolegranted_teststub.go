package events

import (
	"errors"
	"time"

	commonv1 "github.com/buidangphuc/team-identity/generated/platform/common/v1"
)

const UserRoleGrantedEventType = "platform.identity.v1.UserRoleGranted"

func BuildUserRoleGranted(
	eventID, userID, role string,
	grantedAt time.Time,
	principal *commonv1.Principal,
	requestID string,
) ([]byte, error) {
	return nil, errors.New("TEST-STUB: BuildUserRoleGranted not implemented")
}
