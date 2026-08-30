package realtime

import (
	"path/filepath"
	"testing"
	"time"

	"raptor-transit/internal/gtfs"
	"raptor-transit/internal/timetable"
)

// The overlay's whole job is translating the engine's dense indices into
// the GTFS ids the feed speaks. Getting that mapping wrong would apply a
// real delay to the wrong bus, which is worse than applying none.
func TestOverlayMapsIndicesToFeedIDs(t *testing.T) {
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatal(err)
	}
	tt := timetable.Build(feed)

	b := buildFeed(t, tripUpdate("e1", "R1_T1", 0, arrDelay("S3", 420)))
	snap, err := Parse(b, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	o := NewOverlay(tt, snap)

	trip, ok := tt.TripIdx("R1_T1")
	if !ok {
		t.Fatal("fixture trip R1_T1 missing from timetable")
	}
	stop, ok := tt.StopIdx("S3")
	if !ok {
		t.Fatal("fixture stop S3 missing from timetable")
	}
	if got := o.Delay(trip, stop); got != 420 {
		t.Errorf("Delay(R1_T1, S3) = %d, want 420", got)
	}

	// A stop on the same trip with no update reports no delay, not the
	// neighbouring stop's.
	other, _ := tt.StopIdx("S4")
	if got := o.Delay(trip, other); got != 0 {
		t.Errorf("Delay(R1_T1, S4) = %d, want 0 for an unreported stop", got)
	}
	// A different trip at the same stop likewise.
	otherTrip, _ := tt.TripIdx("R1_T2")
	if got := o.Delay(otherTrip, stop); got != 0 {
		t.Errorf("Delay(R1_T2, S3) = %d, want 0", got)
	}
}

func TestOverlayRejectsOutOfRangeIndices(t *testing.T) {
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatal(err)
	}
	tt := timetable.Build(feed)
	o := NewOverlay(tt, &Snapshot{arrival: map[Key]int32{}})

	for _, tc := range []struct{ trip, stop int32 }{
		{-1, 0}, {0, -1}, {1 << 20, 0}, {0, 1 << 20},
	} {
		if got := o.Delay(tc.trip, tc.stop); got != 0 {
			t.Errorf("Delay(%d, %d) = %d, want 0", tc.trip, tc.stop, got)
		}
	}
}

func TestNilOverlayIsSafe(t *testing.T) {
	var o *Overlay
	if got := o.Delay(0, 0); got != 0 {
		t.Errorf("nil overlay Delay = %d, want 0", got)
	}
	if o.Snapshot() != nil {
		t.Error("nil overlay returned a snapshot")
	}
	if got := NewOverlay(nil, nil).Delay(0, 0); got != 0 {
		t.Errorf("overlay with nil snapshot Delay = %d, want 0", got)
	}
}
