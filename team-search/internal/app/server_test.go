package app_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/app"
	"github.com/buidangphuc/team-search/internal/config"
)

const (
	closedDatabaseURL   = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"
	closedOpenSearchURL = "http://127.0.0.1:1"
	healthWindow        = healthx.DefaultInterval + 1*time.Second
	bootBound           = 10 * time.Second
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type stubDoc struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type osStub struct {
	failing atomic.Bool
	docs    []stubDoc

	mu     sync.Mutex
	bodies []string
}

func (s *osStub) searchBodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...)
}

func (s *osStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.failing.Load() {
		http.Error(w, `{"error":"down"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/":
		_, _ = io.WriteString(w, `{"version":{"number":"2.17.1"}}`)
	case r.Method == http.MethodHead && r.URL.Path == "/listings":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPut && r.URL.Path == "/listings/_mapping":
		_, _ = io.WriteString(w, `{"acknowledged":true}`)
	case strings.HasSuffix(r.URL.Path, "/_search"):
		raw, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(raw))
		s.mu.Unlock()
		s.search(w, raw)
	default:
		http.NotFound(w, r)
	}
}

func (s *osStub) search(w http.ResponseWriter, raw []byte) {
	body := string(raw)
	var picked []stubDoc
	if strings.Contains(body, "bool_prefix") {
		var req struct {
			Query struct {
				Bool struct {
					Must struct {
						MultiMatch struct {
							Query string `json:"query"`
						} `json:"multi_match"`
					} `json:"must"`
				} `json:"bool"`
			} `json:"query"`
		}
		_ = json.Unmarshal(raw, &req)
		prefix := strings.ToLower(req.Query.Bool.Must.MultiMatch.Query)
		for _, d := range s.docs {
			if d.Status == "published" && strings.HasPrefix(strings.ToLower(d.Title), prefix) {
				picked = append(picked, d)
			}
		}
	} else {
		for _, d := range s.docs {
			if strings.Contains(body, `"status":"`+d.Status+`"`) {
				picked = append(picked, d)
			}
		}
	}
	hits := make([]map[string]any, 0, len(picked))
	for _, d := range picked {
		hits = append(hits, map[string]any{"_id": d.ID, "_score": 1.0, "_source": d})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"hits": map[string]any{"total": map[string]any{"value": len(picked)}, "hits": hits},
	})
}

func startOpenSearch(t *testing.T) (*osStub, string) {
	t.Helper()
	stub := &osStub{docs: []stubDoc{
		{ID: "a1", Title: "iPhone 15 Pro", Status: "published"},
		{ID: "b2", Title: "iPad Air", Status: "draft"},
		{ID: "c3", Title: "iPhone 14", Status: "published"},
	}}
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	return stub, srv.URL
}

func freshURL(t *testing.T) string {
	t.Helper()
	return testkit.Schema(t, testkit.SharedPostgres(t))
}

func dial(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func tableExists(t *testing.T, url, table string) bool {
	t.Helper()
	var ok bool
	err := dial(t, url).QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)`, table).Scan(&ok)
	if err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	return ok
}

func setDatabaseAvailable(t *testing.T, url string, available bool) {
	t.Helper()
	admin, err := pgx.Connect(context.Background(), strings.Replace(url, "/test?", "/postgres?", 1))
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	qs := []string{"ALTER DATABASE test ALLOW_CONNECTIONS " + strconv.FormatBool(available)}
	if !available {
		qs = append(qs, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'test'")
	}
	for _, q := range qs {
		if _, err := admin.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func searchSettings(port int, dbURL, osURL string) config.Settings {
	return config.Settings{
		Runtime:    sdkconfig.Runtime{Env: "local", LogLevel: "info", LogJSON: true},
		GRPC:       sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres:   sdkconfig.Postgres{URL: dbURL, MaxConns: 5},
		OpenSearch: config.OpenSearch{URL: osURL, Index: "listings"},
		Retrieval:  config.Retrieval{ModelServerURL: "http://127.0.0.1:1", EnableHybridSearch: false},
	}
}

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	logs   *syncBuffer
}

func captureStdout(t *testing.T) *syncBuffer {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = pw
	buf := &syncBuffer{}
	readDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(buf, pr)
		close(readDone)
	}()
	t.Cleanup(func() {
		os.Stdout = orig
		_ = pw.Close()
		<-readDone
		_ = pr.Close()
	})
	return buf
}

func start(t *testing.T, s config.Settings) *running {
	t.Helper()
	logs := captureStdout(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{}), logs: logs}
	go func() { r.err = app.Run(ctx, s); close(r.done) }()
	var err error
	r.conn, err = grpc.NewClient("127.0.0.1:"+strconv.Itoa(s.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.conn.Close()
		cancel()
		select {
		case <-r.done:
		case <-time.After(20 * time.Second):
			t.Error("app did not stop")
		}
	})
	return r
}

func (r *running) stop(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(20 * time.Second):
		t.Fatal("app did not stop")
	}
}

