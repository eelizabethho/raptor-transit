package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"raptor-transit/internal/calendar"
	"raptor-transit/internal/gtfs"
	"raptor-transit/internal/raptor"
	"raptor-transit/internal/stopsearch"
	"raptor-transit/internal/timetable"
)

// defaultStopLimit caps GET /stops results when the client doesn't ask.
const defaultStopLimit = 10

// maxStopLimit bounds what a client can ask for, so one request can't be
// made to serialize all 6,400 stops.
const maxStopLimit = 100

// Server holds the immutable query-time state shared by all requests: the
// timetable, the RAPTOR engine, and the stop-name index. All three are
// built once at startup and never mutated, so the zero-synchronization
// sharing here is safe (raptor.Engine.Query allocates its own working
// state per call).
type Server struct {
	tt     *timetable.Timetable
	engine *raptor.Engine
	stops  *stopsearch.Index

	// delays, when set, supplies a live overlay for ?realtime=true. It is
	// a function rather than a stored snapshot because the snapshot is
	// replaced by the poller on every refresh: the handler must read the
	// current one per request, not capture one at startup.
	delays func() raptor.Delays
}

// NewServer builds a Server over an already-loaded timetable and engine.
func NewServer(tt *timetable.Timetable, engine *raptor.Engine) *Server {
	return &Server{tt: tt, engine: engine, stops: stopsearch.New(tt)}
}

// SetDelays installs a source of live delays, enabling ?realtime=true. The
// function is called once per realtime request and may return nil when no
// feed has been fetched yet. Call before serving; it is not synchronized.
func (s *Server) SetDelays(f func() raptor.Delays) { s.delays = f }

// Routes returns a mux with the API endpoints mounted. The caller adds
// operational endpoints (health, metrics) so this package stays about the
// transit API only.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /route", s.handleRoute)
	mux.HandleFunc("GET /stops", s.handleStops)
	return mux
}

// handleStops serves GET /stops?q=&limit= — stop-name autocomplete.
func (s *Server) handleStops(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if strings.TrimSpace(q) == "" {
		writeError(w, http.StatusBadRequest, "missing required parameter: q", nil)
		return
	}

	limit := defaultStopLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer", nil)
			return
		}
		limit = min(n, maxStopLimit)
	}

	matches := s.stops.Search(q, limit)
	resp := StopsResponse{Query: q, Stops: make([]StopRef, 0, len(matches))}
	for _, m := range matches {
		resp.Stops = append(resp.Stops, StopRef{ID: m.StopID, Name: m.Name})
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleRoute serves GET /route?from=&to=&at=&date=.
//
// from/to accept either a GTFS stop_id or a stop name. Status codes:
// 400 for malformed or missing parameters and ambiguous names, 404 for a
// stop that matches nothing, 200 with an empty journeys list when the query
// is well-formed but nothing is reachable — "no bus goes there" is an
// answer, not an error.
func (s *Server) handleRoute(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to, at, date := q.Get("from"), q.Get("to"), q.Get("at"), q.Get("date")

	for name, val := range map[string]string{"from": from, "to": to, "at": at, "date": date} {
		if strings.TrimSpace(val) == "" {
			writeError(w, http.StatusBadRequest, "missing required parameter: "+name, nil)
			return
		}
	}

	dep, err := gtfs.ParseTime(at)
	if err != nil || dep < 0 {
		writeError(w, http.StatusBadRequest, "bad 'at' value "+strconv.Quote(at)+" (want HH:MM:SS)", nil)
		return
	}
	day, err := calendar.ParseDate(date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad 'date' value "+strconv.Quote(date)+" (want YYYYMMDD)", nil)
		return
	}

	fromID, ok := s.resolveStop(w, "from", from)
	if !ok {
		return
	}
	toID, ok := s.resolveStop(w, "to", to)
	if !ok {
		return
	}

	// ?realtime=true opts into the live delay overlay. It is deliberately
	// outside the required-parameter loop above: absent means schedule.
	var (
		delays raptor.Delays
		rtNote string
		wantRT = q.Get("realtime") == "true"
	)
	if wantRT {
		switch {
		case s.delays == nil:
			rtNote = "realtime was requested but this server has no realtime feed configured; times are scheduled"
		default:
			if d := s.delays(); d != nil {
				delays = d
			} else {
				rtNote = "realtime was requested but no feed has been fetched yet; times are scheduled"
			}
		}
	}

	journeys, err := s.engine.QueryWith(fromID, toID, int32(dep), date, delays)
	if err != nil {
		// Stops and date are already validated above, so anything left is
		// a genuine server-side failure rather than bad input.
		slog.Error("query failed", "from", fromID, "to", toID, "at", at, "date", date, "error", err)
		writeError(w, http.StatusInternalServerError, "query failed", nil)
		return
	}

	fromIdx, _ := s.tt.StopIdx(fromID)
	toIdx, _ := s.tt.StopIdx(toID)
	depClock, _ := clock(int32(dep))
	resp := RouteResponse{
		Origin:      stopRef(s.tt, fromIdx),
		Destination: stopRef(s.tt, toIdx),
		Date:        date,
		DepartAfter: depClock,
		Journeys:    make([]Journey, 0, len(journeys)),
	}
	for _, j := range journeys {
		resp.Journeys = append(resp.Journeys, toWireJourney(s.tt, j))
	}

	// An empty result is much more often a date outside the feed window
	// than a genuinely unreachable pair, and the two deserve different
	// messages. Only worth checking when there's nothing to report.
	if len(resp.Journeys) == 0 && !calendar.InFeedWindow(s.tt.Services, day) {
		resp.Notes = append(resp.Notes,
			"no service runs on "+date+"; the loaded feed does not cover that date")
	}
	if rtNote != "" {
		resp.Notes = append(resp.Notes, rtNote)
	}
	resp.Realtime = wantRT && delays != nil

	writeJSON(w, http.StatusOK, resp)
}

// resolveStop turns a from/to parameter into a stop_id, writing the
// appropriate error response and returning false if it can't.
func (s *Server) resolveStop(w http.ResponseWriter, param, input string) (string, bool) {
	id, candidates, err := s.stops.Resolve(input)
	switch {
	case err == nil:
		return id, true
	case errors.Is(err, stopsearch.ErrNotFound):
		writeError(w, http.StatusNotFound, "no stop matches "+param+"="+strconv.Quote(input), nil)
	case errors.Is(err, stopsearch.ErrAmbiguous):
		refs := make([]StopRef, 0, len(candidates))
		for _, m := range candidates {
			refs = append(refs, StopRef{ID: m.StopID, Name: m.Name})
		}
		writeError(w, http.StatusBadRequest,
			strconv.Quote(input)+" matches several stops; retry with a stop_id", refs)
	default:
		writeError(w, http.StatusBadRequest, "bad "+param+" value", nil)
	}
	return "", false
}

// writeJSON writes v as the response body. An encoding failure after the
// header is written can't be turned into an error status, so it is logged
// and the client sees a truncated body — the honest outcome.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string, candidates []StopRef) {
	writeJSON(w, status, ErrorResponse{Error: msg, Candidates: candidates})
}
