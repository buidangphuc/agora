package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
	// ShutdownGrace is the one budget shared by stopping the components and
	// running the closers: both must finish within ShutdownGrace of the
	// components being cancelled, and the closers receive a context with the
	// time that is left (already expired if the components used it all).
	// Zero means 10s.
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
// waits for them and runs the closers in reverse order, all within one
// ShutdownGrace. The worst case from cancellation to return is therefore
// DrainDelay + ShutdownGrace plus whatever the closers ignore of their context.
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

	shutCtx, cancelShut := context.WithTimeout(context.WithoutCancel(ctx), grace)
	defer cancelShut()

	var deadline error
	select {
	case <-allDone:
	case <-shutCtx.Done():
		deadline = fmt.Errorf("lifecycle: components still running after %s: %w", grace, context.DeadlineExceeded)
		logger.ErrorContext(ctx, "lifecycle: components did not stop within the grace period", "grace", grace)
	}

	var closeErr error
	for i := len(o.Closers) - 1; i >= 0; i-- {
		if err := o.Closers[i](shutCtx); err != nil {
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

// StopAfter returns a Component that runs primary and the dependents together
// and stops the dependents only after primary has returned. Cancelling the
// parent context reaches primary alone, so work that primary drains on the way
// out (in-flight RPCs that enqueue outbox rows, say) is still seen by the
// dependents. A dependent that fails cancels primary. Run returns after all
// of them have returned, with their errors joined.
func StopAfter(primary Component, dependents ...Component) Component {
	return Func(func(ctx context.Context) error {
		primaryCtx, cancelPrimary := context.WithCancel(ctx)
		defer cancelPrimary()
		depCtx, cancelDeps := context.WithCancel(context.WithoutCancel(ctx))
		defer cancelDeps()

		var (
			wg   sync.WaitGroup
			mu   sync.Mutex
			errs []error
		)
		for _, d := range dependents {
			wg.Go(func() {
				if err := d.Run(depCtx); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					cancelPrimary()
				}
			})
		}

		primaryErr := primary.Run(primaryCtx)
		cancelDeps()
		wg.Wait()

		mu.Lock()
		defer mu.Unlock()
		return errors.Join(append([]error{primaryErr}, errs...)...)
	})
}

// EveryOption configures Every.
type EveryOption func(*everyOptions)

type everyOptions struct {
	runAtStart bool
	logger     *slog.Logger
}

// RunAtStart makes Every call fn once immediately instead of waiting for the
// first interval.
func RunAtStart() EveryOption { return func(o *everyOptions) { o.runAtStart = true } }

// WithLogger sets the logger for failed runs. Default slog.Default().
func WithLogger(l *slog.Logger) EveryOption { return func(o *everyOptions) { o.logger = l } }

// Every returns a Component that calls fn every interval, and once at start
// with RunAtStart. A returned error is logged and the worker keeps running;
// Run returns nil when ctx ends. fn receives ctx and should return when it is
// cancelled. It returns an error when interval is not positive.
func Every(interval time.Duration, fn func(context.Context) error, opts ...EveryOption) (Component, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("lifecycle: Every interval must be positive, got %s", interval)
	}
	var o everyOptions
	for _, opt := range opts {
		opt(&o)
	}
	logger := o.logger
	if logger == nil {
		logger = slog.Default()
	}
	return Func(func(ctx context.Context) error {
		tick := func() {
			if err := fn(ctx); err != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "lifecycle: periodic task failed", "err", err)
			}
		}
		if o.runAtStart && ctx.Err() == nil {
			tick()
		}
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-t.C:
				tick()
			}
		}
	}), nil
}

// HTTP returns a Component that serves srv until ctx ends and then calls
// Shutdown bounded by grace (zero means 10s), so in-flight requests finish.
// A serve error other than http.ErrServerClosed is returned at once; when the
// grace period runs out the remaining connections are closed and the deadline
// error is returned.
func HTTP(srv *http.Server, grace time.Duration) Component {
	return Func(func(ctx context.Context) error {
		if grace <= 0 {
			grace = defaultGrace
		}
		serveErr := make(chan error, 1)
		go func() { serveErr <- srv.ListenAndServe() }()
		select {
		case err := <-serveErr:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
		}
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), grace)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			_ = srv.Close()
			return fmt.Errorf("lifecycle: http shutdown: %w", err)
		}
		return nil
	})
}
