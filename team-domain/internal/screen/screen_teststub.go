package screen

// TEST-STUB: compile-only; the implementer replaces this file with screen.go.

import "errors"

type Screener struct{}

func New(path string) (*Screener, error)  { return nil, errors.New("not implemented") }
func (s *Screener) Len() int              { return 0 }
func (s *Screener) Terms() []string       { return nil }
func (s *Screener) Match(string) []string { return nil }
