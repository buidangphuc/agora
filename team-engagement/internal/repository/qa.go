package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrQuestionNotFound = errors.New("question not found")
)

type ProductAnswer struct {
	ID          string
	QuestionID  string
	ListingID   string
	UserID      string
	AnswerText  string
	IsShopReply bool
	CreatedAt   time.Time
}

type ProductQuestion struct {
	ID           string
	ListingID    string
	UserID       string
	QuestionText string
	Answers      []ProductAnswer
	CreatedAt    time.Time
}

type QARepository interface {
	CreateQuestion(ctx context.Context, q ProductQuestion) (ProductQuestion, error)
	GetQuestionByID(ctx context.Context, id string) (ProductQuestion, error)
	CreateAnswer(ctx context.Context, a ProductAnswer) (ProductAnswer, error)
	ListQuestionsByListing(ctx context.Context, listingID string, offset, limit int) ([]ProductQuestion, int64, error)
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresQARepository struct {
	pool *pgxpool.Pool
}

func NewPostgresQARepository(pool *pgxpool.Pool) *PostgresQARepository {
	return &PostgresQARepository{pool: pool}
}

const (
	questionColumns = `id, listing_id, user_id, question_text, created_at`
	answerColumns   = `id, question_id, listing_id, user_id, answer_text, is_shop_reply, created_at`
)

func (r *PostgresQARepository) CreateQuestion(ctx context.Context, q ProductQuestion) (ProductQuestion, error) {
	if q.ID == "" {
		q.ID = uuid.NewString()
	}
	if q.CreatedAt.IsZero() {
		q.CreatedAt = time.Now().UTC()
	}

	const query = `INSERT INTO product_questions (id, listing_id, user_id, question_text, created_at)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := r.pool.Exec(ctx, query, q.ID, q.ListingID, q.UserID, q.QuestionText, q.CreatedAt); err != nil {
		return ProductQuestion{}, fmt.Errorf("insert product question: %w", err)
	}
	if q.Answers == nil {
		q.Answers = make([]ProductAnswer, 0)
	}
	return q, nil
}

func (r *PostgresQARepository) GetQuestionByID(ctx context.Context, id string) (ProductQuestion, error) {
	var q ProductQuestion
	const query = `SELECT ` + questionColumns + ` FROM product_questions WHERE id = $1`
	if err := r.pool.QueryRow(ctx, query, id).Scan(&q.ID, &q.ListingID, &q.UserID, &q.QuestionText, &q.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProductQuestion{}, ErrQuestionNotFound
		}
		return ProductQuestion{}, fmt.Errorf("get question: %w", err)
	}

	answers, err := r.getAnswersForQuestion(ctx, q.ID)
	if err != nil {
		return ProductQuestion{}, err
	}
	q.Answers = answers
	return q, nil
}

func (r *PostgresQARepository) CreateAnswer(ctx context.Context, a ProductAnswer) (ProductAnswer, error) {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}

	const query = `INSERT INTO product_answers (id, question_id, listing_id, user_id, answer_text, is_shop_reply, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	if _, err := r.pool.Exec(ctx, query, a.ID, a.QuestionID, a.ListingID, a.UserID, a.AnswerText, a.IsShopReply, a.CreatedAt); err != nil {
		return ProductAnswer{}, fmt.Errorf("insert product answer: %w", err)
	}
	return a, nil
}

func (r *PostgresQARepository) ListQuestionsByListing(ctx context.Context, listingID string, offset, limit int) ([]ProductQuestion, int64, error) {
	var total int64
	countQuery := `SELECT COUNT(*) FROM product_questions WHERE listing_id = $1`
	if err := r.pool.QueryRow(ctx, countQuery, listingID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count questions: %w", err)
	}

	if total == 0 {
		return make([]ProductQuestion, 0), 0, nil
	}

	listQuery := `SELECT ` + questionColumns + ` FROM product_questions WHERE listing_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, listQuery, listingID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query questions: %w", err)
	}
	defer rows.Close()

	var questions []ProductQuestion
	var questionIDs []string
	for rows.Next() {
		var q ProductQuestion
		if err := rows.Scan(&q.ID, &q.ListingID, &q.UserID, &q.QuestionText, &q.CreatedAt); err != nil {
			return nil, 0, err
		}
		q.Answers = make([]ProductAnswer, 0)
		questions = append(questions, q)
		questionIDs = append(questionIDs, q.ID)
	}

	if len(questionIDs) > 0 {
		answersMap, err := r.getAnswersForQuestions(ctx, questionIDs)
		if err != nil {
			return nil, 0, err
		}
		for i := range questions {
			if ans, ok := answersMap[questions[i].ID]; ok {
				questions[i].Answers = ans
			}
		}
	}

	return questions, total, nil
}

func (r *PostgresQARepository) getAnswersForQuestion(ctx context.Context, questionID string) ([]ProductAnswer, error) {
	m, err := r.getAnswersForQuestions(ctx, []string{questionID})
	if err != nil {
		return nil, err
	}
	if ans, ok := m[questionID]; ok {
		return ans, nil
	}
	return make([]ProductAnswer, 0), nil
}

func (r *PostgresQARepository) getAnswersForQuestions(ctx context.Context, questionIDs []string) (map[string][]ProductAnswer, error) {
	const query = `SELECT ` + answerColumns + ` FROM product_answers WHERE question_id = ANY($1) ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, query, questionIDs)
	if err != nil {
		return nil, fmt.Errorf("query answers: %w", err)
	}
	defer rows.Close()

	m := make(map[string][]ProductAnswer)
	for rows.Next() {
		var a ProductAnswer
		if err := rows.Scan(&a.ID, &a.QuestionID, &a.ListingID, &a.UserID, &a.AnswerText, &a.IsShopReply, &a.CreatedAt); err != nil {
			return nil, err
		}
		m[a.QuestionID] = append(m[a.QuestionID], a)
	}
	return m, nil
}

// ── InMemory Implementation ───────────────────────────────────────────

type InMemoryQARepository struct {
	mu        sync.RWMutex
	questions []ProductQuestion
	answers   []ProductAnswer
}

func NewInMemoryQARepository() *InMemoryQARepository {
	return &InMemoryQARepository{
		questions: make([]ProductQuestion, 0),
		answers:   make([]ProductAnswer, 0),
	}
}

func (r *InMemoryQARepository) CreateQuestion(_ context.Context, q ProductQuestion) (ProductQuestion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if q.ID == "" {
		q.ID = uuid.NewString()
	}
	if q.CreatedAt.IsZero() {
		q.CreatedAt = time.Now().UTC()
	}
	if q.Answers == nil {
		q.Answers = make([]ProductAnswer, 0)
	}
	r.questions = append(r.questions, q)
	return q, nil
}

func (r *InMemoryQARepository) GetQuestionByID(_ context.Context, id string) (ProductQuestion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, q := range r.questions {
		if q.ID == id {
			res := q
			res.Answers = make([]ProductAnswer, 0)
			for _, a := range r.answers {
				if a.QuestionID == id {
					res.Answers = append(res.Answers, a)
				}
			}
			return res, nil
		}
	}
	return ProductQuestion{}, ErrQuestionNotFound
}

func (r *InMemoryQARepository) CreateAnswer(_ context.Context, a ProductAnswer) (ProductAnswer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if question exists
	found := false
	for _, q := range r.questions {
		if q.ID == a.QuestionID {
			found = true
			if a.ListingID == "" {
				a.ListingID = q.ListingID
			}
			break
		}
	}
	if !found {
		return ProductAnswer{}, ErrQuestionNotFound
	}

	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	r.answers = append(r.answers, a)
	return a, nil
}

func (r *InMemoryQARepository) ListQuestionsByListing(_ context.Context, listingID string, offset, limit int) ([]ProductQuestion, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []ProductQuestion
	for i := len(r.questions) - 1; i >= 0; i-- {
		q := r.questions[i]
		if q.ListingID == listingID {
			qCopy := q
			qCopy.Answers = make([]ProductAnswer, 0)
			for _, a := range r.answers {
				if a.QuestionID == q.ID {
					qCopy.Answers = append(qCopy.Answers, a)
				}
			}
			filtered = append(filtered, qCopy)
		}
	}

	total := int64(len(filtered))
	if offset >= len(filtered) {
		return make([]ProductQuestion, 0), total, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

var (
	_ QARepository = (*PostgresQARepository)(nil)
	_ QARepository = (*InMemoryQARepository)(nil)
)