func waitServing(t *testing.T, r *running) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil || resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		select {
		case <-r.done:
			t.Fatalf("app exited before serving: %v", r.err)
		default:
		}
		t.Fatalf("health = %v, %v, want SERVING", resp.GetStatus(), err)
	}
}

func healthStatus(r *running, service string) (grpc_health_v1.HealthCheckResponse_ServingStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
	if err != nil {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, err
	}
	return resp.GetStatus(), nil
}

func overall(r *running) grpc_health_v1.HealthCheckResponse_ServingStatus {
	st, _ := healthStatus(r, "")
	return st
}

func assertPortFree(t *testing.T, port int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

func awaitBootError(t *testing.T, r *running) error {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(bootBound):
		t.Fatalf("Run did not return within %v; it is serving or hanging", bootBound)
	}
	if r.err == nil {
		t.Fatal("Run returned nil, want a boot error")
	}
	return r.err
}

func principal(ctx context.Context, id, typ, scopes string) context.Context {
	return metadata.AppendToOutgoingContext(ctx,
		"x-principal-id", id, "x-principal-type", typ, "x-principal-scopes", scopes)
}

func callCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func startServing(t *testing.T) (*running, *osStub) {
	t.Helper()
	stub, osURL := startOpenSearch(t)
	r := start(t, searchSettings(testkit.FreePort(t), freshURL(t), osURL))
	waitServing(t, r)
	return r, stub
}

func TestRunFailsOnUnreachableDatabase(t *testing.T) {
	for _, env := range []string{"local", "production"} {
		t.Run(env, func(t *testing.T) {
			// scenario: An unreachable database fails boot in a local environment / in production
			_, osURL := startOpenSearch(t)
			s := searchSettings(testkit.FreePort(t), closedDatabaseURL, osURL)
			s.Env = env
			r := start(t, s)
			err := awaitBootError(t, r)
			if !strings.Contains(strings.ToLower(err.Error()), "postgres") {
				t.Fatalf("boot error %q does not wrap the postgres open failure", err)
			}
			assertPortFree(t, s.Port)
		})
	}
}

func TestRunFailsOnUnreachableOpenSearch(t *testing.T) {
	// scenario: An unreachable OpenSearch fails boot
	s := searchSettings(testkit.FreePort(t), freshURL(t), closedOpenSearchURL)
	r := start(t, s)
	err := awaitBootError(t, r)
	if !strings.Contains(strings.ToLower(err.Error()), "index") {
		t.Fatalf("boot error %q does not wrap the ensure-index failure", err)
	}
	assertPortFree(t, s.Port)
}

