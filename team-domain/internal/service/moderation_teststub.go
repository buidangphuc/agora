package service

// TEST-STUB (task 8): compile-only placeholders so the red tests build.
// The implementer deletes this file and writes internal/service/moderation.go.

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-domain/internal/repository"
)

type ModerationDecision string

const (
	DecisionApprove ModerationDecision = "approve"
	DecisionReject  ModerationDecision = "reject"
	DecisionBan     ModerationDecision = "ban"
)

type ModerateInput struct {
	ListingID  string
	Decision   ModerationDecision
	ReasonCode string
	Note       string
}

var ErrTransitionNotAllowed = errors.New("moderation transition not allowed")

func (s *ListingService) ModerateListing(context.Context, ModerateInput, string, repository.EnqueueFn) (repository.Listing, error) {
	return repository.Listing{}, errors.New("not implemented")
}

func (s *ListingService) ListModerationQueue(context.Context, string, int32) (repository.Page, error) {
	return repository.Page{}, errors.New("not implemented")
}
