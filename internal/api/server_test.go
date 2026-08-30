package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"raptor-transit/internal/gtfs"
	"raptor-transit/internal/raptor"
	"raptor-transit/internal/timetable"
)

// Fixture recap (internal/gtfs/testdata/EXPECTED.md):
//   R1_T1 (WKDY, route 10):   S1 08:00 -> S2 -> S3 08:10 -> S4 08:18 -> S5 08:26
//   R1_T2 (WKDY, route 10):   S1 09:15 -> S2 -> S3 09:25
//   R2_OWL1 (SPECIAL, route 84): S1 25:30 -> S2 -> S4 25:45 (overnight)
//   WKDY: Mon-Fri 2026, removed 20261225. SPECIAL: 20260704, 20261225,
//   20261231 only.
// 20260310 is a Tuesday (WKDY runs). 20260704 is a Saturday (SPECIAL only).

const (
	weekday    = "20260310"
	specialDay = "20260704"
	// A date inside 2026 that WKDY covers but where nothing was removed,
	// versus one outside every service's window entirely.
	outsideFeed = "20300101"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatal(err)
	}
	tt := timetable.Build(feed)
	// No footpaths: the fixture's stops are kilometres apart, and these
	// tests are about the HTTP layer, not walking.
	return NewServer(tt, raptor.New(tt, nil))
}

// get issues a request against the mounted routes and decodes the body.
func get(t *testing.T, s *Server, target string, into any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if into != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			t.Fatalf("decode %s: %v (body %q)", target, err, rec.Body.String())
		}
	}
	return rec
}

func TestRouteHappyPath(t *testing.T) {
	s := testServer(t)
	var resp RouteResponse
	rec := get(t, s, "/route?from=S1&to=S5&at=07:00:00&date="+weekday, &resp)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if len(resp.Journeys) == 0 {
		t.Fatal("no journeys returned")
	}

	j := resp.Journeys[0]
	if j.Arrival != "08:26:00" {
		t.Errorf("arrival = %q, want 08:26:00", j.Arrival)
	}
	if j.Departure != "08:00:00" {
		t.Errorf("departure = %q, want 08:00:00", j.Departure)
	}
	// Duration is boarding to arrival (08:00 -> 08:26), excluding the hour
	// spent waiting at the stop after the 07:00 request time.
	if want := int32(1560); j.DurationSeconds != want {
		t.Errorf("duration = %d, want %d", j.DurationSeconds, want)
	}
	if j.Transfers != 0 {
		t.Errorf("transfers = %d, want 0", j.Transfers)
	}
	if len(j.Legs) != 1 {
		t.Fatalf("legs = %d, want 1", len(j.Legs))
	}

	leg := j.Legs[0]
	if leg.Mode != "transit" {
		t.Errorf("mode = %q, want transit", leg.Mode)
	}
	if leg.Route != "10" {
		t.Errorf("route = %q, want 10 (route_short_name)", leg.Route)
	}
	if leg.TripID != "R1_T1" {
		t.Errorf("trip_id = %q, want R1_T1", leg.TripID)
	}
	if leg.From.ID != "S1" || leg.To.ID != "S5" {
		t.Errorf("leg %s->%s, want S1->S5", leg.From.ID, leg.To.ID)
	}
	if leg.From.Name != "Pike St & 3rd Ave" {
		t.Errorf("from name = %q, want the feed's display name", leg.From.Name)
	}
	if resp.Origin.ID != "S1" || resp.Destination.ID != "S5" {
		t.Errorf("origin/destination = %s/%s, want S1/S5", resp.Origin.ID, resp.Destination.ID)
	}
}

// Stop names must work everywhere stop_ids do — that is the point of Phase 3.
func TestRouteByStopName(t *testing.T) {
	s := testServer(t)
	var resp RouteResponse
	rec := get(t, s, "/route?from=Pike+St+%26+3rd+Ave&to=Northgate&at=07:00:00&date="+weekday, &resp)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if resp.Origin.ID != "S1" || resp.Destination.ID != "S5" {
		t.Fatalf("resolved to %s->%s, want S1->S5", resp.Origin.ID, resp.Destination.ID)
	}
	if len(resp.Journeys) == 0 {
		t.Error("no journeys for a name-based query that works by ID")
	}
}

// An overnight trip must report a >24h clock and the next_day flag rather
// than wrapping to a time that looks earlier than its departure.
func TestRouteOvernightLeg(t *testing.T) {
	s := testServer(t)
	var resp RouteResponse
	get(t, s, "/route?from=S1&to=S4&at=20:00:00&date="+specialDay, &resp)

	if len(resp.Journeys) == 0 {
		t.Fatal("no journeys; expected the SPECIAL owl trip")
	}
	j := resp.Journeys[0]
	if j.Arrival != "25:45:00" {
		t.Errorf("arrival = %q, want 25:45:00", j.Arrival)
	}
	if j.ArrivalSeconds != 92700 {
		t.Errorf("arrival_seconds = %d, want 92700", j.ArrivalSeconds)
	}
	leg := j.Legs[len(j.Legs)-1]
	if !leg.NextDay {
		t.Error("next_day = false on a leg arriving at 25:45")
	}
	if leg.Route != "84" {
		t.Errorf("route = %q, want 84", leg.Route)
	}
}

