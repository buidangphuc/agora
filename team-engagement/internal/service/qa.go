package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/buidangphuc/team-engagement/internal/repository"
)

var (
	ErrEmptyQuestionText = errors.New("question_text is required")
	ErrEmptyAnswerText   = errors.New("answer_text is required")
	ErrEmptyQuestionID   = errors.New("question_id is required")
	ErrEmptyUserID       = errors.New("user_id is required")
)

type QAService struct {
	repo   repository.QARepository
	logger *slog.Logger
}

func NewQAService(repo repository.QARepository, logger *slog.Logger) *QAService {
	if logger == nil {
		logger = slog.Default()
	}
	return &QAService{repo: repo, logger: logger}
}

func (s *QAService) AskQuestion(
	ctx context.Context,
	listingID, userID, questionText string,
) (repository.ProductQuestion, error) {
	listingID = strings.TrimSpace(listingID)
	if listingID == "" {
		return repository.ProductQuestion{}, ErrEmptyListing
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return repository.ProductQuestion{}, ErrEmptyUserID
	}
	questionText = strings.TrimSpace(questionText)
	if questionText == "" {
		return repository.ProductQuestion{}, ErrEmptyQuestionText
	}

	q := repository.ProductQuestion{
		ListingID:    listingID,
		UserID:       userID,
		QuestionText: questionText,
	}

	saved, err := s.repo.CreateQuestion(ctx, q)
	if err != nil {
		return repository.ProductQuestion{}, fmt.Errorf("create question: %w", err)
	}
	return saved, nil
}

func (s *QAService) AnswerQuestion(
	ctx context.Context,
	questionID, userID, answerText string,
	isShopReply bool,
) (repository.ProductAnswer, error) {
	questionID = strings.TrimSpace(questionID)
	if questionID == "" {
		return repository.ProductAnswer{}, ErrEmptyQuestionID
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return repository.ProductAnswer{}, ErrEmptyUserID
	}
	answerText = strings.TrimSpace(answerText)
	if answerText == "" {
		return repository.ProductAnswer{}, ErrEmptyAnswerText
	}

	a := repository.ProductAnswer{
		QuestionID:  questionID,
		UserID:      userID,
		AnswerText:  answerText,
		IsShopReply: isShopReply,
	}

	saved, err := s.repo.CreateAnswer(ctx, a)
	if err != nil {
		return repository.ProductAnswer{}, fmt.Errorf("create answer: %w", err)
	}
	return saved, nil
}

func (s *QAService) ListQuestionsByListing(
	ctx context.Context,
	listingID string,
	page, pageSize int,
) ([]repository.ProductQuestion, int64, error) {
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
