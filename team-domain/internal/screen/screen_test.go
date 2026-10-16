package screen

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func writeTerms(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "terms.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Scenario: Auto-screen: Embedded term list is the default.
func TestDefaultTermsEmbedded(t *testing.T) {
	s, err := New("") // empty path = embedded list, no file read
	if err != nil {
		t.Fatalf("New(\"\"): %v", err)
	}
	if s.Len() < 1 {
		t.Fatalf("embedded list has %d terms, want >= 1", s.Len())
	}
	// Every embedded term must be matched by itself (list is usable).
	for _, term := range s.Terms() {
		if got := s.Match("bán " + term + " giá rẻ"); len(got) == 0 {
			t.Errorf("embedded term %q does not match its own text", term)
		}
	}
}

// Scenario: Auto-screen: Keyword match ignores case and diacritics.
func TestKeywordMatchCaseAndDiacriticInsensitive(t *testing.T) {
	s, err := New(writeTerms(t, "thuốc lá\n"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, text string
		want       []string
	}{
		{"upper-case ascii", "Bán THUOC LA điếu", []string{"thuốc lá"}},
		{"exact accents", "thuốc lá", []string{"thuốc lá"}},
		{"mixed case accents", "Thuốc Lá nhập khẩu", []string{"thuốc lá"}},
		{"decomposed NFD input", "thuốc lá", []string{"thuốc lá"}},
		{"no match", "căn hộ 2 phòng ngủ", nil},
		{"empty text", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := s.Match(c.text)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("Match(%q) = %v, want %v (term reported as listed)", c.text, got, c.want)
			}
		})
	}
}

func TestKeywordMatchReportsEachHitedTermOnce(t *testing.T) {
	s, err := New(writeTerms(t, "alpha\nbeta\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := s.Match("ALPHA alpha Beta gamma")
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("got %v, want [alpha beta]", got)
	}
}

// Scenario: Auto-screen: Keyword list loads from the config file.
func TestLoadTermsSkipsBlankAndComments(t *testing.T) {
	p := writeTerms(t, "# prohibited terms\nthuốc lá\n\nma túy\n   \n  vũ khí  \n")
	s, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Len() != 3 {
		t.Fatalf("Len = %d (%v), want exactly 3", s.Len(), s.Terms())
	}
	if len(s.Match("# prohibited terms")) != 0 {
		t.Error("comment line must not become a term")
	}
	if len(s.Match("bán vu khi")) != 1 {
		t.Error("surrounding whitespace in a term must be trimmed")
	}
	// The file replaces (not extends) the embedded list.
	for _, term := range s.Terms() {
		if !strings.Contains("thuốc lá ma túy vũ khí", term) {
			t.Errorf("unexpected term %q from file", term)
		}
	}
}

// Scenario: Auto-screen: Missing terms file stops start-up.
func TestNewMissingFileFails(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.txt")
	s, err := New(p)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if s != nil {
		t.Error("screener must be nil on error")
	}
	if !strings.Contains(err.Error(), p) {
		t.Errorf("error %q must name the file %q", err, p)
	}
}
