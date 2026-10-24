// Package screen is the keyword screener for listing moderation: it reports
// which prohibited terms appear in a text, ignoring case and diacritics.
package screen

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

//go:embed terms.txt
var embeddedTerms string

// Screener matches text against a fixed list of prohibited terms.
type Screener struct {
	terms []string // as listed
	norm  []string // normalized, parallel to terms
}

// New builds a Screener from the terms file at path; an empty path uses the
// embedded list. A missing or unreadable file is an error naming the path.
func New(path string) (*Screener, error) {
	content := embeddedTerms
	if path != "" {
		b, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return nil, fmt.Errorf("screen terms file %s: %w", path, err)
		}
		content = string(b)
	}
	s := &Screener{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		n := normalize(line)
		if n == "" {
			continue
		}
		s.terms = append(s.terms, line)
		s.norm = append(s.norm, n)
	}
	return s, nil
}

// Len is the number of loaded terms.
func (s *Screener) Len() int { return len(s.terms) }

// Terms returns the loaded terms as listed.
func (s *Screener) Terms() []string { return append([]string(nil), s.terms...) }

// Match returns each term (as listed) found in text, once, in list order.
func (s *Screener) Match(text string) []string {
	n := normalize(text)
	if n == "" {
		return nil
	}
	var out []string
	for i, t := range s.norm {
		if strings.Contains(n, t) {
			out = append(out, s.terms[i])
		}
	}
	return out
}

// normalize lowercases, strips combining marks (NFD) and folds đ to d.
func normalize(in string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(in)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == 'đ' {
			r = 'd'
		}
		b.WriteRune(r)
	}
	return b.String()
}
