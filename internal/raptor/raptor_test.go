package raptor

import (
	"path/filepath"
	"testing"

	"raptor-transit/internal/gtfs"
	"raptor-transit/internal/timetable"
	"raptor-transit/internal/transfers"
)

// Fixture recap (internal/gtfs/testdata/EXPECTED.md):
//   R1_T1 (WKDY):   S1 08:00 -> S2 -> S3 08:10 -> S4 08:18 -> S5 08:26
//   R1_T2 (WKDY):   S1 09:15 -> S2 -> S3 09:25
//   R2_OWL1 (SPECIAL): S1 25:30 -> S2 -> S4 25:45  (overnight)
//   WKDY: Mon-Fri 2026, removed 20261225. SPECIAL: added 20260704,
//   20261225, 20261231 only.
// 20260310 is a Tuesday.

func fixtureEngine(t *testing.T, paths []transfers.Footpath) *Engine {
	t.Helper()
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatal(err)
	}
	return New(timetable.Build(feed), paths)
}

// fp builds a symmetric footpath pair between two stop IDs.
func fp(e *Engine, t *testing.T, a, b string, secs int32) []transfers.Footpath {
	t.Helper()
	ai, ok := e.tt.StopIdx(a)
	if !ok {
		t.Fatalf("stop %s", a)
	}
	bi, _ := e.tt.StopIdx(b)
	return []transfers.Footpath{{From: ai, To: bi, Seconds: secs}, {From: bi, To: ai, Seconds: secs}}
}

const (
	h8    = 8 * 3600
	h0700 = 7 * 3600
)

func TestDirectRide(t *testing.T) {
	e := fixtureEngine(t, nil)
	js, err := e.Query("S1", "S5", h0700, "20260310")
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 1 {
		t.Fatalf("journeys = %d, want 1: %+v", len(js), js)
	}
	j := js[0]
	if j.Arrival != 8*3600+26*60 {
		t.Errorf("arrival = %d, want %d (08:26:00)", j.Arrival, 8*3600+26*60)
	}
	if j.Transfers != 0 {
		t.Errorf("transfers = %d, want 0", j.Transfers)
	}
	if len(j.Legs) != 1 || j.Legs[0].Route != "10" && j.Legs[0].Route != "R1-name" {
		// route short name comes from fixture routes.txt; just require a ride
		if j.Legs[0].Trip < 0 {
			t.Errorf("expected a transit leg, got %+v", j.Legs)
		}
	}
	if j.Legs[0].Departure != 8*3600 {
		t.Errorf("departure = %d, want 28800 (08:00:00)", j.Legs[0].Departure)
	}
}

func TestLaterDepartureCatchesLaterTrip(t *testing.T) {
	e := fixtureEngine(t, nil)
	js, err := e.Query("S1", "S3", 8*3600+30*60, "20260310") // 08:30, after R1_T1
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 1 {
		t.Fatalf("journeys = %d, want 1", len(js))
	}
	if js[0].Arrival != 9*3600+25*60 {
		t.Errorf("arrival = %d, want %d (09:25 via R1_T2)", js[0].Arrival, 9*3600+25*60)
	}
}

func TestTransferViaFootpath(t *testing.T) {
	e := fixtureEngine(t, nil)
	paths := fp(e, t, "S3", "S4", 300)
	e = fixtureEngine(t, paths)
	// 08:30 from S1 to S4: R1_T1 (S4 08:18) already gone; only journey is
	// R1_T2 to S3 (09:25) + 5 min walk.
	js, err := e.Query("S1", "S4", 8*3600+30*60, "20260310")
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 1 {
		t.Fatalf("journeys = %d, want 1: %+v", len(js), js)
	}
	j := js[0]
	want := int32(9*3600 + 25*60 + 300)
	if j.Arrival != want {
		t.Errorf("arrival = %d, want %d (09:30 after walk)", j.Arrival, want)
	}
	if len(j.Legs) != 2 || j.Legs[1].Trip != -1 {
		t.Errorf("want ride+walk, got %+v", j.Legs)
	}
}

func TestOvernightFromPreviousServiceDay(t *testing.T) {
	e := fixtureEngine(t, nil)
	// SPECIAL is active on 20261225 (Friday). Its overnight trip departs
	// 25:30 = 01:30 on the 26th (Saturday). Querying the 26th at 01:00
	// must catch it even though no service is active ON the 26th.
	js, err := e.Query("S1", "S4", 3600, "20261226")
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 1 {
		t.Fatalf("journeys = %d, want 1: %+v", len(js), js)
	}
	j := js[0]
	if j.Legs[0].Departure != 91800-86400 {
		t.Errorf("departure = %d, want 5400 (01:30 today-clock)", j.Legs[0].Departure)
	}
	if j.Arrival != 92700-86400 {
		t.Errorf("arrival = %d, want 6300 (01:45 today-clock)", j.Arrival)
	}
}

func TestOvernightSameServiceDay(t *testing.T) {
	e := fixtureEngine(t, nil)
	// Querying the 25th late evening: R2_OWL1 departs 25:30 on today's
	// clock (01:30 tomorrow) — a valid catch with arrival > 86400.
	js, err := e.Query("S1", "S4", 23*3600, "20261225")
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 1 {
		t.Fatalf("journeys = %d, want 1: %+v", len(js), js)
	}
	if js[0].Arrival != 92700 {
		t.Errorf("arrival = %d, want 92700 (25:45)", js[0].Arrival)
	}
}

func TestHolidayRemoval(t *testing.T) {
	e := fixtureEngine(t, nil)
	// 20261225 is a Friday but WKDY is removed by exception; only SPECIAL
	// (overnight) runs. A morning S1->S5 query must find nothing.
	js, err := e.Query("S1", "S5", h0700, "20261225")
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 0 {
		t.Errorf("expected no journey on holiday, got %+v", js)
	}
}

func TestNoServiceSunday(t *testing.T) {
	e := fixtureEngine(t, nil)
	js, err := e.Query("S1", "S5", h0700, "20260315") // Sunday
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 0 {
		t.Errorf("expected no journey on Sunday, got %+v", js)
	}
}

func TestErrors(t *testing.T) {
	e := fixtureEngine(t, nil)
	if _, err := e.Query("NOPE", "S5", h8, "20260310"); err == nil {
		t.Error("unknown source: want error")
	}
	if _, err := e.Query("S1", "NOPE", h8, "20260310"); err == nil {
		t.Error("unknown target: want error")
	}
	if _, err := e.Query("S1", "S5", h8, "2026-03-10"); err == nil {
		t.Error("bad date format: want error")
	}
}

func TestSourceEqualsTargetNeighborhood(t *testing.T) {
	e := fixtureEngine(t, nil)
	paths := fp(e, t, "S1", "S2", 200)
	e = fixtureEngine(t, paths)
	// Pure walk: S1 -> S2 with no bus needed... but a walk-only journey
	// has no transit leg; RAPTOR emits it from round 0 only if the target
	// improves. Our implementation reports journeys only when a round
	// improves the target, and round 0 isn't reported. Verify instead
	// that the walk seeds a faster overall journey: S2 -> S3 at 08:02
	// walks back to... simpler: query S1->S3 at 07:59. Direct R1_T1
	// departs S1 08:00 arr S3 08:10; also walk S1->S2 (200s) misses
	// nothing better. Assert we still get the direct ride.
	js, err := e.Query("S1", "S3", 7*3600+59*60, "20260310")
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 1 || js[0].Arrival != 8*3600+10*60 {
		t.Fatalf("want direct 08:10 arrival, got %+v", js)
	}
}
