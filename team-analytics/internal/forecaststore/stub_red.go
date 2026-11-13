package forecaststore

// RED-PHASE STUB: the implementer replaces this file (go-redis/v9 reader).
// It exists only so the tests compile; it implements no behaviour.

import (
	"context"
	"time"
)

type Point struct {
	Date          string
	P10, P50, P90 float64
}

type Forecast struct {
	ModelVersion string
	GeneratedAt  time.Time
	IsColdStart  bool
	Points       []Point
}

type Store struct{}

func New(redisURL string, readTimeout time.Duration) (*Store, error) { return &Store{}, nil }

func (s *Store) Get(ctx context.Context, sellerID, listingID string) (Forecast, bool, error) {
	return Forecast{}, false, nil
}
