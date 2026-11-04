package booking

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func newTestBreaker(cfg CircuitBreakerConfig) (*CircuitBreaker, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	breaker := NewCircuitBreaker(cfg)
	breaker.now = clock.Now
	return breaker, clock
}

func invokeBreaker(breaker *CircuitBreaker, invoker grpc.UnaryInvoker) error {
	return breaker.UnaryInterceptor()(context.Background(), "/test.Service/Method", nil, nil, nil, invoker)
}

func countingInvoker(err error) (grpc.UnaryInvoker, *int) {
	calls := 0
	return func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		return err
	}, &calls
}

func openBreaker(t *testing.T, breaker *CircuitBreaker, clock *fakeClock) {
	t.Helper()

	invoker, _ := countingInvoker(status.Error(codes.Unavailable, "down"))
	for i := 0; i < breaker.cfg.FailureThreshold; i++ {
		if err := invokeBreaker(breaker, invoker); status.Code(err) != codes.Unavailable {
			t.Fatalf("failure call code = %v, want Unavailable", status.Code(err))
		}
		clock.Advance(time.Second)
	}
	if breaker.State() != StateOpen {
		t.Fatalf("state = %v, want OPEN", breaker.State())
	}
}

func TestCircuitBreakerClosedPassesRequests(t *testing.T) {
	breaker, _ := newTestBreaker(CircuitBreakerConfig{FailureThreshold: 3})

	invoker, calls := countingInvoker(nil)
	if err := invokeBreaker(breaker, invoker); err != nil {
		t.Fatalf("call: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("calls = %d, want 1", *calls)
	}
	if breaker.State() != StateClosed {
		t.Fatalf("state = %v, want CLOSED", breaker.State())
	}
}

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	breaker, clock := newTestBreaker(CircuitBreakerConfig{FailureThreshold: 3})

	openBreaker(t, breaker, clock)
}

func TestCircuitBreakerIgnoresOldFailures(t *testing.T) {
	breaker, clock := newTestBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		Window:           10 * time.Second,
	})

	invoker, _ := countingInvoker(status.Error(codes.Unavailable, "down"))
	if err := invokeBreaker(breaker, invoker); err == nil {
		t.Fatal("expected failure")
	}
	clock.Advance(6 * time.Second)
	if err := invokeBreaker(breaker, invoker); err == nil {
		t.Fatal("expected failure")
	}
	clock.Advance(6 * time.Second)
	if err := invokeBreaker(breaker, invoker); err == nil {
		t.Fatal("expected failure")
	}
	if breaker.State() != StateClosed {
		t.Fatalf("state = %v, want CLOSED after old failures expired", breaker.State())
	}
}

func TestCircuitBreakerOpenRejectsWithoutCall(t *testing.T) {
	breaker, clock := newTestBreaker(CircuitBreakerConfig{FailureThreshold: 2})
	openBreaker(t, breaker, clock)

	invoker, calls := countingInvoker(nil)
	err := invokeBreaker(breaker, invoker)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable", status.Code(err))
	}
	if !strings.Contains(status.Convert(err).Message(), "circuit breaker") {
		t.Fatalf("message = %q, want circuit breaker text", status.Convert(err).Message())
	}
	if *calls != 0 {
		t.Fatalf("calls = %d, want 0", *calls)
	}
}

func TestCircuitBreakerHalfOpenAllowsSingleProbe(t *testing.T) {
	breaker, clock := newTestBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		OpenTimeout:      10 * time.Second,
	})
	openBreaker(t, breaker, clock)
	clock.Advance(10 * time.Second)

	started := make(chan struct{})
	release := make(chan error, 1)
	probeInvoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		close(started)
		return <-release
	}
	probeDone := make(chan error, 1)
	go func() {
		probeDone <- invokeBreaker(breaker, probeInvoker)
	}()

	<-started
	if breaker.State() != StateHalfOpen {
		t.Fatalf("state = %v, want HALF_OPEN", breaker.State())
	}

	secondInvoker, secondCalls := countingInvoker(nil)
	if err := invokeBreaker(breaker, secondInvoker); status.Code(err) != codes.Unavailable {
		t.Fatalf("second call code = %v, want Unavailable", status.Code(err))
	}
	if *secondCalls != 0 {
		t.Fatalf("second calls = %d, want 0", *secondCalls)
	}

	release <- nil
	if err := <-probeDone; err != nil {
		t.Fatalf("probe: %v", err)
	}
	if breaker.State() != StateClosed {
		t.Fatalf("state = %v, want CLOSED", breaker.State())
	}
}

