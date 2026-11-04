package booking

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultFailureThreshold = 5
	defaultOpenTimeout      = 15 * time.Second
	defaultWindow           = 30 * time.Second
)

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

type CircuitBreakerConfig struct {
	FailureThreshold int
	OpenTimeout      time.Duration
	Window           time.Duration
}

func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: defaultFailureThreshold,
		OpenTimeout:      defaultOpenTimeout,
		Window:           defaultWindow,
	}
}

type CircuitBreaker struct {
	cfg CircuitBreakerConfig
	now func() time.Time

	mu            sync.Mutex
	state         State
	failures      []time.Time
	openedAt      time.Time
	probeInFlight bool
}

func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold < 1 {
		cfg.FailureThreshold = defaultFailureThreshold
	}
	if cfg.OpenTimeout <= 0 {
		cfg.OpenTimeout = defaultOpenTimeout
	}
	if cfg.Window <= 0 {
		cfg.Window = defaultWindow
	}
	return &CircuitBreaker{
		cfg:   cfg,
		now:   time.Now,
		state: StateClosed,
	}
}

func (b *CircuitBreaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *CircuitBreaker) UnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if err := b.beforeCall(); err != nil {
			return err
		}

		err := invoker(ctx, method, req, reply, cc, opts...)
		b.afterCall(err)
		return err
	}
}

func (b *CircuitBreaker) beforeCall() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	switch b.state {
	case StateClosed:
		return nil
	case StateOpen:
		if now.Sub(b.openedAt) < b.cfg.OpenTimeout {
			return errCircuitOpen()
		}
		b.transitionLocked(StateHalfOpen, fmt.Sprintf("timeout %s elapsed", b.cfg.OpenTimeout))
		b.probeInFlight = true
		return nil
	case StateHalfOpen:
		if b.probeInFlight {
			return errCircuitOpen()
		}
		b.probeInFlight = true
		return nil
	default:
		return errCircuitOpen()
	}
}

func (b *CircuitBreaker) afterCall(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	switch b.state {
	case StateClosed:
		if isBreakerFailure(err) {
			b.recordFailureLocked(now)
		}
	case StateHalfOpen:
		if !b.probeInFlight {
			return
		}
		b.probeInFlight = false
		if isBreakerFailure(err) {
			b.openLocked(now, fmt.Sprintf("probe failed: %v", err))
			return
		}
		b.failures = nil
		b.transitionLocked(StateClosed, "probe succeeded")
	case StateOpen:
		// Результаты запросов, начатых до открытия, уже не меняют состояние
	}
}

func (b *CircuitBreaker) recordFailureLocked(now time.Time) {
	cutoff := now.Add(-b.cfg.Window)
	recent := b.failures[:0]
	for _, failure := range b.failures {
		if failure.After(cutoff) {
			recent = append(recent, failure)
		}
	}
	b.failures = append(recent, now)

	if len(b.failures) >= b.cfg.FailureThreshold {
		b.openLocked(now, fmt.Sprintf("%d failures in %s", len(b.failures), b.cfg.Window))
	}
}

func (b *CircuitBreaker) openLocked(now time.Time, reason string) {
	if b.state != StateOpen {
		b.transitionLocked(StateOpen, reason)
	}
	b.openedAt = now
	b.probeInFlight = false
}

func (b *CircuitBreaker) transitionLocked(next State, reason string) {
	if b.state == next {
		return
	}
	log.Printf("circuit breaker %s -> %s: %s", b.state, next, reason)
	b.state = next
}

func isBreakerFailure(err error) bool {
	if err == nil {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Internal, codes.Unknown:
		return true
	default:
		return false
	}
}

func errCircuitOpen() error {
	return status.Error(codes.Unavailable, "flight service unavailable: circuit breaker is open")
}
