package auth

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const Header = "x-api-key"

func UnaryServerInterceptor(expected string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !authorized(ctx, expected) {
			return nil, status.Error(codes.Unauthenticated, "invalid api key")
		}
		return handler(ctx, req)
	}
}

func StreamServerInterceptor(expected string) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !authorized(stream.Context(), expected) {
			return status.Error(codes.Unauthenticated, "invalid api key")
		}
		return handler(srv, stream)
	}
}

func UnaryClientInterceptor(apiKey string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, Header, apiKey)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func authorized(ctx context.Context, expected string) bool {
	if expected == "" {
		return false
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	for _, value := range md.Get(Header) {
		if subtle.ConstantTimeCompare([]byte(value), []byte(expected)) == 1 {
			return true
		}
	}
	return false
}
