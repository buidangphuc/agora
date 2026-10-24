package app

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	"github.com/buidangphuc/team-domain/internal/config"
)

var errNotImplemented = errors.New("app.Run: not implemented (TEST-STUB, replaced by task 1.6.2)")

var (
	purgeInterval = time.Hour
	wrapDLQ       = func(p consume.DLQProducer) consume.DLQProducer { return p }
)

func Run(context.Context, config.Settings) error { return errNotImplemented }

func Migrate(context.Context) error { return errNotImplemented }
