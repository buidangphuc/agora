// TEMPLATE — rename this module before first commit.
//
// Pick the real repo path for your service, e.g.
//   github.com/your-org/listing-service
// and replace every occurrence of `github.com/your-org/team-service`
// across this repo (go.mod, imports, buf.gen.yaml go_package_prefix).
//
// The `generated/` import paths below only resolve AFTER you run `make proto`
// (see proto-vendor/README.md). Until then this module is compiling-shaped, not
// buildable — that's expected for a seed template.
module github.com/your-org/team-service

go 1.22

require (
	// gRPC transport + server (health, reflection live in subpackages).
	google.golang.org/grpc v1.66.0
	google.golang.org/protobuf v1.34.2

	// OpenTelemetry: API + SDK + gRPC instrumentation + OTLP/gRPC exporter.
	// Exporter is swappable (stdout, OTLP/http, ...) — see ADR-0004.
	go.opentelemetry.io/otel v1.28.0
	go.opentelemetry.io/otel/sdk v1.28.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.28.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.53.0
)

// NOTE: indirect dependencies and go.sum are produced by `go mod tidy` the
// first time you build. They are intentionally omitted from this seed.
