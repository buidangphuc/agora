package app_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const sdkModule = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"

func serviceRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve service root: %v", err)
	}
	return root
}

func TestGoModRequiresSDKTagWithoutReplace(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(serviceRoot(t), "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	requireRe := regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + regexp.QuoteMeta(sdkModule) + `\s+(\S+)`)
	matches := requireRe.FindAllSubmatch(raw, -1)
	if len(matches) != 1 {
		t.Fatalf("go.mod has %d requires of %s, want exactly 1", len(matches), sdkModule)
	}
	if got := string(matches[0][1]); got != "v1.0.2" {
		t.Errorf("go.mod requires %s %s, want v1.0.2", sdkModule, got)
	}
	if n := len(regexp.MustCompile(`(?m)^\s*replace\b`).FindAll(raw, -1)); n != 0 {
		t.Errorf("go.mod has %d replace directives, want 0", n)
	}
	toolRe := regexp.MustCompile(`(?m)^\s*tool\s+github\.com/golangci/golangci-lint/v2/cmd/golangci-lint\s*$`)
	if !toolRe.Match(raw) {
		t.Errorf("go.mod does not pin golangci-lint with a tool directive")
	}
}

func TestNoVendoredSDKOrLocalLintCopy(t *testing.T) {
	root := serviceRoot(t)
	if _, err := os.Stat(filepath.Join(root, "third_party")); !os.IsNotExist(err) {
		t.Errorf("third_party must not exist (stat err: %v)", err)
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == "node_modules" || n == "generated" || n == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if n := d.Name(); n == ".golangci.yml" || n == ".golangci.yaml" || n == "go-service.mk" {
			t.Errorf("copy of shared file must not exist: %s", path)
		}
		return nil
	})
}

func TestMakefileReferencesSharedFiles(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(serviceRoot(t), "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	mk := string(raw)
	checks := map[string]string{
		"include ../platform-core/make/go-service.mk":                   `(?m)^include\s+\.\./platform-core/make/go-service\.mk\s*$`,
		"LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml": `(?m)^LINT_CONFIG\s*:=\s*\.\./platform-core/packages/go-sdk/\.golangci\.yml\s*$`,
		"SERVICE":   `(?m)^SERVICE\s*:?=\s*\S+`,
		"GRPC_PORT": `(?m)^GRPC_PORT\s*:?=\s*\S+`,
		"BINARIES := consumer ga4-relay ga4import": `(?m)^BINARIES\s*:?=\s*consumer\s+ga4-relay\s+ga4import\s*$`,
	}
	for name, pattern := range checks {
		if !regexp.MustCompile(pattern).MatchString(mk) {
			t.Errorf("Makefile is missing %q", name)
		}
	}
}

func TestStandaloneComposeLoadsUnderLocalEnv(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(serviceRoot(t), "docker-compose.local.yaml"))
	if err != nil {
		t.Fatalf("read compose file: %v", err)
	}
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse compose file: %v", err)
	}
	env := doc.Services["team-analytics"].Environment
	if env["ENV"] != "local" {
		t.Errorf("ENV = %q, want local", env["ENV"])
	}
	if env["GRPC_PORT"] != "50059" {
		t.Errorf("GRPC_PORT = %q, want 50059", env["GRPC_PORT"])
	}
	if _, ok := env["KAFKA_ENABLED"]; ok {
		t.Errorf("KAFKA_ENABLED must not be declared")
	}
}

func TestCmdMainsOnlyCallApp(t *testing.T) {
	root := serviceRoot(t)
	forbidden := []string{"/internal/warehouse", "/internal/query", "/internal/events", "github.com/twmb/franz-go"}
	fset := token.NewFileSet()
	for _, bin := range []string{"consumer", "ga4-relay", "ga4import"} {
		path := filepath.Join(root, "cmd", bin, "main.go")
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			for _, bad := range forbidden {
				if strings.Contains(p, bad) {
					t.Errorf("cmd/%s/main.go imports %s", bin, p)
				}
			}
		}
	}
}
