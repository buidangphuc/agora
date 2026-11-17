package verification

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-verification/internal/objects"
	"github.com/buidangphuc/team-verification/internal/store"
)

const (
	maxUploadBytes  = 5 << 20
	maxDocuments    = 3
	maxOpenGrants   = 10
	uploadGrantTTL  = 15 * time.Minute
	documentURLTTL  = 5 * time.Minute
	defaultPageSize = 20
	maxPageSize     = 100
	maxBadgeUserIDs = 50
)

var (
	ErrInvalidUpload   = errors.New("content_type must be image/jpeg, image/png or application/pdf and size_bytes between 1 and 5242880")
	ErrTooManyDocs     = errors.New("at most 3 documents per submission")
	ErrObjectMissing   = errors.New("document has not been uploaded")
	ErrObjectMismatch  = errors.New("uploaded document does not match its upload grant")
	ErrInvalidReason   = errors.New("reason_code is required (lower_snake_case, at most 64 characters) when rejecting and must be empty when approving")
	ErrInvalidStatus   = errors.New("status filter must be PENDING, VERIFIED or REJECTED")
	ErrInvalidCursor   = errors.New("invalid page cursor")
	ErrInvalidIndex    = errors.New("document index out of range")
	ErrInvalidIDs      = errors.New("user_ids must contain between 1 and 50 ids")
	ErrNoObjectStorage = errors.New("object storage is not configured")

	docRefPattern = regexp.MustCompile(`^kyc/[A-Za-z0-9_-]+/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reasonPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
)

var allowedContentTypes = map[string]bool{"image/jpeg": true, "image/png": true, "application/pdf": true}

type Option func(*VerificationService)

func WithObjects(o objects.Store) Option { return func(s *VerificationService) { s.objs = o } }

func WithClock(c func() time.Time) Option { return func(s *VerificationService) { s.now = c } }

type Upload struct {
	URL       string
	DocRef    string
	ExpiresAt time.Time
}

type Page struct {
	Submissions []store.Submission
	NextCursor  string
}

var (
	ErrEmptyUser     = errors.New("user id is required")
	ErrEmptyDocType  = errors.New("doc_type is required")
	ErrEmptyDocRef   = errors.New("doc_ref is required")
	ErrEmptyID       = errors.New("submission id is required")
	ErrInvalidReview = errors.New("decision must be 'approve' or 'reject'")
)

type VerificationService struct {
	repo store.KycRepository
	objs objects.Store
	now  func() time.Time
}

func NewVerificationService(repo store.KycRepository, opts ...Option) *VerificationService {
	s := &VerificationService{repo: repo, now: func() time.Time { return time.Now().UTC() }}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *VerificationService) Submit(ctx context.Context, userID, docType, docRef string) (*store.Submission, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	if strings.TrimSpace(docType) == "" {
		return nil, ErrEmptyDocType
	}
	if strings.TrimSpace(docRef) == "" {
		return nil, ErrEmptyDocRef
	}
	sub := &store.Submission{
		ID:      "kyc_" + uuid.NewString(),
		UserID:  userID,
		DocType: docType,
		DocRef:  docRef,
		Status:  store.StatusPending,
	}
	return s.repo.Create(ctx, sub)
}

func (s *VerificationService) GetStatus(ctx context.Context, userID string) (store.Status, bool, error) {
	if userID == "" {
		return "", false, ErrEmptyUser
	}
	sub, err := s.repo.GetLatestByUser(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return store.StatusPending, false, nil
	}
	if err != nil {
		return "", false, err
	}
	return sub.Status, badgeFor(sub.Status), nil
}

func (s *VerificationService) Get(ctx context.Context, id string) (*store.Submission, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	return s.repo.GetByID(ctx, id)
}

func NormalizeDecision(decision string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "approve", "approved", "verify", "verified":
		return "approve", nil
	case "reject", "rejected", "deny", "denied":
		return "reject", nil
	default:
		return "", ErrInvalidReview
	}
}

func (s *VerificationService) Review(ctx context.Context, id, decision string) (store.Status, error) {
	if id == "" {
		return "", ErrEmptyID
	}
	norm, err := NormalizeDecision(decision)
	if err != nil {
		return "", err
	}
	next := store.StatusRejected
	if norm == "approve" {
		next = store.StatusVerified
	}
	sub, err := s.repo.UpdateStatus(ctx, id, next, time.Now().UTC())
	if err != nil {
		return "", err
	}
	return sub.Status, nil
}

func badgeFor(status store.Status) bool {
	return status == store.StatusVerified
}

func (s *VerificationService) CreateUpload(ctx context.Context, userID, contentType string, size int64) (*Upload, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	if !allowedContentTypes[contentType] || size < 1 || size > maxUploadBytes {
		return nil, ErrInvalidUpload
	}
	if s.objs == nil {
		return nil, ErrNoObjectStorage
	}
	now := s.now()
	key := "kyc/" + userID + "/" + uuid.NewString()
	url, err := s.objs.PresignPut(ctx, key, contentType, size, uploadGrantTTL)
	if err != nil {
		return nil, err
	}
	g := &store.Grant{ObjectKey: key, UserID: userID, ContentType: contentType, SizeBytes: size, IssuedAt: now, ExpiresAt: now.Add(uploadGrantTTL)}
	if err := s.repo.CreateGrant(ctx, g, maxOpenGrants, now); err != nil {
		return nil, err
	}
	return &Upload{URL: url, DocRef: key, ExpiresAt: g.ExpiresAt}, nil
}

func (s *VerificationService) SubmitDocuments(ctx context.Context, userID, docType string, refs []string) (*store.Submission, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	if strings.TrimSpace(docType) == "" {
		return nil, ErrEmptyDocType
	}
	if len(refs) == 0 || strings.TrimSpace(refs[0]) == "" {
		return nil, ErrEmptyDocRef
	}
	if len(refs) > maxDocuments {
		return nil, ErrTooManyDocs
	}
	for _, r := range refs {
		if !docRefPattern.MatchString(r) {
			return nil, store.ErrGrantInvalid
		}
	}
	if s.objs == nil {
		return nil, ErrNoObjectStorage
	}
	sub := &store.Submission{
		ID:      "kyc_" + uuid.NewString(),
		UserID:  userID,
		DocType: docType,
		Status:  store.StatusPending,
	}
	return s.repo.SubmitWithDocuments(ctx, sub, refs, s.now(), func(grants []store.Grant) error {
		return s.verifyObjects(ctx, grants)
	})
}

func (s *VerificationService) verifyObjects(ctx context.Context, grants []store.Grant) error {
	for _, g := range grants {
		info, err := s.objs.Stat(ctx, g.ObjectKey)
		if errors.Is(err, objects.ErrNotFound) {
			return ErrObjectMissing
		}
		if err != nil {
			return err
		}
		if info.Size != g.SizeBytes || info.Size > maxUploadBytes || info.ContentType != g.ContentType {
			if derr := s.objs.Delete(ctx, g.ObjectKey); derr != nil {
				return derr
			}
			return ErrObjectMismatch
		}
	}
	return nil
}

func (s *VerificationService) Latest(ctx context.Context, userID string) (*store.Submission, error) {
	if userID == "" {
		return nil, ErrEmptyUser
	}
	sub, err := s.repo.GetLatestByUser(ctx, userID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return sub, err
}

func ValidateReason(decision, reason string) error {
	if decision == "approve" {
		if reason != "" {
			return ErrInvalidReason
		}
		return nil
	}
	if !reasonPattern.MatchString(reason) {
		return ErrInvalidReason
	}
	return nil
}

func (s *VerificationService) ReviewWithReason(ctx context.Context, id, decision, reason string) (store.Status, error) {
	if id == "" {
		return "", ErrEmptyID
	}
	norm, err := NormalizeDecision(decision)
	if err != nil {
		return "", err
	}
	if err := ValidateReason(norm, reason); err != nil {
		return "", err
	}
	if norm == "approve" {
		return s.Review(ctx, id, norm)
	}
	sub, err := s.repo.ReviewWithReason(ctx, id, store.StatusRejected, reason, s.now())
	if err != nil {
		return "", err
	}
	return sub.Status, nil
}

func (s *VerificationService) List(ctx context.Context, status store.Status, cursor string, size int32) (*Page, error) {
	switch status {
	case "", store.StatusPending, store.StatusVerified, store.StatusRejected:
	default:
		return nil, ErrInvalidStatus
	}
	limit := int(size)
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	f := store.ListFilter{Status: status, Limit: limit + 1}
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		nanos, id, ok := strings.Cut(string(raw), "|")
		n, perr := strconv.ParseInt(nanos, 10, 64)
		if !ok || perr != nil || id == "" {
			return nil, ErrInvalidCursor
		}
		f.HasCursor, f.AfterAt, f.AfterID = true, time.Unix(0, n).UTC(), id
	}
	rows, err := s.repo.ListSubmissions(ctx, f)
	if err != nil {
		return nil, err
	}
	page := &Page{Submissions: rows}
	if len(rows) > limit {
		page.Submissions = rows[:limit]
		last := page.Submissions[limit-1]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(last.CreatedAt.UnixNano(), 10) + "|" + last.ID))
	}
	return page, nil
}

func (s *VerificationService) DocumentKey(ctx context.Context, submissionID string, index int32) (*store.Submission, string, error) {
	sub, err := s.Get(ctx, submissionID)
	if err != nil {
		return nil, "", err
	}
	docs, err := s.repo.ListDocuments(ctx, submissionID)
	if err != nil {
		return nil, "", err
	}
	if index < 0 || int(index) >= len(docs) {
		return nil, "", ErrInvalidIndex
	}
	if docs[index].DeletedAt != nil {
		return nil, "", store.ErrNotFound
	}
	return sub, docs[index].ObjectKey, nil
}

func (s *VerificationService) DocumentURL(ctx context.Context, key string) (string, time.Time, error) {
	if s.objs == nil {
		return "", time.Time{}, ErrNoObjectStorage
	}
	exp := s.now().Add(documentURLTTL)
	url, err := s.objs.PresignGet(ctx, key, documentURLTTL)
	return url, exp, err
}

func (s *VerificationService) Badges(ctx context.Context, ids []string) (map[string]bool, []string, error) {
	seen := map[string]bool{}
	var distinct []string
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			distinct = append(distinct, id)
		}
	}
	if len(ids) == 0 || len(ids) > maxBadgeUserIDs || len(distinct) == 0 {
		return nil, nil, ErrInvalidIDs
	}
	st, err := s.repo.LatestStatuses(ctx, distinct)
	if err != nil {
		return nil, nil, err
	}
	out := make(map[string]bool, len(distinct))
	for _, id := range distinct {
		out[id] = badgeFor(st[id])
	}
	return out, distinct, nil
}
