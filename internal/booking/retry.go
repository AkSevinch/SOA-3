package booking

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultMaxAttempts = 3
	defaultBaseDelay   = 100 * time.Millisecond
)

type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: defaultMaxAttempts,
		BaseDelay:   defaultBaseDelay,
	}
}

func UnaryRetryInterceptor(cfg RetryConfig) grpc.UnaryClientInterceptor {
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 1
	}
	if cfg.BaseDelay < 0 {
		cfg.BaseDelay = 0
	}

	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var err error
		for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
			err = invoker(ctx, method, req, reply, cc, opts...)
			if err == nil {
				return nil
			}

			code := status.Code(err)
			if !retryable(code) || attempt == cfg.MaxAttempts {
				return err
			}

			delay := cfg.BaseDelay * time.Duration(1<<(attempt-1))
			log.Printf("retry %s attempt %d/%d after %s: %s", method, attempt, cfg.MaxAttempts, delay, code)

			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return status.FromContextError(ctx.Err()).Err()
			case <-timer.C:
			}
		}
		return err
	}
}

func retryable(code codes.Code) bool {
	return code == codes.Unavailable || code == codes.DeadlineExceeded
}
