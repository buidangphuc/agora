// Package deps pins the modules later messaging packets import, so go.mod
// stays stable while they land. It is deleted once every module has a real importer.
package deps

import (
	// Blank imports keep the modules in go.mod until packets 4 and 5 import them.
	_ "github.com/open-feature/go-sdk-contrib/providers/flipt/pkg/provider"
	_ "github.com/open-feature/go-sdk/openfeature"
	_ "github.com/testcontainers/testcontainers-go/modules/redpanda"
	_ "github.com/twmb/franz-go/pkg/kgo"
	_ "github.com/twmb/franz-go/plugin/kotel"
)
