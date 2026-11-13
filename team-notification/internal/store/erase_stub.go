package store

import (
	"context"
	"errors"
)

type Eraser struct{ q Querier }

func NewEraser(q Querier) *Eraser { return &Eraser{q: q} }

func (e *Eraser) Erase(_ context.Context, _ string) (int64, error) {
	return 0, errors.New("not implemented")
}
