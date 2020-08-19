package dns

import (
	"net"
)

const COMMON_DNS_PORT = 53

type FakeDns interface {
	// GenerateFakeResponse generates a fake dns response for the specify request.
	GenerateFakeResponse(request []byte) ([]byte, error)

	// QueryDomain returns the corresponding domain for the given IP.
	QueryDomain(ip net.IP) string

	// IsFakeIP checks if the given ip is a fake IP.
	IsFakeIP(ip net.IP) bool
}
