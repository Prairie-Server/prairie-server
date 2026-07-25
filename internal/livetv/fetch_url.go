package livetv

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateMediaFetchURL guards server-side fetches of tuner/guide/stream URLs.
// Allows http(s) for LAN HDHomeRun devices, but blocks credentialed URLs and
// well-known cloud metadata endpoints (SSRF to instance metadata).
func ValidateMediaFetchURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%w: invalid URL", ErrInvalidArgument)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("%w: URL must use http or https", ErrInvalidArgument)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("%w: URL has no host", ErrInvalidArgument)
	}
	if parsed.User != nil {
		return fmt.Errorf("%w: URL must not embed credentials", ErrInvalidArgument)
	}
	lowerHost := strings.ToLower(host)
	switch lowerHost {
	case "metadata.google.internal", "metadata", "metadata.aws.internal":
		return fmt.Errorf("%w: metadata hosts are not allowed", ErrInvalidArgument)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedMetadataIP(ip) {
			return fmt.Errorf("%w: metadata addresses are not allowed", ErrInvalidArgument)
		}
	}
	return nil
}

func isBlockedMetadataIP(ip net.IP) bool {
	// AWS / Azure / GCP link-local metadata endpoints.
	if ip.Equal(net.ParseIP("169.254.169.254")) || ip.Equal(net.ParseIP("169.254.169.253")) {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		// Alibaba Cloud metadata.
		if ip4.Equal(net.ParseIP("100.100.100.200").To4()) {
			return true
		}
	}
	return false
}
