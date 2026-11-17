package paymentfake

import (
	"context"
	"sync"
)

type Call struct {
	SellerID    string
	Amount      int64
	ReferenceID string
}

type Charger struct {
	mu     sync.Mutex
	calls  []Call
	ledger map[string]Call
	err    error
	onCall func()
}

func New() *Charger {
	return &Charger{ledger: make(map[string]Call)}
}

func (c *Charger) SetError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
}

func (c *Charger) OnCall(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onCall = fn
}

func (c *Charger) ChargeSellerFee(_ context.Context, sellerID string, amount int64, referenceID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := Call{SellerID: sellerID, Amount: amount, ReferenceID: referenceID}
	c.calls = append(c.calls, call)
	if c.onCall != nil {
		c.onCall()
	}
	if c.err != nil {
		return false, c.err
	}
	if _, seen := c.ledger[referenceID]; seen {
		return false, nil
	}
	c.ledger[referenceID] = call
	return true, nil
}

func (c *Charger) Calls() []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Call(nil), c.calls...)
}

func (c *Charger) Fees() []Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Call, 0, len(c.ledger))
	for _, f := range c.ledger {
		out = append(out, f)
	}
	return out
}
