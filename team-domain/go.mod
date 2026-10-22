module github.com/buidangphuc/team-domain

go 1.26.0

require (
	github.com/google/uuid v1.6.0
	// Postgres — this service OWNS its database (Rule 3 / ADR). pgx is the driver
	// and pgxpool the connection pool used by the repository layer.
	github.com/jackc/pgx/v5 v5.9.0

	// Kafka producer (ADR-0002) via franz-go. Pinned to a Go-1.22-compatible
	// release (newer tags require Go 1.25). + uuid for event ids.
	github.com/twmb/franz-go v1.18.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.53.0

	// OpenTelemetry: API + SDK + gRPC instrumentation + OTLP/gRPC exporter.
	// Exporter is swappable (stdout, OTLP/http, ...) — see ADR-0004.
	go.opentelemetry.io/otel v1.44.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0
	go.opentelemetry.io/otel/sdk v1.44.0
	// gRPC transport + server (health, reflection live in subpackages).
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.11
)

require golang.org/x/text v0.42.0

require (
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.18.7 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.9.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
)

// Indirect dependencies and go.sum are produced by `go mod tidy` on first build.
