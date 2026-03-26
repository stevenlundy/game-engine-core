//go:build tools

// Package tools pins build-time and indirect dependencies so they are
// recorded in go.mod and go.sum even before the packages that use them
// are implemented. Remove this file once the real imports exist.
package tools

import (
	_ "google.golang.org/grpc"
	_ "google.golang.org/protobuf/proto"
)
