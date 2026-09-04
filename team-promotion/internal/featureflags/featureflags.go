// Package featureflags wires the CNCF OpenFeature SDK (vendor-neutral) with the
// Flipt provider into team-promotion. Call sites depend only on the tiny Evaluator
// interface — they never import OpenFeature or Flipt directly, so Flipt stays a
// swappable provider (anti-lock-in) and tests can supply a fake.
//
// This same shape is copy-ready for the other Go services (team-domain,
// team-gateway, …) when they adopt flags — a per-repo init mirroring the
// reference template, not a shared central library.
package featureflags

import (
	"context"
	"log/slog"

	"github.com/open-feature/go-sdk/openfeature"
)

// FlagFlashSaleEnabled is the boolean emergency kill-switch for flash-sale
// redemption. Its default is ON (true): an emergency kill-switch is an explicit
// human OFF action during an incident, so a Flipt outage must NOT disable
// promotions (fail-open). Real gating lands with the Wave 1 business logic.
const FlagFlashSaleEnabled = "flash-sale-enabled"

// clientName is the OpenFeature client/logical name for this service.
const clientName = "team-promotion"

// Evaluator is the minimal flag-reading surface handlers depend on. Kept tiny so
// call sites never import the OpenFeature or Flipt packages, and so tests can
// substitute a fake.
type Evaluator interface {
	// BooleanEnabled evaluates a boolean flag. On ANY provider error it returns
	// defaultValue — the caller chooses fail-open vs fail-closed by the default
	// it passes (checkout passes true → fail-open).
	BooleanEnabled(ctx context.Context, flag string, defaultValue bool) bool
}

// Config is the feature-flag slice of team-promotion settings.
type Config struct {
	// Enabled turns the Flipt-backed provider on. When false, evaluations always
	// return the caller's default (fail-open) and Flipt is never contacted.
	Enabled bool
	// FliptAddr is the Flipt gRPC endpoint (e.g. "flipt:9000" in compose,
	// "flipt.marketplace.svc:9000" in-cluster).
	FliptAddr string
	// EvalTimeoutMS bounds a single evaluation. With streaming/in-memory
	// evaluation a check is a local lookup, so this only guards the degenerate
	// case where the provider blocks.
	EvalTimeoutMS int
}

// Client is the OpenFeature-backed Evaluator. A disabled Client (nil ofClient)
// always returns the caller's default — the fail-open path used when flags are
// disabled or Flipt is unreachable at boot.
type Client struct {
	ofClient *openfeature.Client
	logger   *slog.Logger
	enabled  bool
}

var _ Evaluator = (*Client)(nil)

// New constructs the OpenFeature client backed by the Flipt provider configured
// for streaming / in-process (in-memory snapshot) evaluation. When cfg.Enabled is
// false it returns a disabled (fail-open) client and never touches Flipt.
//
// A non-nil error means the provider could not be initialized; callers
// (bootstrap) should degrade to Disabled() rather than block boot.
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if !cfg.Enabled {
		logger.Info("feature flags disabled; evaluations fail open to defaults")
		return &Client{logger: logger}, nil
	}

	provider, err := newFliptProvider(cfg)
	if err != nil {
		return nil, err
	}
	// SetProviderAndWait blocks until the provider is READY (initial snapshot
	// pulled from Flipt), so the very first evaluation is already a local lookup.
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		return nil, err
	}
	logger.Info("feature flags enabled via Flipt provider", slog.String("flipt_addr", cfg.FliptAddr))
	return &Client{
		ofClient: openfeature.NewClient(clientName),
		logger:   logger,
		enabled:  true,
	}, nil
}

// Disabled returns a fail-open client that always yields the caller's default.
// Used by bootstrap when Flipt is unreachable so boot is never blocked.
func Disabled(logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{logger: logger}
}

// NewWithClient wraps an existing OpenFeature client. Used by tests to back the
// Evaluator with an in-memory (fake) provider.
func NewWithClient(ofClient *openfeature.Client, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{ofClient: ofClient, logger: logger, enabled: true}
}

// BooleanEnabled evaluates a boolean flag, failing to defaultValue on any error.
func (c *Client) BooleanEnabled(ctx context.Context, flag string, defaultValue bool) bool {
	if c == nil || c.ofClient == nil {
		return defaultValue
	}
	evalCtx := openfeature.NewEvaluationContext("system", map[string]interface{}{})
	val, err := c.ofClient.BooleanValue(ctx, flag, defaultValue, evalCtx)
	if err != nil {
		// Fail to the caller's default. A flag-system outage must not become a
		// checkout outage (checkout passes default=true → fail-open).
		c.logger.Warn("feature flag evaluation failed; using default",
			slog.String("flag", flag),
			slog.Bool("default", defaultValue),
			slog.Any("err", err),
		)
		return defaultValue
	}
	return val
}

// Close shuts the OpenFeature provider / Flipt stream down as part of resource
// teardown. Safe to call on a disabled client.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || !c.enabled {
		return nil
	}
	openfeature.Shutdown()
	return nil
}
