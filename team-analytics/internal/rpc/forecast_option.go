package rpc

import "github.com/buidangphuc/team-analytics/internal/forecaststore"

func WithForecastStore(st *forecaststore.Store) Option { return func(h *Handler) { h.forecasts = st } }