func TestCircuitBreakerHalfOpenFailureReopens(t *testing.T) {
	breaker, clock := newTestBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		OpenTimeout:      10 * time.Second,
	})
	openBreaker(t, breaker, clock)
	clock.Advance(10 * time.Second)

	probeInvoker, _ := countingInvoker(status.Error(codes.Unavailable, "still down"))
	if err := invokeBreaker(breaker, probeInvoker); status.Code(err) != codes.Unavailable {
		t.Fatalf("probe code = %v, want Unavailable", status.Code(err))
	}
	if breaker.State() != StateOpen {
		t.Fatalf("state = %v, want OPEN", breaker.State())
	}

	invoker, calls := countingInvoker(nil)
	if err := invokeBreaker(breaker, invoker); status.Code(err) != codes.Unavailable {
		t.Fatalf("call code = %v, want Unavailable", status.Code(err))
	}
	if *calls != 0 {
		t.Fatalf("calls = %d, want 0", *calls)
	}
}

func TestCircuitBreakerBusinessErrorsDoNotOpen(t *testing.T) {
	breaker, _ := newTestBreaker(CircuitBreakerConfig{FailureThreshold: 3})

	invoker, _ := countingInvoker(status.Error(codes.NotFound, "not found"))
	for i := 0; i < 10; i++ {
		if err := invokeBreaker(breaker, invoker); status.Code(err) != codes.NotFound {
			t.Fatalf("code = %v, want NotFound", status.Code(err))
		}
	}
	if breaker.State() != StateClosed {
		t.Fatalf("state = %v, want CLOSED", breaker.State())
	}
}

func TestCircuitBreakerBusinessErrorClosesHalfOpen(t *testing.T) {
	breaker, clock := newTestBreaker(CircuitBreakerConfig{
		FailureThreshold: 2,
		OpenTimeout:      10 * time.Second,
	})
	openBreaker(t, breaker, clock)
	clock.Advance(10 * time.Second)

	invoker, _ := countingInvoker(status.Error(codes.NotFound, "not found"))
	if err := invokeBreaker(breaker, invoker); status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
	if breaker.State() != StateClosed {
		t.Fatalf("state = %v, want CLOSED", breaker.State())
	}
}

func TestCircuitBreakerCountsLogicalFailureOnceWithRetry(t *testing.T) {
	breaker, _ := newTestBreaker(CircuitBreakerConfig{FailureThreshold: 2})

	invoker, calls := countingInvoker(status.Error(codes.Unavailable, "down"))
	withRetry := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		retry := UnaryRetryInterceptor(RetryConfig{MaxAttempts: 3})
		return retry(ctx, method, req, reply, cc, invoker, opts...)
	}

	if err := invokeBreaker(breaker, withRetry); status.Code(err) != codes.Unavailable {
		t.Fatalf("first call code = %v, want Unavailable", status.Code(err))
	}
	if *calls != 3 {
		t.Fatalf("calls after first logical failure = %d, want 3", *calls)
	}
	if breaker.State() != StateClosed {
		t.Fatalf("state = %v, want CLOSED after one logical failure", breaker.State())
	}

	if err := invokeBreaker(breaker, withRetry); status.Code(err) != codes.Unavailable {
		t.Fatalf("second call code = %v, want Unavailable", status.Code(err))
	}
	if *calls != 6 {
		t.Fatalf("calls after second logical failure = %d, want 6", *calls)
	}
	if breaker.State() != StateOpen {
		t.Fatalf("state = %v, want OPEN", breaker.State())
	}

	if err := invokeBreaker(breaker, withRetry); status.Code(err) != codes.Unavailable {
		t.Fatalf("open call code = %v, want Unavailable", status.Code(err))
	}
	if *calls != 6 {
		t.Fatalf("calls after open = %d, want 6", *calls)
	}
}
