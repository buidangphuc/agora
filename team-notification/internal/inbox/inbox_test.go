package inbox_test

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/buidangphuc/team-notification/internal/inbox"
)

func TestErrNotFoundSentinel_UsersDoNotSeeEachOthersData(t *testing.T) {
	if inbox.ErrNotFound == nil {
		t.Fatalf("inbox.ErrNotFound must be defined")
	}
	wrapped := errors.Join(errors.New("ctx"), inbox.ErrNotFound)
	if !errors.Is(wrapped, inbox.ErrNotFound) {
		t.Fatalf("errors.Is must match inbox.ErrNotFound")
	}
	if inbox.ErrNotFound.Error() != "notification not found" {
		t.Fatalf("message must stay %q, got %q", "notification not found", inbox.ErrNotFound.Error())
	}
}

func TestPackageHasNoGRPCImport_UsersDoNotSeeEachOthersData(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, "google.golang.org/grpc") {
				t.Fatalf("%s imports %s", e.Name(), imp.Path.Value)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("no non-test source files")
	}
}
