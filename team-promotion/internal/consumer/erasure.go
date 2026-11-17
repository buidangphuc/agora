package consumer

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"

	identityv1 "github.com/buidangphuc/team-promotion/generated/platform/identity/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

const typeUserErased = "platform.identity.v1.UserErased"

func NewErasureHandler(pool *pgxpool.Pool, cfg Config) consume.Handler {
	repo := store.NewPostgresErasureRepository(pool)
	return func(ctx context.Context, rec consume.Record) error {
		env, err := decode(rec)
		if err != nil {
			return err
		}
		if env.GetType() != typeUserErased {
			return nil
		}
		var ev identityv1.UserErased
		if err := unpack(env, &ev); err != nil {
			return err
		}
		if err := required("user_id", ev.GetUserId()); err != nil {
			return err
		}
		if err := repo.EraseUser(ctx, ev.GetUserId(), cfg.now()); err != nil {
			return err
		}
		cfg.logger().DebugContext(ctx, "user erased event applied", "key_fp", logx.PrincipalFP(ev.GetUserId()))
		return nil
	}
}
