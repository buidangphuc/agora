package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	eventsv1 "github.com/buidangphuc/team-verification/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-verification/generated/platform/identity/v1"
	"github.com/buidangphuc/team-verification/internal/objects"
)

const userErasedType = "platform.identity.v1.UserErased"

type UserErasedConsumer struct {
	pool   *pgxpool.Pool
	objs   objects.Store
	logger *slog.Logger
}

func NewUserErasedConsumer(pool *pgxpool.Pool, objs objects.Store, logger *slog.Logger) *UserErasedConsumer {
	return &UserErasedConsumer{pool: pool, objs: objs, logger: logger}
}

func (c *UserErasedConsumer) Handle(ctx context.Context, rec consume.Record) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal envelope: %w", err))
	}
	if env.GetType() != userErasedType {
		return nil
	}
	var ev identityv1.UserErased
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		return consume.Permanent(fmt.Errorf("unmarshal UserErased: %w", err))
	}
	user := ev.GetUserId()
	if user == "" {
		return consume.Permanent(errors.New("empty user_id"))
	}
	return c.erase(ctx, user)
}

func (c *UserErasedConsumer) erase(ctx context.Context, user string) error {
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	keys, err := c.keys(ctx, tx, user)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := c.objs.Delete(ctx, k); err != nil {
			return fmt.Errorf("delete object: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM kyc_upload_grants WHERE user_id = $1`, user); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM kyc_submissions WHERE user_id = $1`, user); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c *UserErasedConsumer) keys(ctx context.Context, tx pgx.Tx, user string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT d.object_key FROM kyc_documents d JOIN kyc_submissions s ON s.id = d.submission_id
		WHERE s.user_id = $1 AND d.deleted_at IS NULL
		UNION
		SELECT object_key FROM kyc_upload_grants WHERE user_id = $1`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
