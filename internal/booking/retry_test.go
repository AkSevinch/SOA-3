package booking

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testRetryConfig() RetryConfig {
	return RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}
}

func TestRetryInterceptorSucceedsFirstAttempt(t *testing.T) {
	calls := 0
	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		return nil
	}

	err := UnaryRetryInterceptor(testRetryConfig())(context.Background(), "/test.Service/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestRetryInterceptorRetriesUnavailable(t *testing.T) {
	calls := 0
	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		if calls <= 2 {
			return status.Error(codes.Unavailable, "unavailable")
		}
		return nil
	}

	err := UnaryRetryInterceptor(testRetryConfig())(context.Background(), "/test.Service/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRetryInterceptorRetriesDeadlineExceeded(t *testing.T) {
	calls := 0
	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		if calls == 1 {
			return status.Error(codes.DeadlineExceeded, "deadline")
		}
		return nil
	}

	err := UnaryRetryInterceptor(testRetryConfig())(context.Background(), "/test.Service/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestRetryInterceptorStopsAtMaxAttempts(t *testing.T) {
	calls := 0
	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		return status.Error(codes.Unavailable, "unavailable")
	}

	err := UnaryRetryInterceptor(testRetryConfig())(context.Background(), "/test.Service/Method", nil, nil, nil, invoker)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable", status.Code(err))
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRetryInterceptorDoesNotRetryOtherCodes(t *testing.T) {
	for _, code := range []codes.Code{
		codes.InvalidArgument,
		codes.NotFound,
		codes.ResourceExhausted,
		codes.AlreadyExists,
		codes.Unauthenticated,
	} {
		t.Run(code.String(), func(t *testing.T) {
			calls := 0
			invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
				calls++
				return status.Error(code, "boom")
			}

			err := UnaryRetryInterceptor(testRetryConfig())(context.Background(), "/test.Service/Method", nil, nil, nil, invoker)
			if status.Code(err) != code {
				t.Fatalf("code = %v, want %v", status.Code(err), code)
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want 1", calls)
			}
		})
	}
}

func TestRetryInterceptorStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
		calls++
		cancel()
		return status.Error(codes.Unavailable, "unavailable")
	}

	cfg := RetryConfig{MaxAttempts: 3, BaseDelay: time.Second}
	err := UnaryRetryInterceptor(cfg)(ctx, "/test.Service/Method", nil, nil, nil, invoker)
	if status.Code(err) != codes.Canceled {
		t.Fatalf("code = %v, want Canceled", status.Code(err))
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}
