package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"

	eventsv1 "github.com/buidangphuc/team-referral/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-referral/generated/platform/identity/v1"
	orderv1 "github.com/buidangphuc/team-referral/generated/platform/order/v1"
	"github.com/buidangphuc/team-referral/internal/store"
)

const (
	typeUserRegistered  = "platform.identity.v1.UserRegistered"
	typeOrderCompleted  = "platform.order.v1.OrderCompleted"
	typeReturnRequested = "platform.order.v1.ReturnRequested"
)

type Config struct {
	RewardXU    int64
	LifetimeCap int
	Now         func() time.Time
	Logger      *slog.Logger
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

func decode(r consume.Record) (*eventsv1.EventEnvelope, error) {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(r.Value, &env); err != nil {
		return nil, consume.Permanent(fmt.Errorf("decode envelope: %w", err))
	}
	return &env, nil
}

func unpack(env *eventsv1.EventEnvelope, into proto.Message) error {
	if env.GetEventId() == "" {
		return consume.Permanent(errors.New("envelope has no event id"))
	}
	if err := proto.Unmarshal(env.GetPayload(), into); err != nil {
		return consume.Permanent(fmt.Errorf("decode %s: %w", env.GetType(), err))
	}
	return nil
}

func timeOr(ts *timestamppb.Timestamp, fallback time.Time) time.Time {
	if ts.IsValid() {
		return ts.AsTime()
	}
	return fallback
}

func required(field, value string) error {
	if value == "" {
		return consume.Permanent(fmt.Errorf("%s is required", field))
	}
	return nil
}

func NewIdentityHandler(repo store.ReferralRepository, cfg Config) consume.Handler {
	return func(ctx context.Context, rec consume.Record) error {
		env, err := decode(rec)
		if err != nil {
			return err
		}
		if env.GetType() != typeUserRegistered {
			return nil
		}
		var ev identityv1.UserRegistered
		if err := unpack(env, &ev); err != nil {
			return err
		}
		if err := required("user_id", ev.GetUserId()); err != nil {
			return err
		}
		now := cfg.now()
		err = repo.ApplyUserRegistered(ctx, store.UserRegisteredInput{
			EventID:      env.GetEventId(),
			UserID:       ev.GetUserId(),
			ReferralCode: ev.GetReferralCode(),
			RegisteredAt: timeOr(ev.GetRegisteredAt(), timeOr(env.GetOccurredAt(), now)),
			At:           now,
			LifetimeCap:  cfg.LifetimeCap,
		})
		if err != nil {
			return err
		}
		cfg.logger().DebugContext(ctx, "user registered event applied", "key_fp", logx.PrincipalFP(ev.GetUserId()), "has_code", ev.GetReferralCode() != "")
		return nil
	}
}

func NewOrderHandler(repo store.ReferralRepository, cfg Config) consume.Handler {
	return func(ctx context.Context, rec consume.Record) error {
		env, err := decode(rec)
		if err != nil {
			return err
		}
		switch env.GetType() {
		case typeOrderCompleted:
			return applyOrderCompleted(ctx, repo, cfg, env)
		case typeReturnRequested:
			return applyReturnRequested(ctx, repo, cfg, env)
		}
		return nil
	}
}

func applyOrderCompleted(ctx context.Context, repo store.ReferralRepository, cfg Config, env *eventsv1.EventEnvelope) error {
	var ev orderv1.OrderCompleted
	if err := unpack(env, &ev); err != nil {
		return err
	}
	if err := errors.Join(required("order_id", ev.GetOrderId()), required("buyer_id", ev.GetBuyerId())); err != nil {
		return consume.Permanent(err)
	}
	if !ev.GetReturnWindowEndsAt().IsValid() {
		return consume.Permanent(errors.New("return_window_ends_at is required"))
	}
	now := cfg.now()
	err := repo.ApplyOrderCompleted(ctx, store.OrderCompletedInput{
		EventID:     env.GetEventId(),
		OrderID:     ev.GetOrderId(),
		BuyerID:     ev.GetBuyerId(),
		CompletedAt: timeOr(ev.GetCompletedAt(), timeOr(env.GetOccurredAt(), now)),
		ReleaseAt:   ev.GetReturnWindowEndsAt().AsTime(),
		At:          now,
		RewardXU:    cfg.RewardXU,
		LifetimeCap: cfg.LifetimeCap,
	})
	if err != nil {
		return err
	}
	cfg.logger().DebugContext(ctx, "order completed event applied", "key_fp", logx.PrincipalFP(ev.GetBuyerId()))
	return nil
}

func applyReturnRequested(ctx context.Context, repo store.ReferralRepository, cfg Config, env *eventsv1.EventEnvelope) error {
	var ev orderv1.ReturnRequested
	if err := unpack(env, &ev); err != nil {
		return err
	}
	if err := required("order_id", ev.GetOrderId()); err != nil {
		return err
	}
	err := repo.ApplyReturnRequested(ctx, store.ReturnRequestedInput{
		EventID: env.GetEventId(),
		OrderID: ev.GetOrderId(),
		At:      cfg.now(),
	})
	if err != nil {
		return err
	}
	cfg.logger().DebugContext(ctx, "return requested event applied", "key_fp", logx.PrincipalFP(ev.GetBuyerId()))
	return nil
}
