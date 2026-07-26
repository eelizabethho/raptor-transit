// Command server runs the raptor-transit HTTP API server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// slog is Go's standard structured logger. The JSON handler writes one
	// JSON object per log line to stdout, similar to Python's structlog.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// A ServeMux routes incoming requests to handlers by path.
	// The "GET " prefix (Go 1.22+) restricts the route to that HTTP method.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Run the server in a goroutine (a lightweight thread) so the main
	// goroutine is free to wait for a shutdown signal below.
	go func() {
		slog.Info("server starting", "addr", server.Addr)
		// ListenAndServe blocks until the server stops. ErrServerClosed is
		// the expected "error" after a graceful Shutdown, so we ignore it.
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Block until we receive SIGINT (Ctrl-C) or SIGTERM (e.g. from Docker).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	// Give in-flight requests up to 10 seconds to finish.
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// handleHealthz reports service health. It always returns 200 with a small
// JSON body; a load balancer or container runtime polls this endpoint.
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
