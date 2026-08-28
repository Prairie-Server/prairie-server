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

func newMediaTransport() *http.Transport {
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
	return &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: mediaFetchTimeout,
	}
}

func mediaCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= mediaFetchRedirects {
		return errors.New("too many redirects")
	}
	if err := ValidateMediaFetchURL(req.URL.String()); err != nil {
		return err
	}
	return nil
}

// NewMediaHTTPClient builds an HTTP client for tuner/guide metadata fetches.
// Private LAN destinations are allowed (HDHomeRun), but cloud-metadata IPs are
// blocked at dial time and redirects are re-validated + bounded.
// Do not use this client for long-lived MPEG-TS proxying — Client.Timeout would
// cut the body mid-stream; use NewStreamHTTPClient instead.
func NewMediaHTTPClient() *http.Client {
	return &http.Client{
		Timeout:       mediaFetchTimeout,
		Transport:     newMediaTransport(),
		CheckRedirect: mediaCheckRedirect,
	}
}

// NewStreamHTTPClient builds an HTTP client for long-lived live stream proxying.
// It keeps dial/header/redirect safeguards from NewMediaHTTPClient but omits
// Client.Timeout so active MPEG-TS body reads are not terminated early.
func NewStreamHTTPClient() *http.Client {
	return &http.Client{
		Transport:     newMediaTransport(),
		CheckRedirect: mediaCheckRedirect,
	}
}
