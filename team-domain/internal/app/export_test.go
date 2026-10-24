package app

import (
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
)

func SetPurgeInterval(t interface{ Cleanup(func()) }, d time.Duration) {
	prev := purgeInterval
	purgeInterval = d
	t.Cleanup(func() { purgeInterval = prev })
}

func SetDLQWrap(t interface{ Cleanup(func()) }, f func(consume.DLQProducer) consume.DLQProducer) {
	prev := wrapDLQ
	wrapDLQ = f
	t.Cleanup(func() { wrapDLQ = prev })
}
