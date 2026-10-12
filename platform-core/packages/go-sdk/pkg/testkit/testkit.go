package testkit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	postgresImage = "postgres:17-alpine"
	redpandaImage = "docker.redpanda.com/redpandadata/redpanda:v24.3.1"
)

// Postgres starts a dedicated Postgres container for the test and returns its
// connection URL. The container is terminated when the test ends.
// Without Docker the test is skipped, unless GO_SDK_REQUIRE_DOCKER is set, which makes it fail.
func Postgres(t testing.TB) string {
	t.Helper()
	ctx := context.Background()

	requireDocker(t)

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

// Redpanda starts a dedicated single-node Redpanda container for the test and
// returns its Kafka seed broker address (host:port). The broker auto-creates topics,
// but franz-go clients must opt in with kgo.AllowAutoTopicCreation. Topics are created on first
// use. The container is terminated when the test ends. Docker handling matches Postgres.
func Redpanda(t testing.TB) (brokers string) {
	t.Helper()
	ctx := context.Background()

	requireDocker(t)

	ctr, err := tcredpanda.Run(ctx, redpandaImage, tcredpanda.WithAutoCreateTopics())
	if ctr != nil {
		t.Cleanup(func() {
			if err := testcontainers.TerminateContainer(ctr); err != nil {
				t.Logf("terminate redpanda container: %v", err)
			}
		})
	}
	if err != nil {
		t.Fatalf("start redpanda container: %v", err)
	}

	brokers, err = ctr.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatalf("redpanda seed broker: %v", err)
	}
	waitProducible(t, brokers)
	return brokers
}

// waitProducible blocks until the broker leads a freshly auto-created topic.
// The container is "ready" before its first partition leader is elected, and a
// produce in that window fails with UNKNOWN_TOPIC_OR_PARTITION.
func waitProducible(t testing.TB, brokers string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		err := probeProduce(brokers)
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("redpanda not producible: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// probeProduce uses a fresh client per attempt: a client that saw
// UNKNOWN_TOPIC_OR_PARTITION once can stay stuck on that topic.
func probeProduce(brokers string) error {
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.AllowAutoTopicCreation())
	if err != nil {
		return err
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.ProduceSync(ctx, &kgo.Record{Topic: "testkit.ready"}).FirstErr()
}

func requireDocker(t testing.TB) {
	t.Helper()
	if err := dockerHealthy(context.Background()); err != nil {
		if os.Getenv("GO_SDK_REQUIRE_DOCKER") == "" {
			t.Skipf("docker unavailable: %v", err)
		}
		t.Fatalf("docker unavailable and GO_SDK_REQUIRE_DOCKER is set: %v", err)
	}
}

func dockerHealthy(ctx context.Context) error {
	p, err := testcontainers.NewDockerProvider()
	if err != nil {
		return err
	}
	defer p.Close()
	return p.Health(ctx)
}
