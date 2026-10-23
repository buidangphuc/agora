package app

import (
	"context"
	"errors"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-chat/internal/config"
)

type Options struct {
	NewPublisher  func(sdkconfig.Kafka) (outbox.Publisher, error)
	PurgeInterval time.Duration
}

func Run(ctx context.Context, s config.Settings) error {
	return RunWithOptions(ctx, s, Options{})
}

func RunWithOptions(context.Context, config.Settings, Options) error {
	return errors.New("not implemented: task 1.6.1")
}
