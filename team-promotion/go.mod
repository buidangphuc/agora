module github.com/buidangphuc/team-promotion

go 1.26.0

require (
	github.com/jackc/pgx/v5 v5.11.0
	// OpenFeature Go SDK — vendor-neutral flag evaluation API + in-memory test provider.
	github.com/open-feature/go-sdk v1.19.0
	// Kafka producer (ADR-0002) via franz-go — the house Kafka client already used
	// by team-order/team-domain/team-search. Pinned to the same Go-1.22 line.
	github.com/twmb/franz-go v1.22.1
	// ASSUMED Flipt OpenFeature provider (mirrors team-order): package path/version
	// and the streaming/in-process eval config must be confirmed at apply time.
	go.flipt.io/flipt-openfeature-provider v0.2.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.72.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.2 // indirect
	github.com/caarlos0/env/v11 v11.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/grpc-ecosystem/grpc-gateway v1.16.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.30 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.14.0 // indirect
	go.flipt.io/flipt/errors v1.45.0 // indirect
	go.flipt.io/flipt/rpc/flipt v1.54.0 // indirect
	go.flipt.io/flipt/sdk/go v0.13.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/exp v0.0.0-20240506185415-9bf2ced13842 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto v0.0.0-20230526161137-0005af68ea54 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
