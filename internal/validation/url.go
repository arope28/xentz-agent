package validation

import (
	"fmt"
	"net"
	"net/url"
)

// ValidateServerURL validates server URL to prevent SSRF attacks.
// It ensures:
// - Only http/https schemes are allowed
// - localhost is blocked (127.0.0.1, ::1, localhost)
// - Private RFC1918 IPs are allowed (for legitimate internal control plane deployments)
//
// Rationale for allowing private IPs:
// Many enterprise deployments use internal control plane servers on private networks.
// Blocking private IPs would prevent legitimate use cases. The localhost restriction
// prevents SSRF attacks against the local machine while still allowing access to
// internal infrastructure.
func ValidateServerURL(serverURL string) error {
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	// Only allow http/https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("only http/https schemes allowed")
	}
	// Block localhost to prevent SSRF (private IPs allowed for legitimate internal servers)
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return fmt.Errorf("localhost not allowed (SSRF protection)")
	}
	return nil
}

// ValidateServerURLStrict validates server URL with strict SSRF protection.
// Unlike ValidateServerURL, this also blocks private RFC1918 IP addresses.
// Use this when you only want to allow public control plane servers.
func ValidateServerURLStrict(serverURL string) error {
	// First run standard validation
	if err := ValidateServerURL(serverURL); err != nil {
		return err
	}

	parsed, _ := url.Parse(serverURL) // Already validated above
	host := parsed.Hostname()

	// Check if host is an IP address
	ip := net.ParseIP(host)
	if ip == nil {
		// Not an IP, must be a hostname - allow it (DNS will resolve)
		return nil
	}

	// Check if IP is in private ranges (RFC1918)
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return fmt.Errorf("private IP addresses not allowed (strict SSRF protection): %s", host)
	}

	return nil
}
