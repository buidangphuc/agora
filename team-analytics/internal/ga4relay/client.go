package ga4relay

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// DefaultRequestTimeout bounds a single GA4 request attempt.
const DefaultRequestTimeout = 5 * time.Second

const (
	maxAttempts = 5
	maxBackoff  = 30 * time.Second
)

// Backoff returns the delay before retry number `retry` (0-based): 1s*2^n, capped at 30s.
func Backoff(retry int) time.Duration {
	if retry < 0 {
		retry = 0
	}
	if retry >= 5 {
		return maxBackoff
	}
	d := time.Second << uint(retry)
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

// SendError is returned when a payload could not be delivered.
type SendError struct {
	Reason   string // "4xx" | "exhausted"
	Status   int
	Attempts int
}

func (e *SendError) Error() string {
	return fmt.Sprintf("ga4 send failed: reason=%s status=%d attempts=%d", e.Reason, e.Status, e.Attempts)
}

// ClientConfig configures a Client.
type ClientConfig struct {
	Endpoint      string
	MeasurementID string
	APISecret     string
	Logger        *slog.Logger
	Sleep         func(ctx context.Context, d time.Duration) error
	Timeout       time.Duration
}

// Client posts Measurement Protocol payloads to GA4 with retry.
type Client struct {
	cfg  ClientConfig
	http *http.Client
}

// NewClient builds a Client, applying defaults.
func NewClient(cfg ClientConfig) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultRequestTimeout
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if cfg.Sleep == nil {
		cfg.Sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	return &Client{cfg: cfg, http: &http.Client{}}
}

// Send delivers body, retrying 429/5xx/network errors up to 5 attempts.
func (c *Client) Send(ctx context.Context, body []byte) error {
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return errors.New("ga4 send failed: invalid endpoint")
	}
	q := u.Query()
	q.Set("measurement_id", c.cfg.MeasurementID)
	q.Set("api_secret", c.cfg.APISecret)
	u.RawQuery = q.Encode()
	target := u.String()

	status := 0
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		status = c.once(ctx, target, body)
		if status >= 200 && status < 300 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if status >= 400 && status < 500 && status != http.StatusTooManyRequests {
			c.cfg.Logger.Warn("ga4 send rejected", "status", status, "attempt", attempt)
			return &SendError{Reason: "4xx", Status: status, Attempts: attempt}
		}
		c.cfg.Logger.Warn("ga4 send attempt failed", "status", status, "attempt", attempt)
		if attempt == maxAttempts {
			break
		}
		if err := c.cfg.Sleep(ctx, Backoff(attempt-1)); err != nil {
			return err
		}
	}
	return &SendError{Reason: "exhausted", Status: status, Attempts: maxAttempts}
}

// once performs a single attempt; returns the HTTP status or 0 on network error/timeout.
func (c *Client) once(ctx context.Context, target string, body []byte) int {
	rctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error embeds the URL (with api_secret); log only the inner cause.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		c.cfg.Logger.Warn("ga4 request error", "error", err.Error())
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}
