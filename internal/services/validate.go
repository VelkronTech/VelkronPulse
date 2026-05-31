package services

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"unicode"
)

const (
	MaxEndpointNameLen = 128
	MaxEndpointURLLen  = 2048
)

var metadataAddrs = []string{
	"169.254.169.254",
	"169.254.170.2",
	"fd00:ec2::254",
}

// ValidateEndpointInput checks name and target before persisting or probing.
func ValidateEndpointInput(name, endpointType, rawTarget string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > MaxEndpointNameLen {
		return fmt.Errorf("name must be at most %d characters", MaxEndpointNameLen)
	}
	for _, r := range name {
		if r == '\n' || r == '\r' || r == '\t' || unicode.IsControl(r) {
			return fmt.Errorf("name contains invalid characters")
		}
	}

	rawTarget = strings.TrimSpace(rawTarget)
	if rawTarget == "" {
		return fmt.Errorf("URL is required")
	}
	if len(rawTarget) > MaxEndpointURLLen {
		return fmt.Errorf("URL must be at most %d characters", MaxEndpointURLLen)
	}

	switch endpointType {
	case "http":
		return validateHTTPTarget(rawTarget)
	case "tcp":
		return validateTCPTarget(rawTarget)
	default:
		return fmt.Errorf("type must be http or tcp")
	}
}

// ValidateProbeTarget re-validates a stored target before each probe (DNS rebinding defense).
func ValidateProbeTarget(endpointType, rawTarget string) error {
	switch endpointType {
	case "http":
		return validateHTTPTarget(rawTarget)
	case "tcp":
		return validateTCPTarget(rawTarget)
	default:
		return fmt.Errorf("invalid endpoint type")
	}
}

func validateHTTPTarget(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if u.User != nil {
		return fmt.Errorf("URL credentials are not allowed")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("host is required")
	}
	return validateResolvedHost(host)
}

func validateTCPTarget(raw string) error {
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return fmt.Errorf("TCP target must use host:port format")
	}
	if host == "" || port == "" {
		return fmt.Errorf("TCP target must use host:port format")
	}
	return validateResolvedHost(host)
}

func validateResolvedHost(host string) error {
	if isBlockedLiteralHost(host) {
		return fmt.Errorf("target host is not allowed")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host")
	}
	if len(ips) == 0 {
		return fmt.Errorf("cannot resolve host")
	}

	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("target resolves to a blocked address")
		}
	}
	return nil
}

func isBlockedLiteralHost(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	for _, blocked := range metadataAddrs {
		if host == blocked {
			return true
		}
	}
	return false
}

func isBlockedIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok || !addr.IsValid() {
		return true
	}
	addr = addr.Unmap()

	if addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}

	// Block cloud metadata and link-local ranges used for SSRF (allow normal private/loopback for local monitoring).
	if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return true
	}

	for _, blocked := range metadataAddrs {
		if blockedAddr, err := netip.ParseAddr(blocked); err == nil && addr == blockedAddr.Unmap() {
			return true
		}
	}

	return false
}
