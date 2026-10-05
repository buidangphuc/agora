package revocation

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// RegisterMetrics exposes the consumer's health on the gateway's OTel meter
// (ADR-0004; a no-op provider unless OTEL_ENABLED=true): an up flag (1 when Kafka is
// reachable), the consumer lag in records, and the denylist size. Alert on
// `gateway_revocation_consumer_up == 0` — it means revocations are not enforced.
func RegisterMetrics(c *Consumer) error {
	m := otel.Meter("team-gateway")
	up, err := m.Int64ObservableGauge("gateway_revocation_consumer_up",
		metric.WithDescription("1 when the identity.events consumer can reach Kafka, else 0 (revocations not enforced)"))
	if err != nil {
		return err
	}
	lag, err := m.Int64ObservableGauge("gateway_revocation_consumer_lag",
		metric.WithDescription("Records the identity.events consumer is behind the high watermark"))
	if err != nil {
		return err
	}
	size, err := m.Int64ObservableGauge("gateway_revocation_denylist_size",
		metric.WithDescription("Revoked, unexpired session ids held in memory"))
	if err != nil {
		return err
	}
	_, err = m.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		s := c.Stats()
		var upV int64
		if s.Up {
			upV = 1
		}
		o.ObserveInt64(up, upV)
		o.ObserveInt64(lag, s.Lag)
		o.ObserveInt64(size, int64(s.DenylistSize))
		return nil
	}, up, lag, size)
	return err
}
