package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/repository"
)

func TestInMemoryUserRepository(t *testing.T) {
	repo := repository.NewInMemoryUserRepository()
	ctx := context.Background()

	// 1. Create user
	u := repository.User{
		ID:           "u123",
		Username:     "test_repo_user",
		PasswordHash: "oldhash",
		Roles:        []string{"buyer"},
	}
	created, err := repo.Create(ctx, u)
	if err != nil {
		t.Fatalf("unexpected error creating user: %v", err)
	}
	if created.ID != "u123" {
		t.Errorf("expected ID u123, got %s", created.ID)
	}

	// 2. Duplicate create fails
	_, err = repo.Create(ctx, u)
	if err == nil {
		t.Errorf("expected duplicate error")
	}

	// 3. GetByUsername
	byName, err := repo.GetByUsername(ctx, "test_repo_user")
	if err != nil {
		t.Fatalf("unexpected error get by username: %v", err)
	}
	if byName.ID != "u123" {
		t.Errorf("expected u123, got %s", byName.ID)
	}

	// 4. GetByID
	byID, err := repo.GetByID(ctx, "u123")
	if err != nil {
		t.Fatalf("unexpected error get by id: %v", err)
	}
	if byID.Username != "test_repo_user" {
		t.Errorf("expected test_repo_user, got %s", byID.Username)
	}

	// 5. UpdatePassword
	err = repo.UpdatePassword(ctx, "u123", "newhash")
	if err != nil {
		t.Fatalf("unexpected error updating password: %v", err)
	}
	updated, _ := repo.GetByID(ctx, "u123")
	if updated.PasswordHash != "newhash" {
		t.Errorf("expected newhash, got %s", updated.PasswordHash)
	}

	// 6. Reset tokens
	token := repository.PasswordResetToken{
		TokenHash: "hash123",
		UserID:    "u123",
		ExpiresAt: time.Now().Add(15 * time.Minute),
		Used:      false,
		CreatedAt: time.Now(),
	}
	err = repo.CreateResetToken(ctx, token)
	if err != nil {
		t.Fatalf("unexpected error creating reset token: %v", err)
	}

	gotToken, err := repo.GetResetToken(ctx, "hash123")
	if err != nil {
		t.Fatalf("unexpected error getting reset token: %v", err)
	}
	if gotToken.UserID != "u123" || gotToken.Used {
		t.Errorf("unexpected token data: %+v", gotToken)
	}

	err = repo.MarkResetTokenUsed(ctx, "hash123")
	if err != nil {
		t.Fatalf("unexpected error marking token used: %v", err)
	}

	usedToken, _ := repo.GetResetToken(ctx, "hash123")
	if !usedToken.Used {
		t.Errorf("expected token used=true")
	}

	// Non-existent token
	_, err = repo.GetResetToken(ctx, "nonexistent")
	if err == nil {
		t.Errorf("expected error for non-existent token")
	}
}
