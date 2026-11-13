package rpc

import "github.com/buidangphuc/team-analytics/internal/forecaststore"

// STUB (T2 red): the implementer replaces this file by wiring the store into Handler.
func WithForecastStore(_ *forecaststore.Store) Option { return func(*Handler) {} }
