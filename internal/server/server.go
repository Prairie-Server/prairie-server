// Package server provides HTTP server lifecycle management with graceful
// shutdown support for Prairie.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/prairie-server/prairie-server/internal/config"
)

// Server wraps an http.Server with lifecycle management and graceful shutdown.
type Server struct {
	httpServer *http.Server
	config     *config.Config
	listener   net.Listener
	stopCh     chan struct{}
	doneCh     chan struct{}
	once       sync.Once
}

// New creates a new Server with the given configuration and HTTP handler.
func New(cfg *config.Config, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:    cfg.Server.Listen,
			Handler: handler,
		},
		config: cfg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins serving HTTP requests. It binds the listener and starts
// serving in a background goroutine, returning immediately. Use Shutdown
// to stop the server gracefully.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.httpServer.Addr, err)
	}
	s.listener = ln

	go func() {
		defer close(s.doneCh)
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			// Log unexpected serve errors; ErrServerClosed is expected during
			// graceful shutdown.
			fmt.Printf("server: unexpected serve error: %v\n", err)
		}
	}()

	return nil
}

// Shutdown performs graceful shutdown in the ordered sequence:
//  1. Stop accepting new requests
//  2. Wait for active requests to complete (with timeout from context)
//  3. Signal all workers to stop via stopCh
//  4. Close
func (s *Server) Shutdown(ctx context.Context) error {
	var shutdownErr error

	s.once.Do(func() {
		// Step 1 & 2: http.Server.Shutdown stops accepting new connections
		// and waits for active requests to complete.
		shutdownErr = s.httpServer.Shutdown(ctx)

		// Step 3: Signal workers to stop.
		close(s.stopCh)
	})

	// Step 4: Wait for the serve goroutine to exit.
	select {
	case <-s.doneCh:
	case <-ctx.Done():
		if shutdownErr == nil {
			shutdownErr = ctx.Err()
		}
	}

	return shutdownErr
}

// ListenAddr returns the actual address the server is listening on. This is
// useful when the server was configured with ":0" to get an ephemeral port.
// Returns an empty string if the server has not been started.
func (s *Server) ListenAddr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// StopCh returns the channel that is closed when the server begins shutting
// down. Workers can select on this channel to know when to stop.
func (s *Server) StopCh() <-chan struct{} {
	return s.stopCh
}
