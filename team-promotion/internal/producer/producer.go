// Package producer emits team-promotion state-change events to the
// promotion.events topic (ADR-0002). Voucher/campaign create/update are wrapped in
// an EventEnvelope carrying the causing principal and W3C traceparent so the async
// hop stays auditable and traceable.
//
// The payload (VoucherChanged / FlashSaleChanged) is proto-marshalled and wrapped
// in the generated platform.events.v1.EventEnvelope (ADR-0002); the whole envelope
// is proto-marshalled onto the topic, matching every other platform producer.
package producer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-promotion/generated/platform/events/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/interceptor"
)

// Fully-qualified proto names carried in the envelope's Type; consumers switch on
// these.
const (
	TypeVoucherChanged   = "platform.promotion.v1.VoucherChanged"
	TypeFlashSaleChanged = "platform.promotion.v1.FlashSaleChanged"
)

// Publisher is the transport seam: a keyed synchronous publish to promotion.events.
// bootstrap.EventProducer satisfies it; tests supply a capturing fake.
type Publisher interface {
	Publish(ctx context.Context, key string, value []byte) error
}

// Emitter builds envelopes and publishes them. A nil Emitter, or one over a nil
// Publisher (Kafka disabled), degrades to a no-op so the gRPC path never fails
// because events could not be published.
type Emitter struct {
	pub    Publisher
	logger *slog.Logger
	nowFn  func() time.Time
	idFn   func() string
}

// NewEmitter wraps a Publisher. Pass nil when Kafka is disabled — emits become
// no-ops.
func NewEmitter(pub Publisher, logger *slog.Logger) *Emitter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Emitter{pub: pub, logger: logger, nowFn: time.Now, idFn: newEventID}
}

// EmitVoucherChanged publishes a VoucherChanged event keyed by voucher id.
func (e *Emitter) EmitVoucherChanged(ctx context.Context, v *promotionv1.Voucher) error {
	if e == nil || e.pub == nil || v == nil {
		return nil
	}
	payload, err := proto.Marshal(&promotionv1.VoucherChanged{Voucher: v})
	if err != nil {
		return fmt.Errorf("marshal VoucherChanged: %w", err)
	}
	return e.emit(ctx, v.GetId(), TypeVoucherChanged, payload)
}

// EmitFlashSaleChanged publishes a FlashSaleChanged event keyed by campaign id.
func (e *Emitter) EmitFlashSaleChanged(ctx context.Context, c *promotionv1.FlashSaleCampaign) error {
	if e == nil || e.pub == nil || c == nil {
		return nil
	}
	payload, err := proto.Marshal(&promotionv1.FlashSaleChanged{Campaign: c})
	if err != nil {
		return fmt.Errorf("marshal FlashSaleChanged: %w", err)
	}
	return e.emit(ctx, c.GetId(), TypeFlashSaleChanged, payload)
}

func (e *Emitter) emit(ctx context.Context, key, eventType string, payload []byte) error {
	env := &eventsv1.EventEnvelope{
		EventId:     e.idFn(),
		Type:        eventType,
		OccurredAt:  timestamppb.New(e.nowFn().UTC()),
		Principal:   principalFromContext(ctx),
		Traceparent: firstIncomingMD(ctx, "traceparent"),
		RequestId:   firstIncomingMD(ctx, "x-request-id"),
		Payload:     payload,
	}
	value, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	return e.pub.Publish(ctx, key, value)
}

func principalFromContext(ctx context.Context) *commonv1.Principal {
	p, ok := interceptor.PrincipalFromContext(ctx)
	if !ok || p == nil || p.GetId() == "" {
		return nil
	}
	return p
}

func firstIncomingMD(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// newEventID returns a random 128-bit hex id for an event occurrence; consumers
// may dedupe on it.
func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a time-based id; an event id only needs to be unique enough
		// for dedupe and this path is effectively unreachable.
		return "evt-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b[:])
}
