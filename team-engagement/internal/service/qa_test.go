package service_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

func TestQAService(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryQARepository()
	svc := service.NewQAService(repo, nil)

	t.Run("AskQuestion Validations", func(t *testing.T) {
		_, err := svc.AskQuestion(ctx, "", "user-1", "Hỏi câu gì đó")
		if err != service.ErrEmptyListing {
			t.Fatalf("expected ErrEmptyListing, got %v", err)
		}

		_, err = svc.AskQuestion(ctx, "listing-1", "", "Hỏi câu gì đó")
		if err != service.ErrEmptyUserID {
			t.Fatalf("expected ErrEmptyUserID, got %v", err)
		}

		_, err = svc.AskQuestion(ctx, "listing-1", "user-1", "   ")
		if err != service.ErrEmptyQuestionText {
			t.Fatalf("expected ErrEmptyQuestionText, got %v", err)
		}
	})

	t.Run("Ask and Answer Flow", func(t *testing.T) {
		q, err := svc.AskQuestion(ctx, "listing-1", "buyer-1", "Sản phẩm bảo hành bao lâu?")
		if err != nil {
			t.Fatalf("unexpected error asking question: %v", err)
		}
		if q.ID == "" {
			t.Fatal("expected question ID")
		}

		// Answer validations
		_, err = svc.AnswerQuestion(ctx, "", "seller-1", "Bảo hành 1 năm", true)
		if err != service.ErrEmptyQuestionID {
			t.Fatalf("expected ErrEmptyQuestionID, got %v", err)
		}

		_, err = svc.AnswerQuestion(ctx, q.ID, "", "Bảo hành 1 năm", true)
		if err != service.ErrEmptyUserID {
			t.Fatalf("expected ErrEmptyUserID, got %v", err)
		}

		_, err = svc.AnswerQuestion(ctx, q.ID, "seller-1", "", true)
		if err != service.ErrEmptyAnswerText {
			t.Fatalf("expected ErrEmptyAnswerText, got %v", err)
		}

		// Valid answer
		ans, err := svc.AnswerQuestion(ctx, q.ID, "seller-1", "Bảo hành chính hãng 1 năm", true)
		if err != nil {
			t.Fatalf("unexpected error answering question: %v", err)
		}
		if ans.ID == "" {
			t.Fatal("expected answer ID")
		}
		if !ans.IsShopReply {
			t.Fatal("expected IsShopReply true")
		}

		// List questions
		questions, total, err := svc.ListQuestionsByListing(ctx, "listing-1", 1, 10)
		if err != nil {
			t.Fatalf("unexpected error listing questions: %v", err)
		}
		if total != 1 || len(questions) != 1 {
			t.Fatalf("expected 1 question, got total %d, count %d", total, len(questions))
		}
		if len(questions[0].Answers) != 1 {
			t.Fatalf("expected 1 answer attached, got %d", len(questions[0].Answers))
		}
	})

	t.Run("ListQuestions Validations", func(t *testing.T) {
		_, _, err := svc.ListQuestionsByListing(ctx, "", 1, 10)
		if err != service.ErrEmptyListing {
			t.Fatalf("expected ErrEmptyListing, got %v", err)
		}
	})
}