// A well-formed query with no answer is a 200 with an empty list, not an
// error — "no bus goes there" is a result.
func TestRouteUnreachableIsEmptyNot404(t *testing.T) {
	s := testServer(t)
	var resp RouteResponse
	// S5 is the last stop of the only route serving it; nothing runs back.
	rec := get(t, s, "/route?from=S5&to=S1&at=07:00:00&date="+weekday, &resp)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(resp.Journeys) != 0 {
		t.Errorf("got %d journeys, want 0", len(resp.Journeys))
	}
	// Encoded as [] rather than null, so clients can iterate unconditionally.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["journeys"]) != "[]" {
		t.Errorf("journeys encoded as %s, want []", raw["journeys"])
	}
}

// A date the feed doesn't cover is still 200, but says why it's empty.
func TestRouteOutsideFeedWindowExplains(t *testing.T) {
	s := testServer(t)
	var resp RouteResponse
	rec := get(t, s, "/route?from=S1&to=S5&at=07:00:00&date="+outsideFeed, &resp)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(resp.Journeys) != 0 {
		t.Fatalf("got %d journeys, want 0", len(resp.Journeys))
	}
	if len(resp.Notes) == 0 {
		t.Error("no note explaining the date is outside the feed window")
	}
}

func TestRouteBadRequests(t *testing.T) {
	s := testServer(t)
	cases := []struct {
		name   string
		target string
		status int
	}{
		{"missing from", "/route?to=S5&at=08:00:00&date=" + weekday, http.StatusBadRequest},
		{"missing to", "/route?from=S1&at=08:00:00&date=" + weekday, http.StatusBadRequest},
		{"missing at", "/route?from=S1&to=S5&date=" + weekday, http.StatusBadRequest},
		{"missing date", "/route?from=S1&to=S5&at=08:00:00", http.StatusBadRequest},
		{"blank from", "/route?from=+&to=S5&at=08:00:00&date=" + weekday, http.StatusBadRequest},
		{"bad time", "/route?from=S1&to=S5&at=8am&date=" + weekday, http.StatusBadRequest},
		{"bad date", "/route?from=S1&to=S5&at=08:00:00&date=March", http.StatusBadRequest},
		{"unknown stop", "/route?from=S1&to=Ballard&at=08:00:00&date=" + weekday, http.StatusNotFound},
		{"ambiguous name", "/route?from=Station&to=S5&at=08:00:00&date=" + weekday, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var resp ErrorResponse
			rec := get(t, s, c.target, &resp)
			if rec.Code != c.status {
				t.Errorf("status = %d, want %d (body %q)", rec.Code, c.status, rec.Body.String())
			}
			if resp.Error == "" {
				t.Error("error body has no message")
			}
		})
	}
}

// An ambiguous name is a dead end unless the response says what to pick.
func TestRouteAmbiguousReturnsCandidates(t *testing.T) {
	s := testServer(t)
	var resp ErrorResponse
	get(t, s, "/route?from=Station&to=S5&at=08:00:00&date="+weekday, &resp)
	if len(resp.Candidates) < 2 {
		t.Fatalf("got %d candidates, want at least 2", len(resp.Candidates))
	}
	for _, c := range resp.Candidates {
		if c.ID == "" || c.Name == "" {
			t.Errorf("candidate %+v missing id or name", c)
		}
	}
}

func TestStopsSearch(t *testing.T) {
	s := testServer(t)

	t.Run("prefix", func(t *testing.T) {
		var resp StopsResponse
		rec := get(t, s, "/stops?q=West", &resp)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if len(resp.Stops) != 1 || resp.Stops[0].ID != "S2" {
			t.Fatalf("got %+v, want [S2]", resp.Stops)
		}
	})

	t.Run("limit", func(t *testing.T) {
		var resp StopsResponse
		get(t, s, "/stops?q=Station&limit=2", &resp)
		if len(resp.Stops) != 2 {
			t.Errorf("got %d stops, want 2", len(resp.Stops))
		}
	})

	t.Run("no match is empty not 404", func(t *testing.T) {
		var resp StopsResponse
		rec := get(t, s, "/stops?q=Ballard", &resp)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
		if len(resp.Stops) != 0 {
			t.Errorf("got %d stops, want 0", len(resp.Stops))
		}
	})

	t.Run("bad requests", func(t *testing.T) {
		for _, target := range []string{"/stops", "/stops?q=", "/stops?q=+", "/stops?q=West&limit=0", "/stops?q=West&limit=abc"} {
			rec := get(t, s, target, nil)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, want 400", target, rec.Code)
			}
		}
	})
}

// The engine and index are shared across requests with no locking; this is
// the assertion that sharing is actually safe. Meaningful under -race.
func TestConcurrentRequests(t *testing.T) {
	s := testServer(t)
	targets := []string{
		"/route?from=S1&to=S5&at=07:00:00&date=" + weekday,
		"/route?from=S1&to=S3&at=09:00:00&date=" + weekday,
		"/route?from=S1&to=S4&at=20:00:00&date=" + specialDay,
		"/stops?q=Station",
		"/stops?q=West",
	}
	handler := s.Routes()

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, targets[i%len(targets)], nil))
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		}(i)
	}
	wg.Wait()
}

// Repeated identical queries must produce identical bodies — a shared engine
// that leaked state between queries would show up here.
func TestQueryIsRepeatable(t *testing.T) {
	s := testServer(t)
	target := "/route?from=S1&to=S5&at=07:00:00&date=" + weekday
	first := get(t, s, target, nil).Body.String()
	for i := 0; i < 5; i++ {
		if got := get(t, s, target, nil).Body.String(); got != first {
			t.Fatalf("run %d differs:\n %s\n %s", i, first, got)
		}
	}
}
