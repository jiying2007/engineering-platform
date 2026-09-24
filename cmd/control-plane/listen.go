package main

import "net"

// The bootstrap API has no production authentication. Network exposure must be
// explicit; a PostgreSQL backend does not make this a trusted remote endpoint.
func controlPlaneAddress(host, port string) string {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "8080"
	}
	return net.JoinHostPort(host, port)
}
