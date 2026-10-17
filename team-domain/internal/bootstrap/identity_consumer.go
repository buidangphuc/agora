package bootstrap

import (
	"context"
	"errors"
	"log/slog"

	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/consumer"
	"github.com/buidangphuc/team-domain/internal/repository"
)

// identityConsumerAddon runs the identity UserStatusChanged consumer when
// IDENTITY_CONSUMER_ENABLED is true. Disabled, it opens nothing.
type identityConsumerAddon struct {
	cons *consumer.Consumer
	stop context.CancelFunc
	done chan struct{}
}

func (a *identityConsumerAddon) Name() string { return "identity-consumer" }

func (a *identityConsumerAddon) Enabled(s *config.Settings) bool {
	return s.Events.IdentityConsumerEnabled
}

func (a *identityConsumerAddon) Open(_ context.Context, s *config.Settings, res *Resources) error {
	if res == nil || res.Pool == nil || res.Outbox == nil {
		return errors.New("identity consumer requires the database (writes seller holds)")
	}
	cons, err := consumer.New(s.KafkaBrokers(), s.Events.IdentityGroup, s.Events.IdentityTopic)
	if err != nil {
		return err
	}
	logger := slog.Default()
	h := consumer.NewIdentityConsumer(repository.NewPgTxWriter(res.Pool, res.Outbox), logger)
	runCtx, cancel := context.WithCancel(context.Background())
	a.cons, a.stop, a.done = cons, cancel, make(chan struct{})
	go func() {
		defer close(a.done)
		if err := cons.Run(runCtx, h.Handle, logger); err != nil && runCtx.Err() == nil {
			logger.Error("identity consumer stopped", slog.Any("err", err))
		}
	}()
	return nil
}

func (a *identityConsumerAddon) Close(_ context.Context, _ *Resources) error {
	if a.stop != nil {
		a.stop()
		<-a.done
		a.stop = nil
	}
	if a.cons != nil {
		a.cons.Close()
		a.cons = nil
	}
	return nil
}
