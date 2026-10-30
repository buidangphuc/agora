package app

import "testing"

type Hooks = testHooks

func SetHooks(t testing.TB, h Hooks) {
	t.Helper()
	prev := hooks
	hooks = h
	t.Cleanup(func() { hooks = prev })
}
