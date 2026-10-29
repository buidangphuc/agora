package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/app"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

const modulePath = "github.com/buidangphuc/team-analytics"

var errDuckDBDown = errors.New("duckdb down")

func failingPing(context.Context, *sql.DB) error { return errDuckDBDown }

func TestBinariesLinkNoInMemoryImplementation(t *testing.T) {
	cmd := exec.Command("go", "list", "-e", "-deps", "-json", "./cmd/consumer", "./cmd/ga4-relay", "./cmd/ga4import")
	cmd.Dir = serviceRoot(t)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(t, err, stderr.String())

	type pkg struct {
		ImportPath string
		Dir        string
		GoFiles    []string
		Error      *struct{ Err string }
	}
	forbidden := regexp.MustCompile(`^(NewMemory|NewInMemory|Noop)`)
	dec := json.NewDecoder(bytes.NewReader(out))
	linksApp := false
	for dec.More() {
		var p pkg
		require.NoError(t, dec.Decode(&p))
		if !strings.HasPrefix(p.ImportPath, modulePath) {
			continue
		}
		require.Nil(t, p.Error, "package %s does not resolve: %v", p.ImportPath, p.Error)
		if p.ImportPath == modulePath+"/internal/app" {
			linksApp = true
		}
		fset := token.NewFileSet()
		for _, name := range p.GoFiles {
			f, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, parser.SkipObjectResolution)
			require.NoError(t, err)
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					assert.False(t, forbidden.MatchString(d.Name.Name), "%s declares %s", p.ImportPath, d.Name.Name)
				case *ast.GenDecl:
					for _, s := range d.Specs {
						switch s := s.(type) {
						case *ast.TypeSpec:
							assert.False(t, forbidden.MatchString(s.Name.Name), "%s declares type %s", p.ImportPath, s.Name.Name)
						case *ast.ValueSpec:
							for _, n := range s.Names {
								assert.False(t, forbidden.MatchString(n.Name), "%s declares %s", p.ImportPath, n.Name)
							}
						}
					}
				}
			}
		}
	}
	assert.True(t, linksApp, "the binaries must link internal/app")
}

func TestBootFailsWhenWarehouseCannotOpen(t *testing.T) {
	s := baseSettings(t, closedBroker(t), nil)
	s.Warehouse.DuckDBPath = filepath.Join(t.TempDir(), "missing-dir", "analytics.duckdb")
	p := startProc(t, app.RunConsumer, s)
	err := p.result(t, 10*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open warehouse")
	notListening(t, s.GRPC.Addr())
}

func TestUnknownMockAdapterRefused(t *testing.T) {
	s := baseSettings(t, closedBroker(t), nil)
	s.MockData.Adapters = "bigquery"
	p := startProc(t, app.RunConsumer, s)
	err := p.result(t, 10*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bigquery")
	assert.Contains(t, err.Error(), "ga4export")
	notListening(t, s.GRPC.Addr())
}

func TestEmptyLogKeyFPSecretRefusesBoot(t *testing.T) {
	s := baseSettings(t, closedBroker(t), nil)
	s.LogKeyFP.Secret = ""
	p := startProc(t, app.RunConsumer, s)
	require.Error(t, p.result(t, 10*time.Second))
	notListening(t, s.GRPC.Addr())
}

func TestWarehouseOutageFlipsReadinessNotLiveness(t *testing.T) {
	app.SetHooks(t, app.Hooks{PingDuckDB: failingPing, HealthInterval: 200 * time.Millisecond})
	s := baseSettings(t, closedBroker(t), nil)
	p := startProc(t, app.RunConsumer, s)
	conn := waitUp(t, p)
	requireStatusWithin(t, conn, "", grpc_health_v1.HealthCheckResponse_NOT_SERVING, healthWindow)
	requireStatusHolds(t, conn, "liveness", grpc_health_v1.HealthCheckResponse_SERVING, 600*time.Millisecond)
}

func TestKafkaOutageDoesNotFlipReadiness(t *testing.T) {
	app.SetHooks(t, app.Hooks{HealthInterval: 200 * time.Millisecond})
	s := baseSettings(t, closedBroker(t), nil)
	p := startProc(t, app.RunConsumer, s)
	conn := waitUp(t, p)
	requireStatusHolds(t, conn, "", grpc_health_v1.HealthCheckResponse_SERVING, 2*time.Second)
	requireStatusHolds(t, conn, "liveness", grpc_health_v1.HealthCheckResponse_SERVING, 200*time.Millisecond)
}

func TestDLQProducersAreRedacting(t *testing.T) {
	type built struct {
		role string
		p    consume.DLQProducer
	}
	seen := make(chan built, 8)
	app.SetHooks(t, app.Hooks{DLQProducer: func(role string, p consume.DLQProducer) { seen <- built{role, p} }})

	cons := startProc(t, app.RunConsumer, baseSettings(t, closedBroker(t), nil))
	relay := startProc(t, app.RunGA4Relay, baseSettings(t, closedBroker(t), map[string]string{
		"GA4_API_SECRET":     "test-secret",
		"GA4_MEASUREMENT_ID": "G-TEST",
	}))
	_, _ = cons, relay

	got := map[string]consume.DLQProducer{}
	deadline := time.After(20 * time.Second)
	for len(got) < 2 {
		select {
		case b := <-seen:
			got[b.role] = b.p
		case <-deadline:
			t.Fatalf("dead-letter producers built for %v, want consumer and relay", keys(got))
		}
	}
	for role, p := range got {
		assert.IsType(t, &events.RedactingDLQ{}, p, "role %s", role)
	}
}

func keys(m map[string]consume.DLQProducer) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestShutdownDrainsHealthBeforeServerStops(t *testing.T) {
	app.SetHooks(t, app.Hooks{HealthInterval: 200 * time.Millisecond})
	s := baseSettings(t, closedBroker(t), map[string]string{"SHUTDOWN_DRAIN_DELAY": "2s"})
	p := startProc(t, app.RunConsumer, s)
	conn := waitUp(t, p)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := grpc_health_v1.NewHealthClient(conn).Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	first, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, first.GetStatus())

	p.cancel()
	for {
		msg, err := stream.Recv()
		require.NoError(t, err, "watch ended before NOT_SERVING was delivered")
		if msg.GetStatus() == grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			break
		}
	}
	select {
	case <-p.finished:
		t.Fatal("root returned before the drain delay elapsed: server stopped before health drained")
	default:
	}
	require.NoError(t, p.result(t, 30*time.Second))
}

func TestNonDuckDBWarehouseServesHealthOnly(t *testing.T) {
	w := &recWriter{}
	app.SetHooks(t, app.Hooks{
		OpenWarehouse: func(context.Context, config.Settings) (warehouse.WarehouseWriter, error) { return w, nil },
	})
	s := baseSettings(t, closedBroker(t), map[string]string{
		"WAREHOUSE_DRIVER":   config.DriverBigQuery,
		"BIGQUERY_PROJECT":   "test-project",
		"GA4_EXPORT_DATASET": "",
	})
	p := startProc(t, app.RunConsumer, s)
	conn := waitUp(t, p)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := analyticsv1.NewAnalyticsQueryServiceClient(conn).GetSellerFunnel(ctx, &analyticsv1.GetSellerFunnelRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	st, err := healthOf(conn, "")
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, st)
}
