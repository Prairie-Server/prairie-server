package livetv

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

const (
	mediaFetchTimeout   = 30 * time.Second
	mediaFetchRedirects = 3
)

// NewMediaHTTPClient builds an HTTP client for tuner/guide/stream fetches.
// Private LAN destinations are allowed (HDHomeRun), but cloud-metadata IPs are
// blocked at dial time and redirects are re-validated + bounded.
func NewMediaHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("livetv dial: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("livetv dial: invalid address %q", address)
			}
			if isBlockedMetadataIP(ip) {
				return errors.New("livetv dial: metadata addresses are not allowed")
			}
			return nil
		},
	}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: mediaFetchTimeout,
	}
	return &http.Client{
		Timeout:   mediaFetchTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= mediaFetchRedirects {
				return errors.New("too many redirects")
			}
			if err := ValidateMediaFetchURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}
