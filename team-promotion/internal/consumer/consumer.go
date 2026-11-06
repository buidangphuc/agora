package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	eventsv1 "github.com/buidangphuc/team-promotion/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-promotion/generated/platform/identity/v1"
	referralv1 "github.com/buidangphuc/team-promotion/generated/platform/referral/v1"
)

const (
	typeUserRegistered = "platform.identity.v1.UserRegistered"
	typeAttributed     = "platform.referral.v1.ReferralAttributed"
	typeRejected       = "platform.referral.v1.ReferralRejected"

	verdictAttributed = "attributed"
	verdictRejected   = "rejected"

	defaultSweepInterval = time.Minute
)

type Config struct {
	WelcomeAmount  int64
	InvitedAmount  int64
	MinSpend       int64
	VoucherTTL     time.Duration
	VerdictTimeout time.Duration
	SweepInterval  time.Duration
	Now            func() time.Time
	Logger         *slog.Logger
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

func required(field, value string) error {
	if value == "" {
		return consume.Permanent(fmt.Errorf("%s is required", field))
	}
	return nil
}

type userState struct {
	registered bool
	hasCode    bool
	verdict    string
	issued     bool
}

func markProcessed(ctx context.Context, tx pgx.Tx, eventID string, now time.Time) (bool, error) {
	ct, err := tx.Exec(ctx, `INSERT INTO promotion_processed_events (event_id, processed_at) VALUES ($1, $2) ON CONFLICT (event_id) DO NOTHING`, eventID, now)
	if err != nil {
		return false, fmt.Errorf("record processed event: %w", err)
	}
	return ct.RowsAffected() == 1, nil
}

func lockState(ctx context.Context, tx pgx.Tx, userID string, now time.Time) (userState, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO new_user_voucher_state (user_id, updated_at) VALUES ($1, $2) ON CONFLICT (user_id) DO NOTHING`, userID, now); err != nil {
		return userState{}, fmt.Errorf("create voucher state: %w", err)
	}
	var st userState
	err := tx.QueryRow(ctx, `SELECT registered, has_code, verdict, issued_at IS NOT NULL FROM new_user_voucher_state WHERE user_id = $1 FOR UPDATE`, userID).
		Scan(&st.registered, &st.hasCode, &st.verdict, &st.issued)
	if err != nil {
		return userState{}, fmt.Errorf("lock voucher state: %w", err)
	}
	return st, nil
}

func (c Config) amountFor(verdict string) int64 {
	if verdict == verdictAttributed {
		return c.InvitedAmount
	}
	return c.WelcomeAmount
}

func NewIdentityHandler(pool *pgxpool.Pool, cfg Config) consume.Handler {
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
		hasCode := ev.GetReferralCode() != ""
		err = postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
			fresh, err := markProcessed(ctx, tx, env.GetEventId(), now)
			if err != nil || !fresh {
				return err
			}
			st, err := lockState(ctx, tx, ev.GetUserId(), now)
			if err != nil || st.registered {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE new_user_voucher_state SET registered = true, has_code = $2, registered_at = $3, updated_at = $3 WHERE user_id = $1`,
				ev.GetUserId(), hasCode, now); err != nil {
				return fmt.Errorf("mark registered: %w", err)
			}
			if st.issued || (hasCode && st.verdict == "") {
				return nil
			}
			return issue(ctx, tx, cfg, ev.GetUserId(), cfg.amountFor(st.verdict), now)
		})
		if err != nil {
			return err
		}
		cfg.logger().DebugContext(ctx, "user registered event applied", "key_fp", logx.PrincipalFP(ev.GetUserId()), "has_code", hasCode)
		return nil
	}
}

func NewReferralHandler(pool *pgxpool.Pool, cfg Config) consume.Handler {
	return func(ctx context.Context, rec consume.Record) error {
		env, err := decode(rec)
		if err != nil {
			return err
		}
		var referee, verdict string
		switch env.GetType() {
		case typeAttributed:
			var ev referralv1.ReferralAttributed
			if err := unpack(env, &ev); err != nil {
				return err
			}
			referee, verdict = ev.GetRefereeId(), verdictAttributed
		case typeRejected:
			var ev referralv1.ReferralRejected
			if err := unpack(env, &ev); err != nil {
				return err
			}
			referee, verdict = ev.GetRefereeId(), verdictRejected
		default:
			return nil
		}
		if err := required("referee_id", referee); err != nil {
			return err
		}
		now := cfg.now()
		err = postgres.WithTx(ctx, pool, func(tx pgx.Tx) error {
			fresh, err := markProcessed(ctx, tx, env.GetEventId(), now)
			if err != nil || !fresh {
				return err
			}
			st, err := lockState(ctx, tx, referee, now)
			if err != nil || st.issued || st.verdict != "" {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE new_user_voucher_state SET verdict = $2, updated_at = $3 WHERE user_id = $1`, referee, verdict, now); err != nil {
				return fmt.Errorf("record verdict: %w", err)
			}
			if !st.registered || !st.hasCode {
				return nil
			}
			return issue(ctx, tx, cfg, referee, cfg.amountFor(verdict), now)
		})
		if err != nil {
			return err
		}
		cfg.logger().DebugContext(ctx, "referral verdict event applied", "key_fp", logx.PrincipalFP(referee), "verdict", verdict)
		return nil
	}
}
