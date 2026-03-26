// Package auth provides gRPC interceptors for token-based authentication.
//
// A shared-secret token is expected in the gRPC metadata under the key
// "authorization" in the format "Bearer <token>". Both unary and streaming
// server interceptors are provided.
//
// # Quick Start
//
//	interceptor := auth.NewTokenInterceptor("my-secret-token")
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(interceptor.Unary()),
//	    grpc.StreamInterceptor(interceptor.Stream()),
//	)
package auth

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	// MetadataKey is the gRPC metadata key that must carry the bearer token.
	MetadataKey = "authorization"

	// bearerPrefix is the expected token prefix (case-sensitive).
	bearerPrefix = "Bearer "
)

// TokenInterceptor validates that every incoming gRPC call carries a known
// shared-secret token in the "authorization" metadata header.
//
// Construct with [NewTokenInterceptor].
type TokenInterceptor struct {
	token string
}

// NewTokenInterceptor creates a TokenInterceptor that validates the given
// shared-secret token. Requests without a matching "authorization: Bearer
// <token>" metadata header are rejected with codes.Unauthenticated.
//
// Panics if token is empty.
func NewTokenInterceptor(token string) *TokenInterceptor {
	if token == "" {
		panic("auth: token must not be empty")
	}
	return &TokenInterceptor{token: token}
}

// validate extracts and validates the bearer token from ctx metadata.
func (t *TokenInterceptor) validate(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get(MetadataKey)
	if len(values) == 0 {
		return status.Errorf(codes.Unauthenticated, "missing %q metadata key", MetadataKey)
	}

	// Accept the first value; compare after stripping the "Bearer " prefix.
	raw := values[0]
	if len(raw) <= len(bearerPrefix) || raw[:len(bearerPrefix)] != bearerPrefix {
		return status.Errorf(codes.Unauthenticated, "malformed authorization header: expected \"Bearer <token>\"")
	}

	provided := raw[len(bearerPrefix):]
	if provided != t.token {
		return status.Error(codes.Unauthenticated, "invalid token")
	}

	return nil
}

// Unary returns a [grpc.UnaryServerInterceptor] that rejects unauthenticated
// requests before they reach the handler.
func (t *TokenInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if err := t.validate(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// Stream returns a [grpc.StreamServerInterceptor] that rejects unauthenticated
// streaming calls before the handler is invoked.
func (t *TokenInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if err := t.validate(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}