func TestReadinessFollowsDependencies(t *testing.T) {
	// scenario: A failing dependency turns readiness off
	r, stub := startServing(t)

	stub.failing.Store(true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return overall(r) == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})

	stub.failing.Store(false)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return overall(r) == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func TestLivenessSurvivesDependencyOutage(t *testing.T) {
	// scenario: Liveness stays serving while a dependency is down
	stub, osURL := startOpenSearch(t)
	url := freshURL(t)
	r := start(t, searchSettings(testkit.FreePort(t), url, osURL))
	waitServing(t, r)

	setDatabaseAvailable(t, url, false)
	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return overall(r) == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if st, err := healthStatus(r, "liveness"); err != nil || st != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v, %v while Postgres is down, want SERVING", st, err)
	}

	setDatabaseAvailable(t, url, true)
	stub.failing.Store(true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return overall(r) == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if st, err := healthStatus(r, "liveness"); err != nil || st != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v, %v while OpenSearch is down, want SERVING", st, err)
	}
}

func failedCheckers(logs string) [][]string {
	var out [][]string
	for _, line := range strings.Split(logs, "\n") {
		var rec struct {
			Msg    string   `json:"msg"`
			Failed []string `json:"failed"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && len(rec.Failed) > 0 {
			out = append(out, rec.Failed)
		}
	}
	return out
}

func TestReadinessCheckersArePostgresAndOpenSearch(t *testing.T) {
	// scenario: Readiness registers no Kafka checker
	stub, osURL := startOpenSearch(t)
	url := freshURL(t)
	r := start(t, searchSettings(testkit.FreePort(t), url, osURL))
	waitServing(t, r)

	stub.failing.Store(true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return overall(r) == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	stub.failing.Store(false)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return overall(r) == grpc_health_v1.HealthCheckResponse_SERVING
	})

	setDatabaseAvailable(t, url, false)
	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return overall(r) == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	setDatabaseAvailable(t, url, true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return overall(r) == grpc_health_v1.HealthCheckResponse_SERVING
	})

	seen := map[string]bool{}
	for _, names := range failedCheckers(r.logs.String()) {
		for _, n := range names {
			seen[n] = true
		}
	}
	if !seen["opensearch"] || !seen["postgres"] || len(seen) != 2 {
		t.Fatalf("failed checker names seen in the health log = %v, want exactly postgres and opensearch", seen)
	}
	if st, err := healthStatus(r, "kafka"); err == nil || status.Code(err) != codes.NotFound {
		t.Fatalf("health Check(kafka) = %v, %v, want NotFound (no kafka checker)", st, err)
	}
}

func TestShutdownDrainsHealth(t *testing.T) {
	// scenario: Shutdown drains health, then stops serving
	_, osURL := startOpenSearch(t)
	s := searchSettings(testkit.FreePort(t), freshURL(t), osURL)
	s.DrainDelay = 1500 * time.Millisecond
	r := start(t, s)
	waitServing(t, r)

	wctx, wcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer wcancel()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(wctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first watch = %v, %v, want SERVING", first.GetStatus(), err)
	}

	r.cancel()
	next, err := stream.Recv()
	if err != nil {
		t.Fatalf("watch ended with %v before reporting NOT_SERVING; health must drain before gRPC stops", err)
	}
	if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("watch after cancel = %v, want NOT_SERVING", next.GetStatus())
	}
	select {
	case <-r.done:
		t.Fatal("Run returned before the drain delay elapsed")
	default:
	}
	select {
	case <-r.done:
	case <-time.After(s.ShutdownGrace + s.DrainDelay + 5*time.Second):
		t.Fatal("app did not stop")
	}
	if r.err != nil {
		t.Errorf("Run = %v, want nil on shutdown", r.err)
	}
}

func TestUnknownHealthServiceNotFound(t *testing.T) {
	// scenario: An unknown health service name is not found
	r, _ := startServing(t)
	_, err := healthStatus(r, "nope")
	if status.Code(err) != codes.NotFound {
		t.Fatalf("health Check(nope) = %v, want NotFound", err)
	}
}

func listReflectedServices(t *testing.T, r *running) ([]string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := reflectionpb.NewServerReflectionClient(r.conn).ServerReflectionInfo(ctx, grpc.WaitForReady(true))
	if err != nil {
		return nil, err
	}
	err = stream.Send(&reflectionpb.ServerReflectionRequest{
		MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""},
	})
	if err != nil {
		if _, rerr := stream.Recv(); rerr != nil {
			return nil, rerr
		}
		return nil, err
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, s := range resp.GetListServicesResponse().GetService() {
		names = append(names, s.GetName())
	}
	return names, nil
}

func TestReflectionNotServedByDefault(t *testing.T) {
	// scenario: Reflection is not served by default
	r, _ := startServing(t)
	_, err := listReflectedServices(t, r)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("reflection err = %v, want Unimplemented", err)
	}
}

func TestReflectionServedWhenEnabled(t *testing.T) {
	// scenario: Reflection is served locally when enabled
	_, osURL := startOpenSearch(t)
	s := searchSettings(testkit.FreePort(t), freshURL(t), osURL)
	s.Env = "local"
	s.ReflectionEnabled = true
	r := start(t, s)
	waitServing(t, r)

	names, err := listReflectedServices(t, r)
	if err != nil {
		t.Fatalf("reflection: %v", err)
	}
	for _, n := range names {
		if n == "platform.search.v1.SearchService" {
			return
		}
	}
	t.Fatalf("reflected services = %v, want platform.search.v1.SearchService", names)
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: The server does not migrate at boot
	_, osURL := startOpenSearch(t)
	url := freshURL(t)
	r := start(t, searchSettings(testkit.FreePort(t), url, osURL))
	waitServing(t, r)
	if tableExists(t, url, "saved_searches") {
		t.Fatal("table saved_searches exists after boot; app.Run must not migrate")
	}
}

func TestSearchListings_FreeTextAndFilter(t *testing.T) {
	// scenario: An anonymous principal holding search:read can search and get suggestions (signed-in search path)
	r, _ := startServing(t)
	ctx := principal(callCtx(t), "u1", "user", "search:read")

	resp, err := searchv1.NewSearchServiceClient(r.conn).SearchListings(ctx, &searchv1.SearchListingsRequest{
		Query:      "iphone",
		Filters:    map[string]string{"status": "published"},
		SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL,
	})
	if err != nil {
		t.Fatalf("SearchListings: %v", err)
	}
	if resp.GetPage().GetTotal() != 2 {
		t.Fatalf("want 2 published iphones, got total=%d hits=%d", resp.GetPage().GetTotal(), len(resp.GetHits()))
	}
	for _, h := range resp.GetHits() {
		if h.GetListingId() != "a1" && h.GetListingId() != "c3" {
			t.Fatalf("unexpected hit %q", h.GetListingId())
		}
	}
}

func TestSuggest(t *testing.T) {
	// scenario: An anonymous principal holding search:read can search and get suggestions (signed-in suggest path)
	r, _ := startServing(t)
	ctx := principal(callCtx(t), "u1", "user", "search:read")

	resp, err := searchv1.NewSearchServiceClient(r.conn).Suggest(ctx, &searchv1.SuggestRequest{Query: "ipho", Limit: 5})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(resp.GetSuggestions()) != 2 {
		t.Fatalf("want 2 suggestions for 'ipho', got %v", resp.GetSuggestions())
	}
}

func TestSearch_Unauthenticated(t *testing.T) {
	// scenario: SearchListings called with no principal metadata
	r, _ := startServing(t)
	_, err := searchv1.NewSearchServiceClient(r.conn).SearchListings(callCtx(t), &searchv1.SearchListingsRequest{Query: "x"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}

func TestSearch_InsufficientScope(t *testing.T) {
	// scenario: A search without the search scope is denied
	r, _ := startServing(t)
	client := searchv1.NewSearchServiceClient(r.conn)

	ctx := principal(callCtx(t), "u1", "user", "other.scope")
	if _, err := client.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SearchListings want PermissionDenied, got %v", err)
	}
	if _, err := client.Suggest(ctx, &searchv1.SuggestRequest{Query: "ip"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Suggest want PermissionDenied, got %v", err)
	}
	anon := principal(callCtx(t), "anonymous", "anonymous", "")
	if _, err := client.SearchListings(anon, &searchv1.SearchListingsRequest{Query: "x"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("anonymous without search:read want PermissionDenied, got %v", err)
	}
}

func TestSearch_AnonymousWithScope(t *testing.T) {
	// scenario: An anonymous principal holding search:read can search and get suggestions
	r, stub := startServing(t)
	client := searchv1.NewSearchServiceClient(r.conn)
	ctx := principal(callCtx(t), "anonymous", "anonymous", "search:read")

	resp, err := client.SearchListings(ctx, &searchv1.SearchListingsRequest{
		Query:      "i",
		SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL,
	})
	if err != nil {
		t.Fatalf("anonymous SearchListings: %v", err)
	}
	if len(resp.GetHits()) != 2 {
		t.Fatalf("anonymous search hits = %d, want the 2 published listings", len(resp.GetHits()))
	}
	for _, h := range resp.GetHits() {
		if h.GetListingId() == "b2" {
			t.Fatal("anonymous search returned a draft listing")
		}
	}
	bodies := stub.searchBodies()
	if len(bodies) == 0 || !strings.Contains(bodies[len(bodies)-1], `"status":"published"`) {
		t.Fatalf("index query was not published-only: %v", bodies)
	}

	sug, err := client.Suggest(ctx, &searchv1.SuggestRequest{Query: "ipho", Limit: 5})
	if err != nil {
		t.Fatalf("anonymous Suggest: %v", err)
	}
	if len(sug.GetSuggestions()) != 2 {
		t.Fatalf("anonymous suggestions = %v, want 2", sug.GetSuggestions())
	}
}

type fakeCollector struct {
	coltracepb.UnimplementedTraceServiceServer
	mu       sync.Mutex
	traceIDs map[string]bool
}

func (c *fakeCollector) Export(_ context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			for _, sp := range ss.GetSpans() {
				c.traceIDs[hex.EncodeToString(sp.GetTraceId())] = true
			}
		}
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func (c *fakeCollector) has(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.traceIDs[id]
}

func startCollector(t *testing.T) (*fakeCollector, string) {
	t.Helper()
	c := &fakeCollector{traceIDs: map[string]bool{}}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(srv, c)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return c, "http://" + lis.Addr().String()
}

type logLine map[string]any

func (l logLine) str(k string) string {
	s, _ := l[k].(string)
	return s
}

func parseLogs(logs string) []logLine {
	var out []logLine
	for _, line := range strings.Split(logs, "\n") {
		var rec logLine
		if json.Unmarshal([]byte(line), &rec) == nil {
			out = append(out, rec)
		}
	}
	return out
}

func startTraced(t *testing.T) (*running, *fakeCollector) {
	t.Helper()
	collector, endpoint := startCollector(t)
	_, osURL := startOpenSearch(t)
	s := searchSettings(testkit.FreePort(t), freshURL(t), osURL)
	s.OTel = sdkconfig.OTel{Enabled: true, Endpoint: endpoint}
	r := start(t, s)
	waitServing(t, r)
	return r, collector
}

func TestRequestIDAndTraceparentPropagate(t *testing.T) {
	// scenario: A request id is logged and a trace is continued
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const secretQuery = "SECRET-QUERY-TEXT-9917"
	r, collector := startTraced(t)

	ctx := metadata.AppendToOutgoingContext(callCtx(t),
		"x-request-id", "req-42",
		"traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	ctx = principal(ctx, "u1", "user", "search:read")
	if _, err := searchv1.NewSearchServiceClient(r.conn).SearchListings(ctx, &searchv1.SearchListingsRequest{
		Query:      secretQuery,
		SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL,
	}); err != nil {
		t.Fatalf("SearchListings: %v", err)
	}

	r.stop(t)
	logs := r.logs.String()
	if strings.Contains(logs, secretQuery) {
		t.Fatal("a log line holds the query text")
	}
	var found bool
	for _, l := range parseLogs(logs) {
		if l.str("request_id") != "req-42" {
			continue
		}
		found = true
		if got := l.str("trace_id"); got != traceID {
			t.Fatalf("call log trace_id = %q, want the incoming %q", got, traceID)
		}
	}
	if !found {
		t.Fatalf("no log line carries request_id req-42:\n%s", logs)
	}
	if !collector.has(traceID) {
		t.Fatal("no exported span belongs to the incoming trace")
	}
}

func TestRequestIDGeneratedAndBadTraceparentIgnored(t *testing.T) {
	// scenario: A malformed traceparent does not fail the call
	r, _ := startTraced(t)

	ctx := metadata.AppendToOutgoingContext(callCtx(t), "traceparent", "garbage-not-a-traceparent")
	ctx = principal(ctx, "u1", "user", "search:read")
	if _, err := searchv1.NewSearchServiceClient(r.conn).SearchListings(ctx, &searchv1.SearchListingsRequest{
		Query:      "iphone",
		SearchMode: searchv1.SearchMode_SEARCH_MODE_LEXICAL,
	}); err != nil {
		t.Fatalf("a malformed traceparent rejected the call: %v", err)
	}

	r.stop(t)
	var found bool
	for _, l := range parseLogs(r.logs.String()) {
		if !strings.Contains(l.str("grpc.method"), "SearchListings") {
			continue
		}
		found = true
		if l.str("request_id") == "" {
			t.Errorf("call log has no generated request_id: %v", l)
		}
		tid := l.str("trace_id")
		if len(tid) != 32 || strings.Trim(tid, "0") == "" {
			t.Errorf("call log trace_id = %q, want a freshly started 32-hex trace", tid)
		}
	}
	if !found {
		t.Fatalf("no call log line for SearchListings:\n%s", r.logs.String())
	}
}

func migrateSettings(url string) config.MigrateSettings {
	return config.MigrateSettings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		Postgres: sdkconfig.Postgres{URL: url, MaxConns: 2},
	}
}

func schemaVersion(t *testing.T, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := dial(t, url).QueryRow(context.Background(), `SELECT version, dirty FROM schema_migrations`).Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func TestMigrateUpAppliesEmbeddedMigrations(t *testing.T) {
	// scenario: migrate up applies the embedded migrations
	url := freshURL(t)
	if err := app.Migrate(context.Background(), migrateSettings(url)); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if !tableExists(t, url, "saved_searches") {
		t.Fatal("table saved_searches missing after migrate")
	}
	v, dirty := schemaVersion(t, url)
	if dirty || v < 1 {
		t.Fatalf("schema version = %d dirty=%v, want a clean applied version", v, dirty)
	}
	if err := app.Migrate(context.Background(), migrateSettings(url)); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if v2, d2 := schemaVersion(t, url); v2 != v || d2 {
		t.Errorf("version after rerun = %d dirty=%v, want %d clean", v2, d2, v)
	}
}

func TestMigrateUpFailsOnUnreachableDatabase(t *testing.T) {
	// scenario: migrate up with an unreachable database fails
	done := make(chan error, 1)
	go func() { done <- app.Migrate(context.Background(), migrateSettings(closedDatabaseURL)) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Migrate returned nil for an unreachable database")
		}
	case <-time.After(bootBound):
		t.Fatalf("Migrate did not return within %v", bootBound)
	}
}

func TestMigrateSubcommandExitCodes(t *testing.T) {
	// scenario: migrate up with an unreachable database fails (process exit) / migrate up applies the embedded migrations
	bin := t.TempDir() + "/server"
	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}
	run := func(url string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "migrate", "up")
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "ENV=local", "DATABASE_URL=" + url}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	if out, err := run(closedDatabaseURL); err == nil {
		t.Fatalf("migrate up against a closed port exited 0:\n%s", out)
	}
	url := freshURL(t)
	if out, err := run(url); err != nil {
		t.Fatalf("migrate up exited non-zero: %v\n%s", err, out)
	}
	if !tableExists(t, url, "saved_searches") {
		t.Fatal("table saved_searches missing after `server migrate up`")
	}
}
