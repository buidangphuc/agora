// Package app is the composition root: it builds every dependency in order,
// wires them together and runs them. Read top to bottom to see what runs.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/pgx"

	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/token"
)

const service = "team-identity"

// Options are the seams tests use; Run passes the zero value.
type Options struct {
	// Publisher receives the outbox rows. Default: a Kafka producer on KAFKA_BROKERS.
	Publisher outbox.Publisher
	// LogOut receives the service log. Default: os.Stdout.
	LogOut io.Writer
}

// Run wires the service and runs it until ctx ends (main cancels it on
// SIGINT/SIGTERM) or a component fails. Build order: logger, signing key,
// telemetry, health, database, publisher, domain, server. Everything opened goes
// on the stack; closers run in reverse order on shutdown and on an early failure.
//
// Shutdown order: health drains, SHUTDOWN_DRAIN_DELAY_SECONDS passes, the gRPC
// server stops, then the outbox relayer stops, then the publisher and the pool
// close.
//
// team-identity owns identity_db and cannot serve without it, so an unreachable
// database fails boot in every ENV. Migrations are not applied here: the deploy
// migrate job team-identity-migrate does it.
func Run(ctx context.Context, s config.Settings) error {
	return run(ctx, s, Options{})
}

func run(ctx context.Context, s config.Settings, o Options) error {
	out := o.LogOut
	if out == nil {
		out = os.Stdout
	}
	logger := logx.NewTo(out, s.Runtime, service)
	slog.SetDefault(logger)

	// Fail fast on an unusable or development signing key, before connecting to anything.
	signer, err := token.NewSigner(s.JWT.PrivateKey, s.JWT.KID)
	if err != nil {
		return fmt.Errorf("build token signer: %w", err)
	}
	if err := s.RequireNonDevSigningKey(signer.KID(), signer.PublicKey()); err != nil {
		return err
	}

	var stack lifecycle.Stack

	shutdownOTel, err := otelx.Init(ctx, s.OTel, service)
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	stack.Push(shutdownOTel)

	health := healthx.New(healthx.DefaultInterval, logger)

	pool, err := pgx.Open(ctx, s.Postgres)
	if err != nil {
		return errors.Join(fmt.Errorf("open database: %w", err), stack.Close(ctx))
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", pgx.Checker(pool))

	// SessionRevoked rows are written in the revoke transaction (see
	// store.PostgresSessionRepository) and published by the relayer below.
	publisher := o.Publisher
	if publisher == nil {
		producer, err := kafkax.NewProducer(s.Kafka)
		if err != nil {
			return errors.Join(fmt.Errorf("kafka producer: %w", err), stack.Close(ctx))
		}
		stack.Push(func(context.Context) error { producer.Close(); return nil })
		publisher = producer
	}

	users := store.NewPostgresUserRepository(pool)
	sessions := store.NewPostgresSessionRepository(pool)
	ttl := time.Duration(s.JWT.TTLSeconds) * time.Second
	authSvc := identity.NewAuthService(users, signer, ttl).WithSessions(sessions)

	// Seed the first admin only when explicitly enabled; the password comes from
	// SEED_ADMIN_PASSWORD (validated in config) and is never logged.
	if s.SeedAdmin.Enabled {
		if err := authSvc.EnsureAdmin(ctx, s.SeedAdmin.Username, s.SeedAdmin.Password); err != nil {
			logger.WarnContext(ctx, "seed admin", "username", s.SeedAdmin.Username, "err", err)
		}
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.Handlers{
		Auth:    rpc.NewAuthHandler(authSvc).WithExposeResetToken(s.PasswordReset.ExposeToken),
		Address: rpc.NewAddressHandler(store.NewPostgresAddressRepository(pool), logger),
		Session: rpc.NewSessionHandler(sessions, logger).WithTokenTTL(ttl),
		Profile: rpc.NewProfileHandler(users, logger),
	})

	relayer := outbox.NewRelayer(outbox.NewStore(pool, store.OutboxTable), publisher, outbox.RelayerOptions{
		Topic:  s.Events.IdentityTopic,
		Config: s.Outbox,
		Logger: logger,
	})

	return lifecycle.Run(ctx, lifecycle.Options{
		ShutdownGrace: s.ShutdownGrace(),
		DrainDelay:    s.DrainDelay(),
		OnShutdown:    []func(){health.Drain},
		Closers:       []func(context.Context) error{stack.Close},
		Logger:        logger,
	},
		// The relayer outlives the server so in-flight revokes that enqueue rows
		// are drained first.
		lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer),
		lifecycle.Func(health.Run),
		jwksServer(s, signer, logger),
	)
}
