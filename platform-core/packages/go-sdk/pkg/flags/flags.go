package flags

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	flipt "github.com/open-feature/go-sdk-contrib/providers/flipt/pkg/provider"
	"github.com/open-feature/go-sdk/openfeature"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// domain names the OpenFeature provider binding for this SDK.
const domain = "go-sdk"

// Evaluator is the surface handlers depend on, so tests can substitute a fake.
type Evaluator interface {
	// BooleanEnabled returns the flag value, or def on any error or timeout.
	BooleanEnabled(ctx context.Context, flag string, def bool) bool
}

// Client is the OpenFeature-backed Evaluator.
type Client struct {
	of      *openfeature.Client // nil when flags are disabled
	logger  *slog.Logger
	timeout time.Duration
}

var _ Evaluator = (*Client)(nil)

// New builds a Client. With c.Enabled false it never contacts Flipt and every
// evaluation returns the caller's default. A returned error means the provider
// could not be set up; callers may degrade to Disabled().
func New(ctx context.Context, c config.Flags, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if !c.Enabled {
		logger.InfoContext(ctx, "feature flags disabled; evaluations return defaults")
		return &Client{logger: logger}, nil
	}
	if c.FliptAddr == "" {
		return nil, errors.New("FLIPT_ADDR is required when feature flags are enabled")
	}
	provider := flipt.NewProvider(flipt.WithAddress(dialTarget(c.FliptAddr)))
	if err := openfeature.SetNamedProviderAndWait(domain, provider); err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "feature flags enabled", "flipt_addr", c.FliptAddr)
	return &Client{
		of:      openfeature.NewClient(domain),
		logger:  logger,
		timeout: time.Duration(c.EvalTimeoutMS) * time.Millisecond,
	}, nil
}

// dialTarget makes a bare host:port parseable by the provider, which runs
// url.Parse on the address: "127.0.0.1:1" would fail there. A scheme-less
// address is gRPC, so give it the dns resolver scheme.
func dialTarget(addr string) string {
	if strings.Contains(addr, "://") {
		return addr
	}
	return "dns:///" + addr
}

type disabled struct{}

func (disabled) BooleanEnabled(_ context.Context, _ string, def bool) bool { return def }

// Disabled returns an Evaluator that always yields the caller's default.
func Disabled() Evaluator { return disabled{} }

// BooleanEnabled evaluates a boolean flag within the eval timeout, returning
// def on any error.
func (c *Client) BooleanEnabled(ctx context.Context, flag string, def bool) bool {
	if c == nil || c.of == nil {
		return def
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	val, err := c.of.BooleanValue(ctx, flag, def, openfeature.NewEvaluationContext("system", map[string]any{}))
	if err != nil {
		c.logger.WarnContext(ctx, "feature flag evaluation failed; using default",
			"flag", flag, "default", def, "err", err)
		return def
	}
	return val
}

// Close shuts the provider down. Safe on a disabled client.
func (c *Client) Close(_ context.Context) error {
	if c == nil || c.of == nil {
		return nil
	}
	openfeature.Shutdown()
	return nil
}
