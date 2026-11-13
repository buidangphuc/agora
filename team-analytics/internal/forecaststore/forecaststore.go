package forecaststore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/redis/go-redis/v9"
)

const maxPayloadBytes = 64 * 1024

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

var errUnavailable = errors.New("forecast store unavailable")

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

type Store struct {
	client  *redis.Client
	timeout time.Duration
}

func New(redisURL string, readTimeout time.Duration) (*Store, error) {
	if redisURL == "" {
		return &Store{timeout: readTimeout}, nil
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("forecast store url: %w", err)
	}
	opt.ReadTimeout = readTimeout
	opt.WriteTimeout = readTimeout
	opt.DialTimeout = readTimeout
	opt.MaxRetries = -1
	return &Store{client: redis.NewClient(opt), timeout: readTimeout}, nil
}

type wirePoint struct {
	Date string   `json:"date"`
	P10  *float64 `json:"p10"`
	P50  *float64 `json:"p50"`
	P90  *float64 `json:"p90"`
}

type wire struct {
	ModelVersion string      `json:"model_version"`
	GeneratedAt  string      `json:"generated_at"`
	IsColdStart  bool        `json:"is_cold_start"`
	Points       []wirePoint `json:"daily_forecasts"`
}

func (s *Store) Get(ctx context.Context, sellerID, listingID string) (Forecast, bool, error) {
	if !idPattern.MatchString(sellerID) || !idPattern.MatchString(listingID) {
		return Forecast{}, false, errors.New("invalid seller or listing id")
	}
	if s.client == nil {
		return Forecast{}, false, errUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	raw, err := s.client.Get(ctx, "fc:v1:seller:"+sellerID+":"+listingID).Bytes()
	if errors.Is(err, redis.Nil) {
		return Forecast{}, false, nil
	}
	if err != nil {
		return Forecast{}, false, fmt.Errorf("%w: %v", errUnavailable, err)
	}
	f, err := decode(raw)
	if err != nil {
		return Forecast{}, false, err
	}
	return f, true, nil
}

func decode(raw []byte) (Forecast, error) {
	if len(raw) > maxPayloadBytes {
		return Forecast{}, errors.New("forecast payload too large")
	}
	var w wire
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&w); err != nil {
		return Forecast{}, fmt.Errorf("forecast payload malformed: %w", err)
	}
	if dec.More() {
		return Forecast{}, errors.New("forecast payload malformed: trailing data")
	}
	at, err := time.Parse(time.RFC3339, w.GeneratedAt)
	if err != nil {
		return Forecast{}, fmt.Errorf("forecast payload malformed: generated_at: %w", err)
	}
	out := Forecast{ModelVersion: w.ModelVersion, GeneratedAt: at, IsColdStart: w.IsColdStart}
	for _, p := range w.Points {
		if p.P10 == nil || p.P50 == nil || p.P90 == nil {
			return Forecast{}, errors.New("forecast payload malformed: missing quantity")
		}
		a, b, c := *p.P10, *p.P50, *p.P90
		for _, v := range []float64{a, b, c} {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
				return Forecast{}, errors.New("forecast payload malformed: bad quantity")
			}
		}
		if a > b || b > c {
			return Forecast{}, errors.New("forecast payload malformed: quantiles out of order")
		}
		out.Points = append(out.Points, Point{Date: p.Date, P10: a, P50: b, P90: c})
	}
	return out, nil
}
