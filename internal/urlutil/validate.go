package urlutil

import (
	"fmt"
	"net"
	"net/url"
)

// privateRanges lists IP networks that must not be reachable via user-supplied URLs.
var privateRanges []*net.IPNet

func init() {
	blocks := []string{
		"0.0.0.0/8",          // "This" network
		"10.0.0.0/8",         // RFC 1918 private
		"100.64.0.0/10",      // Shared address space (RFC 6598)
		"127.0.0.0/8",        // Loopback
		"169.254.0.0/16",     // Link-local / AWS instance metadata
		"172.16.0.0/12",      // RFC 1918 private
		"192.0.0.0/24",       // IETF protocol assignments
		"192.168.0.0/16",     // RFC 1918 private
		"198.18.0.0/15",      // Benchmarking (RFC 2544)
		"198.51.100.0/24",    // TEST-NET-2 (RFC 5737)
		"203.0.113.0/24",     // TEST-NET-3 (RFC 5737)
		"224.0.0.0/4",        // Multicast
		"240.0.0.0/4",        // Reserved
		"255.255.255.255/32", // Broadcast
		"::1/128",            // IPv6 loopback
		"fc00::/7",           // IPv6 unique local
		"fe80::/10",          // IPv6 link-local
		"ff00::/8",           // IPv6 multicast
	}
	for _, b := range blocks {
		_, network, _ := net.ParseCIDR(b)
		privateRanges = append(privateRanges, network)
	}
}

// ValidatePublicURL returns an error if rawURL:
//   - is not an http or https URL
//   - resolves to a private, loopback, or link-local IP address
//
// This prevents SSRF attacks where a user-supplied URL targets internal services
// (e.g. the AWS instance metadata endpoint at 169.254.169.254, or localhost).
func ValidatePublicURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme %q is not allowed; only http and https are supported", u.Scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("URL has no host")
	}

	// Resolve the hostname to IP addresses.
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return fmt.Errorf("could not resolve host %q: %w", hostname, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("host %q resolved to no addresses", hostname)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			return fmt.Errorf("resolved address %q is not a valid IP", addr)
		}
		if isPrivate(ip) {
			return fmt.Errorf("URL resolves to a private or reserved IP address (%s); only public endpoints are allowed", ip)
		}
	}

	return nil
}

func isPrivate(ip net.IP) bool {
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
