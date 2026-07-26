package timetable

import (
	"bytes"
	"encoding/gob"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"raptor-transit/internal/gtfs"
)

// buildMini parses the shared mini fixture (5 stops, 2 GTFS routes,
// 3 trips, 11 stop_times — see internal/gtfs/testdata/EXPECTED.md) and
// builds a Timetable from it.
func buildMini(t *testing.T) *Timetable {
	t.Helper()
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatalf("ParseZip(mini.zip): %v", err)
	}
	return Build(feed)
}

// assertPatternInvariant checks, for every pattern, that every trip's
// stop_times in the source feed visit exactly the pattern's stops in the
// pattern's order. This is the property RAPTOR depends on and the reason
// trips are grouped by stop sequence instead of GTFS route_id.
func assertPatternInvariant(t *testing.T, feed *gtfs.Feed, tt *Timetable) {
	t.Helper()
	for p := range tt.Patterns {
		pat := &tt.Patterns[p]
		for _, trip := range pat.Trips {
			sts := feed.StopTimesByTrip[tt.TripIDs[trip]]
			if len(sts) != len(pat.Stops) {
				t.Fatalf("pattern %d: trip %s has %d stops, pattern has %d",
					p, tt.TripIDs[trip], len(sts), len(pat.Stops))
			}
			for pos, st := range sts {
				if got := tt.StopIDs[pat.Stops[pos]]; got != st.StopID {
					t.Fatalf("pattern %d trip %s position %d: pattern stop %s, trip stop %s",
						p, tt.TripIDs[trip], pos, got, st.StopID)
				}
			}
		}
	}
}

func TestMiniPatterns(t *testing.T) {
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatalf("ParseZip: %v", err)
	}
	tt := Build(feed)

	// R1_T1 (S1..S5), R1_T2 (S1,S2,S3), R2_OWL1 (S1,S2,S4) all have
	// distinct stop sequences, so 3 trips -> 3 patterns of 1 trip each,
	// even though R1_T1 and R1_T2 share GTFS route R1 and service WKDY.
	if len(tt.Patterns) != 3 {
		t.Fatalf("got %d patterns, want 3", len(tt.Patterns))
	}
	for p := range tt.Patterns {
		if n := tt.Patterns[p].NumTrips(); n != 1 {
			t.Errorf("pattern %d has %d trips, want 1", p, n)
		}
	}

	assertPatternInvariant(t, feed, tt)

	// Spot-check pattern metadata via R1_T1's pattern.
	trip, ok := tt.TripIdx("R1_T1")
	if !ok {
		t.Fatal("R1_T1 missing from trip index")
	}
	var pat *Pattern
	for p := range tt.Patterns {
		if len(tt.Patterns[p].Trips) == 1 && tt.Patterns[p].Trips[0] == trip {
			pat = &tt.Patterns[p]
		}
	}
	if pat == nil {
		t.Fatal("no pattern contains R1_T1")
	}
	if pat.RouteID != "R1" {
		t.Errorf("R1_T1 pattern RouteID = %q, want R1", pat.RouteID)
	}
	if pat.ServiceIDs[0] != "WKDY" {
		t.Errorf("R1_T1 ServiceID = %q, want WKDY", pat.ServiceIDs[0])
	}
	if got := tt.TripRouteShortName(trip); got == "" {
		t.Error("TripRouteShortName(R1_T1) is empty; want the R1 short name")
	}
	// First stop departure 08:00:00 = 28800, last stop arrival 08:26:00.
	if got := pat.Departure(0, 0); got != 28800 {
		t.Errorf("R1_T1 first departure = %d, want 28800", got)
	}
	if got := pat.Arrival(0, len(pat.Stops)-1); got != 8*3600+26*60 {
		t.Errorf("R1_T1 last arrival = %d, want %d", got, 8*3600+26*60)
	}
}

func TestTripsSortedByFirstStopDeparture(t *testing.T) {
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatalf("ParseZip: %v", err)
	}
	tt := Build(feed)
	assertTripsSorted(t, tt)
}

// assertTripsSorted checks that within every pattern, trips are ordered
// by departure time at the pattern's first stop.
func assertTripsSorted(t *testing.T, tt *Timetable) {
	t.Helper()
	for p := range tt.Patterns {
		pat := &tt.Patterns[p]
		for i := 1; i < pat.NumTrips(); i++ {
			if pat.Departure(i-1, 0) > pat.Departure(i, 0) {
				t.Fatalf("pattern %d: trip %d departs %d before trip %d at %d",
					p, i, pat.Departure(i, 0), i-1, pat.Departure(i-1, 0))
			}
		}
	}
}

