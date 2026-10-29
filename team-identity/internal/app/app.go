package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/otelx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"

	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/events"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/token"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

const (
	serviceName  = "team-identity"
	auditScope   = "audit.write"
	closeGrace   = 2 * time.Second
	jwksGrace    = 5 * time.Second
	seedTimeout  = 30 * time.Second
	purgeMessage = "outbox purge"
)

type built struct {
	pool      *pgxpool.Pool
	health    *healthx.Health
	relayer   *outbox.Relayer
	purgeTick func(context.Context) error
	run       func(context.Context) error
	closeAll  func(context.Context) error
}

func Run(ctx context.Context, s config.Settings) error {
	b, err := build(ctx, s, nil)
	if err != nil {
		return err
	}
	return b.run(ctx)
}

func build(ctx context.Context, s config.Settings, pub outbox.Publisher) (b *built, err error) {
	logger := logx.New(s.Runtime, serviceName)

	signer, err := token.NewSigner(s.JWT.PrivateKey, s.JWT.KID)
	if err != nil {
		return nil, fmt.Errorf("build token signer: %w", err)
	}
	if err := s.RequireNonDevSigningKey(signer.KID(), signer.PublicKey()); err != nil {
		return nil, err
	}

	var stack lifecycle.Stack
	defer func() {
		if err != nil {
			_ = stack.Close(context.WithoutCancel(ctx))
		}
	}()

	shutdownOTel, err := otelx.Init(ctx, s.OTel, serviceName)
	if err != nil {
		return nil, fmt.Errorf("otel: %w", err)
	}
	stack.Push(shutdownOTel)

	health, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		return nil, err
	}

	pool, err := postgres.Open(ctx, s.Postgres)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	stack.Push(func(context.Context) error { pool.Close(); return nil })
	health.Register("postgres", postgres.Checker(pool))

	if pub == nil {
		producer, err := kafkax.NewProducer(s.Kafka, kgo.AllowAutoTopicCreation())
		if err != nil {
			return nil, fmt.Errorf("kafka: %w", err)
		}
		stack.Push(func(context.Context) error { producer.Close(); return nil })
		pub = producer
	}

	fp, err := kafkax.NewFingerprinter(s.LogKeyFP.Secret)
	if err != nil {
		return nil, fmt.Errorf("log key fingerprint: %w", err)
	}

	outStore, err := store.Outbox(pool)
	if err != nil {
		return nil, err
	}

	auditConn, err := auditx.Dial(sdkconfig.Audit{Addr: s.Upstream.AuditAddr}, "service-"+serviceName, auditScope)
	if err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}
	if auditConn == nil {
		return nil, errors.New("audit: UPSTREAM_AUDIT_ADDR is required")
	}
	stack.Push(func(context.Context) error { return auditConn.Close() })
	gate := auditx.NewGate(upstream.NewAuditWriter(auditConn), logger)

	ttl := time.Duration(s.JWT.TTLSeconds) * time.Second
	users := store.NewPostgresUserRepository(pool)
	sessions := store.NewPostgresSessionRepository(pool)
	svc, err := identity.NewAuthService(users, sessions, signer, ttl, gate)
	if err != nil {
		return nil, err
	}

	if s.SeedAdmin.Enabled {
		sctx, cancel := context.WithTimeout(ctx, seedTimeout)
		if err := svc.EnsureAdmin(sctx, s.SeedAdmin.Username, s.SeedAdmin.Password); err != nil {
			logger.WarnContext(ctx, "seed admin failed", "err", err)
		}
		cancel()
	}

	srv := grpcx.NewServer(grpcx.ServerOptions{Config: s.GRPC, Logger: logger, Health: health})
	rpc.Register(srv, rpc.Handlers{
		Auth:    rpc.NewAuthHandler(svc).WithLogger(logger).WithExposeResetToken(s.PasswordReset.ExposeToken),
		Address: rpc.NewAddressHandler(store.NewPostgresAddressRepository(pool), logger),
		Session: rpc.NewSessionHandler(sessions, logger).WithTokenTTL(ttl),
		Profile: rpc.NewProfileHandler(users, logger),
		Admin:   rpc.NewAccountAdminHandler(svc, users, logger),
	})

	relayer, err := outbox.NewRelayer(outStore, pub, outbox.RelayerOptions{
		Topic:  s.Events.IdentityTopic,
		Config: s.Outbox,
		Logger: slog.New(events.RedactingHandler(logger.Handler(), fp)),
	})
	if err != nil {
		return nil, err
	}

	purgeTick := func(ctx context.Context) error {
		n, err := outStore.Purge(ctx, s.OutboxPurge.Retention)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			logger.WarnContext(ctx, purgeMessage+" failed; will retry")
		case n > 0:
			logger.InfoContext(ctx, purgeMessage+" done", "rows_purged", n)
		}
		return nil
	}
	purge, err := lifecycle.Every(s.OutboxPurge.Interval, purgeTick, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return nil, err
	}

	sweeper := identity.NewSessionSweeper(sessions, identity.SessionSweeperConfig{
		SessionRetention:      s.Retention.Session,
		LoginHistoryRetention: s.Retention.LoginHistory,
		Interval:              s.Retention.SweepInterval,
		BatchSize:             s.Retention.SweepBatchSize,
	}, logger)
	sweep, err := lifecycle.Every(s.Retention.SweepInterval, sweeper.Tick, lifecycle.RunAtStart(), lifecycle.WithLogger(logger))
	if err != nil {
		return nil, err
	}

	jwks := lifecycle.HTTP(&http.Server{
		Addr:              net.JoinHostPort(s.Host, strconv.Itoa(s.JWT.JWKSHTTPPort)),
		Handler:           token.JWKSHandler(token.PublicKey{Kid: signer.KID(), Key: signer.PublicKey()}),
		ReadHeaderTimeout: 5 * time.Second,
	}, jwksGrace)

	return &built{
		pool:      pool,
		health:    health,
		relayer:   relayer,
		purgeTick: purgeTick,
		closeAll:  stack.Close,
		run: func(ctx context.Context) error {
			return lifecycle.Run(ctx, lifecycle.Options{
				ShutdownGrace: s.ShutdownGrace + 2*closeGrace,
				CloseGrace:    closeGrace,
				DrainDelay:    s.DrainDelay,
				OnShutdown:    []func(){health.Drain},
				Closers:       []func(context.Context) error{stack.Close},
				Logger:        logger,
			}, health, jwks, lifecycle.StopAfter(grpcx.Serve(srv, s.GRPC), relayer, purge, sweep))
		},
	}, nil
}
