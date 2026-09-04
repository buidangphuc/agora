module github.com/buidangphuc/team-gateway

go 1.22

require (
	// Connect serves gRPC + gRPC-web + JSON from one handler (the edge).
	connectrpc.com/connect v1.16.2

	// JWT verification at the edge (ADR-0003).
	github.com/golang-jwt/jwt/v5 v5.2.1

	// Kafka producer for edge telemetry (ADR-0002) via franz-go. Pinned to a
	// Go-1.22-compatible release (matches team-domain's producer).
	github.com/google/uuid v1.6.0

	// Edge hardening: CORS + per-key rate limiting. Pinned to Go-1.22-compatible
	// releases (newer x/time requires Go 1.25).
	github.com/rs/cors v1.11.0
	github.com/twmb/franz-go v1.18.0

	// OpenTelemetry (ADR-0004; tracer + meter wiring, exporter swappable).
	// otelgrpc pins to contrib v0.53.0, the release paired with otel core v1.28.0.
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.53.0
	go.opentelemetry.io/otel v1.28.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.28.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.28.0
	go.opentelemetry.io/otel/metric v1.28.0 // indirect
	go.opentelemetry.io/otel/sdk v1.28.0
	go.opentelemetry.io/otel/sdk/metric v1.28.0

	// h2c: serve HTTP/2 (gRPC) over cleartext for local/dev.
	golang.org/x/net v0.26.0
	golang.org/x/time v0.5.0

	// gRPC client to upstream services + protobuf runtime.
	google.golang.org/grpc v1.66.0
	google.golang.org/protobuf v1.34.2
)

require connectrpc.com/grpcreflect v1.3.0

require (
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.20.0 // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/pierrec/lz4/v4 v4.1.21 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.9.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.28.0 // indirect
	go.opentelemetry.io/otel/trace v1.28.0 // indirect
	go.opentelemetry.io/proto/otlp v1.3.1 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240701130421-f6361c86f094 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240701130421-f6361c86f094 // indirect
)

// Indirect dependencies and go.sum are produced by `go mod tidy` on first build.
