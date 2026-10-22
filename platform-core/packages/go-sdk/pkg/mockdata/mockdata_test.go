package mockdata_test

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/mockdata"
)

type settings struct {
	config.Runtime
	config.MockData
}

func load(t *testing.T, raw string) config.MockData {
	t.Helper()
	t.Setenv("ENV", "local")
	t.Setenv("MOCK_DATA", raw)
	s, err := config.Load[settings]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s.MockData
}

func TestNewRejectsUnknownAdapter(t *testing.T) {
	cfg := load(t, "payment-provider,telepathy")
	set, err := mockdata.New(cfg, "payment-provider", "eta-provider")
	if err == nil {
		t.Fatalf("New = %v, want error", set)
	}
	for _, want := range []string{"MOCK_DATA", "telepathy", "payment-provider", "eta-provider"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestNewNormalisesNames(t *testing.T) {
	cfg := load(t, " Payment-Provider ,payment-provider,,")
	set, err := mockdata.New(cfg, "payment-provider")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !set.Active("payment-provider") {
		t.Error("Active(payment-provider) = false")
	}
	if got := set.Adapters(); !slices.Equal(got, []string{"payment-provider"}) {
		t.Errorf("Adapters = %v", got)
	}
}

func TestNewEmptyHasNoActiveAdapters(t *testing.T) {
	set, err := mockdata.New(load(t, ""), "payment-provider")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if set.Active("payment-provider") || len(set.Adapters()) != 0 {
		t.Errorf("Adapters = %v, want none", set.Adapters())
	}
}

func TestLogActiveOncePerSet(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	set, err := mockdata.New(load(t, "payment-provider"), "payment-provider")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	set.LogActive(logger)
	set.LogActive(logger)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "payment-provider") {
		t.Fatalf("log = %q, want exactly one record naming payment-provider", buf.String())
	}

	buf.Reset()
	empty, err := mockdata.New(load(t, ""), "payment-provider")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	empty.LogActive(logger)
	if buf.Len() != 0 {
		t.Errorf("log = %q, want nothing", buf.String())
	}
}

func TestSelectSwapsOnlyActiveAdapter(t *testing.T) {
	set, err := mockdata.New(load(t, "payment-provider"), "payment-provider", "eta-provider")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := mockdata.Select(set, "payment-provider", "real", "fixture"); got != "fixture" {
		t.Errorf("active: got %q, want fixture", got)
	}
	if got := mockdata.Select(set, "eta-provider", "real", "fixture"); got != "real" {
		t.Errorf("inactive: got %q, want real", got)
	}
}
