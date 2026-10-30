package events

import (
	"context"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
)

const consentVersionCurrent = 1

type RedactingDLQ struct {
	next   consume.DLQProducer
	fp     *kafkax.Fingerprinter
	logger *slog.Logger
}

func NewRedactingDLQ(next consume.DLQProducer, fp *kafkax.Fingerprinter, logger *slog.Logger) *RedactingDLQ {
	return &RedactingDLQ{next: next, fp: fp, logger: logger}
}

func (d *RedactingDLQ) PublishRecord(ctx context.Context, r *kgo.Record) error {
	return d.next.PublishRecord(ctx, d.redact(r))
}

func (d *RedactingDLQ) redact(r *kgo.Record) *kgo.Record {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(r.Value, &env); err != nil || env.GetType() != TrackingEventType {
		return r
	}
	var te analyticsv1.TrackingEvent
	if err := proto.Unmarshal(env.GetPayload(), &te); err != nil {
		d.logger.Warn("dead-letter payload dropped", "key_fp", d.fp.Of(r.Key), "err", err)
		return d.withPayload(r, &env, nil)
	}
	consent := te.GetConsent()
	if te.GetGaClientId() == "" || (consent.GetVersion() == consentVersionCurrent && consent.GetAnalytics()) {
		return r
	}
	te.GaClientId = ""
	payload, err := proto.Marshal(&te)
	if err != nil {
		d.logger.Warn("dead-letter payload dropped", "key_fp", d.fp.Of(r.Key), "err", err)
		payload = nil
	}
	return d.withPayload(r, &env, payload)
}

func (d *RedactingDLQ) withPayload(r *kgo.Record, env *eventsv1.EventEnvelope, payload []byte) *kgo.Record {
	env.Payload = payload
	value, err := proto.Marshal(env)
	if err != nil {
		d.logger.Warn("dead-letter value dropped", "key_fp", d.fp.Of(r.Key), "err", err)
		value = nil
	}
	out := *r
	out.Value = value
	return &out
}
