package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	productionShutdownTimeout = 10 * time.Second
	readHeaderTimeout         = 5 * time.Second
	readTimeout               = 10 * time.Second
	writeTimeout              = 10 * time.Second
	idleTimeout               = 60 * time.Second
)

var (
	errHTTPListen       = errors.New("Admin HTTP listener failed")
	errHTTPServe        = errors.New("Admin HTTP server failed")
	errHTTPForceClose   = errors.New("Admin HTTP force close failed")
	errApplicationClose = errors.New("Managed Data Source close failed")
)

// runtimeApplication is the process module's small interface. The production
// Admin application and the external-process test adapter both sit at this seam.
type runtimeApplication interface {
	Handler() http.Handler
	Close() error
}

func newAdminHTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// serveAdmin owns the complete HTTP process lifetime, including signal-driven
// bounded shutdown and the final application/database close.
func serveAdmin(application runtimeApplication, address string, shutdownTimeout time.Duration) (returnErr error) {
	defer func() {
		if err := application.Close(); err != nil {
			returnErr = errors.Join(returnErr, errApplicationClose)
		}
	}()

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return errHTTPListen
	}

	server := newAdminHTTPServer(address, application.Handler())
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case serveErr := <-serverErrors:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return errHTTPServe
		}
		return nil
	case <-signals:
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		shutdownErr := server.Shutdown(shutdownContext)
		cancel()
		if shutdownErr != nil {
			if closeErr := server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
				return errHTTPForceClose
			}
		}
		serveErr := <-serverErrors
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return errHTTPServe
		}
		return nil
	}
}
