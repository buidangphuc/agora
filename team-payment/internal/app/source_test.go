package app_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func serviceRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

func goSources(t *testing.T) map[string]string {
	t.Helper()
	root := serviceRoot(t)
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "generated" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, "_teststub.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestHandRolledInfrastructurePackagesAreGone(t *testing.T) {
	// scenario: Hand-rolled infrastructure packages are gone
	for _, dir := range []string{"bootstrap", "grpcserver", "handler", "interceptor", "repository", "service", "consumer", "pgtest"} {
		if _, err := os.Stat(filepath.Join(serviceRoot(t), "internal", dir)); err == nil {
			t.Errorf("internal/%s still exists; the old layout must be deleted", dir)
		}
	}
	for rel, src := range goSources(t) {
		for _, marker := range []string{"grpc.NewServer(", "health.NewServer("} {
			if strings.Contains(src, marker) {
				t.Errorf("%s builds infrastructure by hand (%s); use the SDK", rel, marker)
			}
		}
	}
}

func TestProductionWiringImportsNoInMemoryStore(t *testing.T) {
	// scenario: Production wiring imports no in-memory store
	fakePkg := filepath.Join("internal", "store", "fake") + string(filepath.Separator)
	for rel, src := range goSources(t) {
		if strings.HasPrefix(rel, fakePkg) {
			continue
		}
		for _, marker := range []string{
			"team-payment/internal/store/fake", "InMemory", "NewNoop", "RequireDurableStorage",
			"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "OpenOrLocal",
		} {
			if strings.Contains(src, marker) {
				t.Errorf("%s contains runtime fallback marker %q", rel, marker)
			}
		}
	}
}

func TestMockPaymentsEnvIsNotReadAnywhere(t *testing.T) {
	// scenario: MOCK_PAYMENTS no longer enables anything
	for rel, src := range goSources(t) {
		if strings.Contains(src, "MOCK_PAYMENTS") {
			t.Errorf("%s still mentions MOCK_PAYMENTS", rel)
		}
	}
}

func TestMainDispatchesToAppOnly(t *testing.T) {
	// scenario: The migrate subcommand applies every embedded migration
	main := goSources(t)[filepath.Join("cmd", "server", "main.go")]
	if strings.Contains(main, "pgxpool") {
		t.Error("cmd/server/main.go opens a pool itself; it must only call app.Run and app.Migrate")
	}
	if !strings.Contains(main, "app.Run") || !strings.Contains(main, "app.Migrate") {
		t.Error("cmd/server/main.go must dispatch to app.Run and app.Migrate")
	}
}

func TestAppUsesSDKCompositionAndNeverMigrates(t *testing.T) {
	// scenario: Booting the service never migrates
	var appSrc strings.Builder
	for rel, src := range goSources(t) {
		if strings.HasPrefix(rel, filepath.Join("internal", "app")+string(filepath.Separator)) {
			appSrc.WriteString(src)
		}
	}
	s := appSrc.String()
	if s == "" {
		t.Fatal("internal/app has no production source")
	}
	for _, want := range []string{"grpcx.NewServer", "rpc.Register", "lifecycle.Run", "mockdata.New", "auditx.Dial"} {
		if !strings.Contains(s, want) {
			t.Errorf("internal/app does not use %s", want)
		}
	}
}
