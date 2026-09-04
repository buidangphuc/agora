package repository_test

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-engagement/internal/repository"
)

func TestInMemoryQARepository(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewInMemoryQARepository()

	// 1. Create questions
	q1, err := repo.CreateQuestion(ctx, repository.ProductQuestion{
		ListingID:    "listing-1",
		UserID:       "user-1",
		QuestionText: "Sản phẩm có sẵn hàng không?",
	})
	if err != nil {
		t.Fatalf("CreateQuestion 1: %v", err)
	}
	if q1.ID == "" {
		t.Fatal("expected generated question ID")
	}

	_, err = repo.CreateQuestion(ctx, repository.ProductQuestion{
		ListingID:    "listing-1",
		UserID:       "user-2",
		QuestionText: "Có freeship không shop?",
	})
	if err != nil {
		t.Fatalf("CreateQuestion 2: %v", err)
	}

	// 2. Get question by ID
	fetched, err := repo.GetQuestionByID(ctx, q1.ID)
	if err != nil {
		t.Fatalf("GetQuestionByID: %v", err)
	}
	if fetched.QuestionText != "Sản phẩm có sẵn hàng không?" {
		t.Fatalf("unexpected question text: %s", fetched.QuestionText)
	}

	// 3. Create answers
	ans1, err := repo.CreateAnswer(ctx, repository.ProductAnswer{
		QuestionID:  q1.ID,
		UserID:      "seller-1",
		AnswerText:  "Dạ shop sẵn hàng ạ!",
		IsShopReply: true,
	})
	if err != nil {
		t.Fatalf("CreateAnswer: %v", err)
	}
	if ans1.ID == "" {
		t.Fatal("expected generated answer ID")
	}

	// Create answer for nonexistent question
	_, err = repo.CreateAnswer(ctx, repository.ProductAnswer{
		QuestionID: "nonexistent",
		UserID:     "user-x",
		AnswerText: "Test",
	})
	if err == nil {
		t.Fatal("expected error when answering nonexistent question")
	}

	// 4. Get question by ID now has answers
	fetchedWithAns, err := repo.GetQuestionByID(ctx, q1.ID)
	if err != nil {
		t.Fatalf("GetQuestionByID: %v", err)
	}
	if len(fetchedWithAns.Answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(fetchedWithAns.Answers))
	}
	if fetchedWithAns.Answers[0].AnswerText != "Dạ shop sẵn hàng ạ!" {
		t.Fatalf("unexpected answer text: %s", fetchedWithAns.Answers[0].AnswerText)
	}

	// 5. List questions by listing
	list, total, err := repo.ListQuestionsByListing(ctx, "listing-1", 0, 10)
	if err != nil {
		t.Fatalf("ListQuestionsByListing: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(list))
	}

	// Nonexistent listing
	emptyList, emptyTotal, err := repo.ListQuestionsByListing(ctx, "listing-none", 0, 10)
	if err != nil {
		t.Fatalf("ListQuestionsByListing nonexistent: %v", err)
	}
	if emptyTotal != 0 || len(emptyList) != 0 {
		t.Fatalf("expected empty list, got total %d, len %d", emptyTotal, len(emptyList))
	}
}
