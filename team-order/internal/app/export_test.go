package app

import "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"

func SetComponentHook(t interface{ Cleanup(func()) }, f func(name string, c lifecycle.Component) lifecycle.Component) {
	prev := wrapComponent
	wrapComponent = f
	t.Cleanup(func() { wrapComponent = prev })
}
