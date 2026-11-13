package edge

import (
	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
)

// STUB (t3-gateway-privacy-forwarder red): the implementer replaces this file.
type PrivacyForwarder struct {
	identityv1connect.UnimplementedPrivacyServiceHandler
}

func NewPrivacyForwarder(_ identityv1.PrivacyServiceClient, _ *Edge) *PrivacyForwarder {
	return &PrivacyForwarder{}
}
