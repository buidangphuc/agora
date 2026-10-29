package config_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	sdkModule = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"
	sdkTag    = "v1.0.2"
	lintTool  = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
)

func serviceFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestGoModRequiresTaggedSDK(t *testing.T) {
	mod := serviceFile(t, "go.mod")

	require := regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkModule) + `\s+` + regexp.QuoteMeta(sdkTag) + `\s*(?://.*)?$`)
	if !require.MatchString(mod) {
		t.Errorf("go.mod does not require %s at %s", sdkModule, sdkTag)
	}

	replace := regexp.MustCompile(`(?m)^\s*(?:replace\s+)?` + regexp.QuoteMeta(sdkModule) + `(?:\s+v\S+)?\s*=>`)
	if replace.MatchString(mod) {
		t.Errorf("go.mod has a replace directive for %s", sdkModule)
	}

	tool := regexp.MustCompile(`(?m)^\s*tool\s+` + regexp.QuoteMeta(lintTool) + `\s*$|^\s*` + regexp.QuoteMeta(lintTool) + `\s*$`)
	if !tool.MatchString(mod) {
		t.Errorf("go.mod has no tool directive for %s", lintTool)
	}
}

func TestNoVendoredSDKOrLintCopy(t *testing.T) {
	root := filepath.Join("..", "..")
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() && d.Name() == "third_party" {
			found = append(found, path)
		}
		if !d.IsDir() && d.Name() == ".golangci.yml" {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(found) > 0 {
		t.Errorf("vendored SDK or lint copy present: %v", found)
	}

	mk := serviceFile(t, "Makefile")
	for _, want := range []string{
		"include ../platform-core/make/go-service.mk",
		"LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml",
	} {
		if !strings.Contains(mk, want) {
			t.Errorf("Makefile lacks %q", want)
		}
	}
}

func TestDockerfileBuildsBothBinaries(t *testing.T) {
	df := serviceFile(t, "Dockerfile")

	for _, want := range []string{
		"-o /out/server ./cmd/server",
		"-o /out/indexer ./cmd/indexer",
		"COPY --from=build /out/server /server",
		"COPY --from=build /out/indexer /indexer",
		`ENTRYPOINT ["/server"]`,
	} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile lacks %q", want)
		}
	}
	if strings.Contains(df, "third_party") {
		t.Errorf("Dockerfile references third_party")
	}
}

func TestReadmeDropsRetiredKeysAndStalePackages(t *testing.T) {
	readme := serviceFile(t, "README.md")
	for _, banned := range []string{
		"DATABASE_ENABLED", "KAFKA_ENABLED", "SHUTDOWN_GRACE_SECONDS",
		"internal/handler", "internal/consumer", "internal/bootstrap", "internal/repository",
	} {
		if strings.Contains(readme, banned) {
			t.Errorf("README.md still mentions %s", banned)
		}
	}
	for _, want := range []string{
		"ENV", "GRPC_PORT", "SHUTDOWN_GRACE", "GRPC_REFLECTION_ENABLED",
		"LOG_KEY_FP_SECRET", "UPSTREAM_AI_ADDR", "/indexer", "make",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md does not document %s", want)
		}
	}
}

func TestNoLeftoverLegacyPackages(t *testing.T) {
	for _, dir := range []string{"consumer", "repository", "handler", "bootstrap"} {
		p := filepath.Join("..", dir)
		if _, err := os.Stat(p); err == nil {
			t.Errorf("legacy package internal/%s still exists", dir)
		}
	}
}
