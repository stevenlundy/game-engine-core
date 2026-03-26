// Package tls provides helpers for loading TLS credentials from a certificate
// and private key file pair.
//
// # Generating a self-signed certificate for local development
//
// Run the following command to create a key + cert valid for 365 days:
//
//	openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem \
//	  -days 365 -nodes -subj '/CN=localhost'
//
// Then start the server with:
//
//	TLS_CERT=cert.pem TLS_KEY=key.pem ./server
package tls

import (
	"fmt"

	"google.golang.org/grpc/credentials"
)

// LoadServerCredentials loads a TLS certificate and private key from the
// supplied file paths and returns gRPC [credentials.TransportCredentials]
// suitable for use as a server option.
func LoadServerCredentials(certFile, keyFile string) (credentials.TransportCredentials, error) {
	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("tls: failed to load server credentials from cert=%q key=%q: %w", certFile, keyFile, err)
	}
	return creds, nil
}

// LoadClientCredentials loads a CA certificate from the supplied file path and
// returns gRPC [credentials.TransportCredentials] suitable for use as a dial
// option when connecting to a server whose certificate is signed by that CA.
func LoadClientCredentials(caFile string) (credentials.TransportCredentials, error) {
	creds, err := credentials.NewClientTLSFromFile(caFile, "")
	if err != nil {
		return nil, fmt.Errorf("tls: failed to load client credentials from ca=%q: %w", caFile, err)
	}
	return creds, nil
}
