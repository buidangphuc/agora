package testkit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const postgresImage = "postgres:17-alpine"

// Postgres starts a dedicated Postgres container for the test and returns its
// connection URL. The container is terminated when the test ends.
// Without Docker the test is skipped, unless GO_SDK_REQUIRE_DOCKER is set, which makes it fail.
func Postgres(t testing.TB) string {
	t.Helper()
	ctx := context.Background()

	if err := dockerHealthy(ctx); err != nil {
		if os.Getenv("GO_SDK_REQUIRE_DOCKER") == "" {
			t.Skipf("docker unavailable: %v", err)
		}
		t.Fatalf("docker unavailable and GO_SDK_REQUIRE_DOCKER is set: %v", err)
	}

	ctr, err := tcpostgres.Run(ctx, postgresImage,
		tcpostgres.WithDatabase("test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp").WithStartupTimeout(90*time.Second)),
	)
	if ctr != nil {
		t.Cleanup(func() {
			if err := testcontainers.TerminateContainer(ctr); err != nil {
				t.Logf("terminate postgres container: %v", err)
			}
		})
	}
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	return url
}

func dockerHealthy(ctx context.Context) error {
	p, err := testcontainers.NewDockerProvider()
	if err != nil {
		return err
	}
	defer p.Close()
	return p.Health(ctx)
}
