package engagement

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/buidangphuc/team-engagement/internal/store"
)

var (
	ErrEmptyQuestionText = errors.New("question_text is required")
	ErrEmptyAnswerText   = errors.New("answer_text is required")
	ErrEmptyQuestionID   = errors.New("question_id is required")
	ErrEmptyUserID       = errors.New("user_id is required")
)

type QAService struct {
	repo   store.QARepository
	logger *slog.Logger
}

func NewQAService(repo store.QARepository, logger *slog.Logger) *QAService {
	if logger == nil {
		logger = slog.Default()
	}
	return &QAService{repo: repo, logger: logger}
}

func (s *QAService) AskQuestion(
	ctx context.Context,
	listingID, userID, questionText string,
) (store.ProductQuestion, error) {
	listingID = strings.TrimSpace(listingID)
	if listingID == "" {
		return store.ProductQuestion{}, ErrEmptyListing
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return store.ProductQuestion{}, ErrEmptyUserID
	}
	questionText = strings.TrimSpace(questionText)
	if questionText == "" {
		return store.ProductQuestion{}, ErrEmptyQuestionText
	}

	q := store.ProductQuestion{
		ListingID:    listingID,
		UserID:       userID,
		QuestionText: questionText,
	}

	saved, err := s.repo.CreateQuestion(ctx, q)
	if err != nil {
		return store.ProductQuestion{}, fmt.Errorf("create question: %w", err)
	}
	return saved, nil
}

func (s *QAService) AnswerQuestion(
	ctx context.Context,
	questionID, userID, answerText string,
	isShopReply bool,
) (store.ProductAnswer, error) {
	questionID = strings.TrimSpace(questionID)
	if questionID == "" {
		return store.ProductAnswer{}, ErrEmptyQuestionID
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return store.ProductAnswer{}, ErrEmptyUserID
	}
	answerText = strings.TrimSpace(answerText)
	if answerText == "" {
		return store.ProductAnswer{}, ErrEmptyAnswerText
	}

	a := store.ProductAnswer{
		QuestionID:  questionID,
		UserID:      userID,
		AnswerText:  answerText,
		IsShopReply: isShopReply,
	}

	saved, err := s.repo.CreateAnswer(ctx, a)
	if err != nil {
		return store.ProductAnswer{}, fmt.Errorf("create answer: %w", err)
	}
	return saved, nil
}

// GetQuestion returns one question (ErrQuestionNotFound when absent).
func (s *QAService) GetQuestion(ctx context.Context, id string) (store.ProductQuestion, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return store.ProductQuestion{}, ErrEmptyQuestionID
	}
	return s.repo.GetQuestionByID(ctx, id)
}

func (s *QAService) ListQuestionsByListing(
	ctx context.Context,
	listingID string,
	page, pageSize int,
) ([]store.ProductQuestion, int64, error) {
	listingID = strings.TrimSpace(listingID)
	if listingID == "" {
		return nil, 0, ErrEmptyListing
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return s.repo.ListQuestionsByListing(ctx, listingID, offset, pageSize)
}
