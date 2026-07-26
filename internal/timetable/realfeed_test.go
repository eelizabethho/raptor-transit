package timetable

import (
	"os"
	"path/filepath"
	"testing"

	"raptor-transit/internal/gtfs"
)

// TestRealFeedBuild builds a Timetable from the actual KCM feed, checks
// the pattern invariant over every pattern, and logs size stats. It
// skips when the feed isn't present (data/ is gitignored) so CI and
// fresh clones still pass.
func TestRealFeedBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	zipPath := filepath.Join("..", "..", "data", "google_transit.zip")
	if _, err := os.Stat(zipPath); err != nil {
		t.Skipf("real feed not present at %s; run `make fetch` first", zipPath)
	}

	feed, err := gtfs.ParseZip(zipPath)
	if err != nil {
		t.Fatalf("ParseZip: %v", err)
	}
	tt := Build(feed)

	if len(tt.Patterns) == 0 || len(tt.StopIDs) == 0 {
		t.Fatalf("suspiciously empty timetable: %d patterns, %d stops",
			len(tt.Patterns), len(tt.StopIDs))
	}

	assertPatternInvariant(t, feed, tt)
	assertTripsSorted(t, tt)

	// Every stop time must be stored exactly once: total flat-array
	// slots == stop_times rows for the trips we kept.
	slots, timeBytes := 0, 0
	for p := range tt.Patterns {
		pat := &tt.Patterns[p]
		slots += len(pat.Arrivals)
		timeBytes += 4 * (len(pat.Arrivals) + len(pat.Departures))
	}
	want := 0
	for _, id := range tt.TripIDs {
		want += len(feed.StopTimesByTrip[id])
	}
	if slots != want {
		t.Errorf("flat arrays hold %d stop times, feed has %d", slots, want)
	}

	t.Logf("patterns: %d (from %d GTFS routes)", len(tt.Patterns), len(tt.Routes))
	t.Logf("stops: %d, trips: %d", len(tt.StopIDs), len(tt.TripIDs))
	t.Logf("int32 time-array bytes: %d (%.1f MB)", timeBytes, float64(timeBytes)/(1<<20))
}
