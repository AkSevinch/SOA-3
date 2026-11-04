package auth

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestUnaryServerInterceptor(t *testing.T) {
	const key = "secret"

	tests := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{
			name: "missing metadata",
			ctx:  context.Background(),
			want: codes.Unauthenticated,
		},
		{
			name: "wrong key",
			ctx:  metadata.NewIncomingContext(context.Background(), metadata.Pairs(Header, "wrong")),
			want: codes.Unauthenticated,
		},
		{
			name: "valid key",
			ctx:  metadata.NewIncomingContext(context.Background(), metadata.Pairs(Header, key)),
			want: codes.OK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			_, err := UnaryServerInterceptor(key)(tt.ctx, nil, nil, func(context.Context, any) (any, error) {
				called = true
				return nil, nil
			})
			if code := status.Code(err); code != tt.want {
				t.Fatalf("code = %v, want %v", code, tt.want)
			}
			if tt.want == codes.OK && !called {
				t.Fatal("handler was not called")
			}
			if tt.want != codes.OK && called {
				t.Fatal("handler was called")
			}
		})
	}
}

func TestUnaryServerInterceptorRejectsEmptyExpectedKey(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(Header, "any"))
	_, err := UnaryServerInterceptor("")(ctx, nil, nil, func(context.Context, any) (any, error) {
		return nil, nil
	})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", code)
	}
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s fakeServerStream) Context() context.Context {
	return s.ctx
}

func TestStreamServerInterceptor(t *testing.T) {
	const key = "secret"
	interceptor := StreamServerInterceptor(key)

	if err := interceptor(nil, fakeServerStream{ctx: context.Background()}, nil, func(any, grpc.ServerStream) error {
		return nil
	}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing key code = %v, want Unauthenticated", status.Code(err))
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(Header, key))
	if err := interceptor(nil, fakeServerStream{ctx: ctx}, nil, func(any, grpc.ServerStream) error {
		return nil
	}); err != nil {
		t.Fatalf("valid key: %v", err)
	}
}

func TestUnaryClientInterceptorAddsMetadata(t *testing.T) {
	const key = "secret"

	var got string
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("outgoing metadata is missing")
		}
		values := md.Get(Header)
		if len(values) != 1 {
			t.Fatalf("values = %v, want one", values)
		}
		got = values[0]
		return nil
	}

	err := UnaryClientInterceptor(key)(context.Background(), "/test.Service/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("interceptor: %v", err)
	}
	if got != key {
		t.Fatalf("key = %q, want %q", got, key)
	}
}
