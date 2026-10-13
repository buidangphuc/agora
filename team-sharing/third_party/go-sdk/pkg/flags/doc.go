// Package flags evaluates boolean feature flags through OpenFeature with the
// Flipt provider. Every failure resolves to the caller's default, so a flag
// outage never becomes a service outage; the caller picks fail-open or
// fail-closed by the default it passes.
package flags
