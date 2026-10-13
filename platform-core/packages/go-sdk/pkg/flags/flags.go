package flags

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	flipt "github.com/open-feature/go-sdk-contrib/providers/flipt/pkg/provider"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// warnEvery is the minimum gap between two evaluation-failure warnings for the
// same flag, so a down Flipt does not log once per request.
const warnEvery = time.Minute

// Evaluator is the surface handlers depend on, so tests can substitute a fake.
type Evaluator interface {
	// BooleanEnabled returns the flag value, or def on any error or timeout.
	// The targeting key defaults to the principal id in ctx; see EvalOption.
	BooleanEnabled(ctx context.Context, flag string, def bool, opts ...EvalOption) bool
}

// EvalOption adjusts the evaluation context of one BooleanEnabled call.
type EvalOption func(*evalOptions)

type evalOptions struct {
	targetingKey string
	attributes   map[string]any
}

// WithTargetingKey overrides the targeting key, which otherwise is the id of
// the principal in ctx.
func WithTargetingKey(key string) EvalOption {
	return func(o *evalOptions) { o.targetingKey = key }
}

// WithAttributes adds attributes to the evaluation context for targeting rules.
// Repeated options merge; a later value wins.
func WithAttributes(attrs map[string]any) EvalOption {
	return func(o *evalOptions) {
		if o.attributes == nil {
			o.attributes = make(map[string]any, len(attrs))
		}
		for k, v := range attrs {
			o.attributes[k] = v
		}
	}
}

// evalContext builds the OpenFeature evaluation context: the targeting key is
// the principal id from ctx unless overridden, and "anonymous" without one.
func evalContext(ctx context.Context, opts []EvalOption) openfeature.EvaluationContext {
	var o evalOptions
	for _, opt := range opts {
		opt(&o)
	}
	key := o.targetingKey
	if key == "" {
		key = auth.TypeAnonymous
		if p, ok := auth.FromContext(ctx); ok && p.ID != "" {
			key = p.ID
		}
	}
	attrs := o.attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return openfeature.NewEvaluationContext(key, attrs)
}

// Client is the OpenFeature-backed Evaluator. Each Client owns a private
// OpenFeature API instance, so closing one never affects another.
type Client struct {
	api     *openfeature.EvaluationAPI // nil when flags are disabled
	of      *openfeature.Client        // nil when flags are disabled
	logger  *slog.Logger
	timeout time.Duration

	mu       sync.Mutex
	lastWarn map[string]time.Time
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
	cl, err := newClient(ctx, provider, time.Duration(c.EvalTimeoutMS)*time.Millisecond, logger)
	if err != nil {
		return nil, err
	}
	logger.InfoContext(ctx, "feature flags enabled", "flipt_addr", c.FliptAddr)
	return cl, nil
}

// NewWithProvider builds a Client that evaluates through p. An evaluation
// error falls back to the caller's default. Closing the Client shuts p down.
func NewWithProvider(p openfeature.FeatureProvider, timeout time.Duration) (*Client, error) {
	return newClient(context.Background(), p, timeout, slog.Default())
}

func newClient(ctx context.Context, p openfeature.FeatureProvider, timeout time.Duration, logger *slog.Logger) (*Client, error) {
	api := isolated.NewAPI()
	if err := api.SetProviderAndWait(ctx, p); err != nil {
		_ = api.Shutdown(ctx)
		return nil, fmt.Errorf("flags: set provider: %w", err)
	}
	return &Client{api: api, of: api.NewClient(), logger: logger, timeout: timeout}, nil
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

func (disabled) BooleanEnabled(_ context.Context, _ string, def bool, _ ...EvalOption) bool {
	return def
}

// Disabled returns an Evaluator that always yields the caller's default.
func Disabled() Evaluator { return disabled{} }

// BooleanEnabled evaluates a boolean flag within the eval timeout, returning
// def on any error (fail open to the caller's default). The targeting key is the
// principal id in ctx ("anonymous" without one); opts can override it and add
// attributes. A failing evaluation is logged at most once a minute per flag.
func (c *Client) BooleanEnabled(ctx context.Context, flag string, def bool, opts ...EvalOption) bool {
	if c == nil || c.of == nil {
		return def
	}
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	val, err := c.of.BooleanValue(ctx, flag, def, evalContext(ctx, opts))
	if err != nil {
		if c.shouldWarn(flag) {
			c.logger.WarnContext(ctx, "feature flag evaluation failed; using default",
				"flag", flag, "default", def, "err", err)
		}
		return def
	}
	return val
}

// shouldWarn reports whether a failure for flag is due a log line now.
func (c *Client) shouldWarn(flag string) bool {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if last, ok := c.lastWarn[flag]; ok && now.Sub(last) < warnEvery {
		return false
	}
	if c.lastWarn == nil {
		c.lastWarn = make(map[string]time.Time)
	}
	c.lastWarn[flag] = now
	return true
}

// Close shuts down this client's provider only; other clients and the global
// OpenFeature API are untouched. Safe on a disabled client.
func (c *Client) Close(ctx context.Context) error {
	if c == nil || c.api == nil {
		return nil
	}
	return c.api.Shutdown(ctx)
}

// NewOrDisabled builds the Flipt-backed Evaluator, or, when setup fails, logs
// a warning and returns Disabled() so the service starts with caller defaults.
// The returned closer is never nil and is safe to call in either case.
func NewOrDisabled(ctx context.Context, c config.Flags, logger *slog.Logger) (Evaluator, func(context.Context) error) {
	if logger == nil {
		logger = slog.Default()
	}
	cl, err := New(ctx, c, logger)
	if err != nil {
		logger.WarnContext(ctx, "feature flags setup failed; evaluations return defaults", "err", err)
		return Disabled(), func(context.Context) error { return nil }
	}
	return cl, cl.Close
}
