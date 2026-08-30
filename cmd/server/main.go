// Command server runs the raptor-transit HTTP API.
//
//	go run ./cmd/server -timetable data/timetable.gob -addr :8080
//
// The 17.8 MB timetable gob and the ~14k walking footpaths are loaded and
// generated exactly once at startup, not per request: the load takes on the
// order of a second and the footpath grid a good fraction of one, which
// would dwarf the ~14 ms query itself. raptor.Engine is safe for concurrent
// use, so every request shares the one instance.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"raptor-transit/internal/api"
	"raptor-transit/internal/raptor"
	"raptor-transit/internal/timetable"
	"raptor-transit/internal/transfers"
)

// Walking-transfer parameters, matching cmd/route so the CLI and API give
// identical answers.
const (
	walkRadiusMeters = 200
	walkSpeedMPS     = 1.2
)

func main() {
	ttPath := flag.String("timetable", "data/timetable.gob", "path to timetable gob (from cmd/ingest)")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	// slog is Go's standard structured logger. The JSON handler writes one
	// JSON object per log line to stdout, similar to Python's structlog.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	start := time.Now()
	tt, err := timetable.Load(*ttPath)
	if err != nil {
		// Without a timetable the server can do nothing useful, so fail at
		// startup rather than serving 500s and looking healthy to a load
		// balancer.
		slog.Error("load timetable", "path", *ttPath, "error", err,
			"hint", "run `make ingest` first")
		os.Exit(1)
	}
	paths, skipped := transfers.Generate(tt.StopLats, tt.StopLons, walkRadiusMeters, walkSpeedMPS)
	if skipped > 0 {
		slog.Warn("stops skipped for bad coordinates", "count", skipped)
	}
	engine := raptor.New(tt, paths)
	slog.Info("timetable loaded",
		"stops", len(tt.StopIDs),
		"patterns", len(tt.Patterns),
		"footpaths", len(paths),
		"took", time.Since(start).String(),
	)

	mux := api.NewServer(tt, engine).Routes()
	mux.HandleFunc("GET /healthz", handleHealthz)

	server := &http.Server{
		Addr:    *addr,
		Handler: logRequests(mux),
		// A slow or stalled client must not be able to hold a connection
		// open indefinitely. Queries are milliseconds, so these are
		// generous.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
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
// JSON body; a load balancer or container runtime polls this endpoint. The
// server only starts listening after the timetable loads, so a 200 here
// means the service can actually answer queries.
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// logRequests wraps a handler with one structured log line per request,
// including the status code (captured via a small ResponseWriter wrapper,
// since net/http doesn't expose it after the fact).
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"query", r.URL.RawQuery,
			"status", rec.status,
			"took_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
