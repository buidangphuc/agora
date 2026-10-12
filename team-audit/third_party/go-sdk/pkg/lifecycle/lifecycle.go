package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const defaultGrace = 10 * time.Second

// Component is anything long-running. Run blocks until ctx is cancelled or the
// component fails.
type Component interface {
	Run(ctx context.Context) error
}

// Func adapts a function to Component.
type Func func(ctx context.Context) error

// Run calls f.
func (f Func) Run(ctx context.Context) error { return f(ctx) }

// Options configures Run.
type Options struct {
	// ShutdownGrace bounds how long components get to stop after cancellation
	// and is also the budget handed to the closers. Zero means 10s.
	ShutdownGrace time.Duration
	// OnShutdown hooks run first on shutdown, before components are cancelled
	// (for example healthx Drain).
	OnShutdown []func()
	// DrainDelay is waited after the OnShutdown hooks and before the
	// components are cancelled, so probes observe NOT_SERVING. Zero means none.
	DrainDelay time.Duration
	// Closers run last, in reverse registration order, even after the grace
	// period was exceeded.
	Closers []func(context.Context) error
	Logger  *slog.Logger
}

// Run starts every component and returns when ctx is cancelled or any
// component returns. It then runs the OnShutdown hooks, cancels the components,
// waits for them up to ShutdownGrace and runs the closers in reverse order.
// It returns the first error that is not context.Canceled, or a wrapped
// context.DeadlineExceeded if the components outlived the grace period.
func Run(ctx context.Context, o Options, cs ...Component) error {
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}
	grace := o.ShutdownGrace
	if grace <= 0 {
		grace = defaultGrace
	}

	// Components outlive ctx so the shutdown hooks can run before they are cancelled.
	compCtx, cancelComps := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelComps()

	var (
		mu   sync.Mutex
		errs []error
	)
	exited := make(chan struct{}, len(cs))
	var g errgroup.Group
	for _, c := range cs {
		g.Go(func() error {
			err := c.Run(compCtx)
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
			exited <- struct{}{}
			return nil
		})
	}
	allDone := make(chan struct{})
	go func() { _ = g.Wait(); close(allDone) }()

	if len(cs) > 0 {
		select {
		case <-ctx.Done():
		case <-exited:
		}
	} else {
		<-ctx.Done()
	}

	for _, hook := range o.OnShutdown {
		hook()
	}
	if o.DrainDelay > 0 {
		time.Sleep(o.DrainDelay)
	}
	cancelComps()

	var deadline error
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-allDone:
	case <-timer.C:
		deadline = fmt.Errorf("lifecycle: components still running after %s: %w", grace, context.DeadlineExceeded)
		logger.ErrorContext(ctx, "lifecycle: components did not stop within the grace period", "grace", grace)
	}

	closeCtx, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), grace)
	defer cancelClose()
	var closeErr error
	for i := len(o.Closers) - 1; i >= 0; i-- {
		if err := o.Closers[i](closeCtx); err != nil {
			logger.ErrorContext(ctx, "lifecycle: closer failed", "err", err)
			closeErr = errors.Join(closeErr, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for _, err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	if deadline != nil {
		return deadline
	}
	return closeErr
}

// Stack collects closers while a composition root wires its dependencies, so
// a failure halfway can release what was already opened.
type Stack struct {
	mu      sync.Mutex
	closers []func(context.Context) error
	closed  bool
}

// Push registers a closer. Closers run in reverse order of registration.
func (s *Stack) Push(f func(context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closers = append(s.closers, f)
}

// Close runs the closers in reverse order and joins their errors. Only the
// first call does anything, so it is safe both on an early failure and as a
// lifecycle closer.
func (s *Stack) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	closers := s.closers
	s.mu.Unlock()

	var err error
	for i := len(closers) - 1; i >= 0; i-- {
		err = errors.Join(err, closers[i](ctx))
	}
	return err
}
