// Package deps blank-imports every module the SDK will use so that go mod tidy
// keeps them in go.mod until the packages that import them exist. Removed when
// the grpcx packet lands.
package deps

import (
	_ "github.com/caarlos0/env/v11"
	_ "github.com/failsafe-go/failsafe-go"
	_ "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/google/uuid"
	_ "github.com/grpc-ecosystem/go-grpc-middleware/v2"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/testcontainers/testcontainers-go"
	_ "github.com/testcontainers/testcontainers-go/modules/postgres"
	_ "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	_ "go.opentelemetry.io/otel"
	_ "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	_ "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	_ "go.opentelemetry.io/otel/sdk"
	_ "go.opentelemetry.io/otel/sdk/metric"
	_ "golang.org/x/sync/errgroup"
	_ "google.golang.org/grpc"
)
