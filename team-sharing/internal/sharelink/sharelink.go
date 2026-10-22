// This file holds the share-link use case: minting a short code for a target
// (with UTM + synthesized Open Graph preview metadata) and resolving a short
// code back to its target while counting the click. It mirrors the notification
// service's layering — validate here, persist in the repository; the short-code
// alphabet and OG defaults are the only domain specifics.
package sharelink

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// Validation errors, mapped to InvalidArgument by the handler.
var (
	ErrEmptyTargetType = errors.New("target_type is required")
	ErrEmptyTargetID   = errors.New("target_id is required")
	ErrEmptyShortCode  = errors.New("short_code is required")
)

var (
	ErrShareLinkNotFound = errors.New("share link not found")
	ErrShortCodeExists   = errors.New("short code already exists")
)

type ShareLink struct {
	ShortCode     string
	TargetType    string
	TargetID      string
	CreatedBy     string
	UTM           map[string]string
	OgTitle       string
	OgDescription string
	OgImageURL    string
	ClickCount    int64
	CreatedAt     time.Time
}

type Repository interface {
	Create(ctx context.Context, link *ShareLink) (*ShareLink, error)
	Resolve(ctx context.Context, shortCode string) (*ShareLink, error)
}

const (
	// base62Alphabet is the short-code alphabet: URL-safe, case-sensitive.
	base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// shortCodeLen is the number of base62 characters in a generated code
	// (~62^7 ≈ 3.5e12 space — ample headroom against collisions).
	shortCodeLen = 7
	// createMaxAttempts bounds the collision-retry loop when minting a code.
	createMaxAttempts = 5
)

// Service is the share-link use-case boundary.
type Service struct {
	repo Repository
	// randCode is injectable so tests can force a collision; production uses
	// the crypto/rand-backed generator.
	randCode func() (string, error)
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, randCode: randBase62}
}

// CreateShareLink mints a short code for the given target, stamps the UTM map and
// a synthesized OG preview, and stores it. target_type/target_id are required;
// utm may be nil (stored as empty). createdBy is the authenticated principal id
// ("" for anonymous callers); the link itself is a public artifact.
func (s *Service) CreateShareLink(ctx context.Context, createdBy, targetType, targetID string, utm map[string]string) (*ShareLink, error) {
	if targetType == "" {
		return nil, ErrEmptyTargetType
	}
	if targetID == "" {
		return nil, ErrEmptyTargetID
	}

	if utm == nil {
		utm = map[string]string{}
	}
	title, desc, image := defaultOgMeta(targetType, targetID)
	var lastErr error
	for attempt := 0; attempt < createMaxAttempts; attempt++ {
		code, err := s.randCode()
		if err != nil {
			return nil, err
		}
		link := &ShareLink{
			ShortCode:     code,
			TargetType:    targetType,
			TargetID:      targetID,
			CreatedBy:     createdBy,
			UTM:           utm,
			OgTitle:       title,
			OgDescription: desc,
			OgImageURL:    image,
		}
		stored, err := s.repo.Create(ctx, link)
		if errors.Is(err, ErrShortCodeExists) {
			lastErr = err
			continue // collision — try a fresh code
		}
		if err != nil {
			return nil, err
		}
		return stored, nil
	}
	return nil, fmt.Errorf("could not mint a unique short code after %d attempts: %w", createMaxAttempts, lastErr)
}

// ResolveShareLink looks up a short code, counts the click, and returns the
// target + UTM + OG metadata. Returns ErrShareLinkNotFound for unknown codes.
func (s *Service) ResolveShareLink(ctx context.Context, shortCode string) (*ShareLink, error) {
	if shortCode == "" {
		return nil, ErrEmptyShortCode
	}
	return s.repo.Resolve(ctx, shortCode)
}

// defaultOgMeta synthesizes a Vietnamese Open Graph preview for a target. Real
// title/description/image would be fetched from the owning service; until that
// enrichment is wired we return a deterministic, human-readable placeholder.
func defaultOgMeta(targetType, targetID string) (title, desc, image string) {
	title = fmt.Sprintf("Xem %s #%s trên Batdongsan", targetType, targetID)
	desc = fmt.Sprintf("Nhấn để xem chi tiết %s này trên Batdongsan.", targetType)
	image = fmt.Sprintf("https://cdn.batdongsan.com.vn/og/%s/%s.png", targetType, targetID)
	return title, desc, image
}

// randBase62 returns a cryptographically random shortCodeLen-character base62
// string.
func randBase62() (string, error) {
	b := make([]byte, shortCodeLen)
	alphabetLen := big.NewInt(int64(len(base62Alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		b[i] = base62Alphabet[n.Int64()]
	}
	return string(b), nil
}
