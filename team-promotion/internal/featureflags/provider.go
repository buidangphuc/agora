package featureflags

import (
	"fmt"

	"github.com/open-feature/go-sdk/openfeature"
	flipt "go.flipt.io/flipt-openfeature-provider/pkg/provider/flipt"
)

// newFliptProvider builds the OpenFeature provider backed by Flipt.
//
// APPLY-TIME RISK (design.md "Provider package specifics"): the exact package and
// its streaming / in-process (in-memory snapshot) evaluation configuration must be
// confirmed at apply time. `go.flipt.io/flipt-openfeature-provider` is the ASSUMED
// package (pinned in go.mod with the same note).
//
// The required BEHAVIOR is non-negotiable and is what the design guarantees: the
// provider must keep an in-memory flag snapshot refreshed over a gRPC stream so a
// BooleanValue check is a local lookup (~0ms), NOT a per-request round-trip — safe
// on the checkout hot path. If delivering that requires the flipt-client engine
// (go.flipt.io/flipt-client) plus its OpenFeature provider instead of the
// server-eval provider below, swap ONLY this constructor; the Evaluator / Client /
// handler / bootstrap seams do not change.
func newFliptProvider(cfg Config) (openfeature.FeatureProvider, error) {
	if cfg.FliptAddr == "" {
		return nil, fmt.Errorf("FLIPT_ADDR is required when feature flags are enabled")
	}
	provider := flipt.NewProvider(
		flipt.WithAddress(cfg.FliptAddr),
	)
	return provider, nil
}
