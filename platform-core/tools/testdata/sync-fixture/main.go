// Command fixture is the minimal service the sync-go-sdk test vendors the SDK into.
package main

import (
	_ "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"

	fixturev1 "github.com/buidangphuc/sync-fixture/generated/fixture/v1"
)

func main() { _ = &fixturev1.Ping{} }