func TestStopPatternsIndexRoundTrips(t *testing.T) {
	tt := buildMini(t)

	// Every index entry must point back at the stop it's filed under...
	entries := 0
	for stop, refs := range tt.StopPatterns {
		for _, ps := range refs {
			entries++
			if got := tt.Patterns[ps.Pattern].Stops[ps.Pos]; got != int32(stop) {
				t.Errorf("StopPatterns[%d] -> pattern %d pos %d, which is stop %d",
					stop, ps.Pattern, ps.Pos, got)
			}
		}
	}
	// ...and every pattern position must appear in the index exactly once,
	// so total entries equal total pattern positions (a loop pattern
	// visiting a stop twice yields two entries for that stop).
	positions := 0
	for p := range tt.Patterns {
		pat := &tt.Patterns[p]
		positions += len(pat.Stops)
		for pos, stop := range pat.Stops {
			found := false
			for _, ps := range tt.StopPatterns[stop] {
				if ps.Pattern == int32(p) && ps.Pos == int32(pos) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("pattern %d pos %d (stop %d) missing from StopPatterns", p, pos, stop)
			}
		}
	}
	if entries != positions {
		t.Errorf("StopPatterns has %d entries, patterns have %d positions", entries, positions)
	}
}

func TestOvernightTimesPreserved(t *testing.T) {
	tt := buildMini(t)

	// R2_OWL1 runs past midnight: 25:30:00=91800 .. 25:45:00=92700.
	// These must survive as raw >86400 values in the flat arrays.
	trip, ok := tt.TripIdx("R2_OWL1")
	if !ok {
		t.Fatal("R2_OWL1 missing from trip index")
	}
	for p := range tt.Patterns {
		pat := &tt.Patterns[p]
		for i, tr := range pat.Trips {
			if tr != trip {
				continue
			}
			if got := pat.Departure(i, 0); got != 91800 {
				t.Errorf("R2_OWL1 first departure = %d, want 91800", got)
			}
			if got := pat.Arrival(i, len(pat.Stops)-1); got != 92700 {
				t.Errorf("R2_OWL1 last arrival = %d, want 92700", got)
			}
			return
		}
	}
	t.Fatal("no pattern contains R2_OWL1")
}

func TestGobRoundTrip(t *testing.T) {
	tt := buildMini(t)

	path := filepath.Join(t.TempDir(), "tt.gob")
	if err := tt.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(tt, loaded) {
		t.Error("round-tripped timetable differs from original")
	}
	// The rebuilt lookups must actually work post-Load.
	if _, ok := loaded.StopIdx("S3"); !ok {
		t.Error("loaded timetable: StopIdx(S3) not found")
	}
	if _, ok := loaded.ServiceByID("SPECIAL"); !ok {
		t.Error("loaded timetable: ServiceByID(SPECIAL) not found")
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	// Build the same feed twice (fresh parses, so map layouts differ)
	// and require byte-identical gob encodings. Phase 1 review flagged
	// nondeterministic gob bytes from map iteration order.
	encode := func() []byte {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(buildMini(t)); err != nil {
			t.Fatalf("gob encode: %v", err)
		}
		return buf.Bytes()
	}
	if !bytes.Equal(encode(), encode()) {
		t.Error("two builds of the same feed produced different gob bytes")
	}
}

func TestServiceActiveOn(t *testing.T) {
	tt := buildMini(t)

	// Dates and expectations straight from EXPECTED.md.
	cases := []struct {
		service string
		date    string
		weekday time.Weekday
		want    bool
	}{
		{"WKDY", "20260310", time.Tuesday, true},     // normal Tuesday
		{"WKDY", "20261225", time.Friday, false},     // removed by exception
		{"SPECIAL", "20261225", time.Friday, true},   // added by exception
		{"SPECIAL", "20260310", time.Tuesday, false}, // no calendar row, not an added date
		{"WKDY", "20260315", time.Sunday, false},     // Sunday, weekday pattern off
	}
	for _, c := range cases {
		svc, ok := tt.ServiceByID(c.service)
		if !ok {
			t.Fatalf("service %s not found", c.service)
		}
		if got := svc.ActiveOn(c.date, c.weekday); got != c.want {
			t.Errorf("%s.ActiveOn(%s) = %v, want %v", c.service, c.date, got, c.want)
		}
	}
}
