package rpc

// STUB (test-author, task t2-obligations-lifecycle-rpcs): the implementer folds this into privacy.go
// and implements the three PrivacyService deletion handlers.

import "github.com/buidangphuc/team-identity/internal/identity"

func (h *PrivacyHandler) WithAccountDeletion(svc *identity.AccountDeletionService) *PrivacyHandler {
	return h
}
