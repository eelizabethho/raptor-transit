package gtfs

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestRealFeedRoundTrip parses the actual KCM feed and checks that the
// gob round-trip reproduces it exactly. It skips when the feed isn't
// present (data/ is gitignored), so CI and fresh clones still pass.
func TestRealFeedRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	zipPath := filepath.Join("..", "..", "data", "google_transit.zip")
	if _, err := os.Stat(zipPath); err != nil {
		t.Skipf("real feed not present at %s; run `make fetch` first", zipPath)
	}

	feed, err := ParseZip(zipPath)
	if err != nil {
		t.Fatalf("ParseZip: %v", err)
	}
	if len(feed.Stops) == 0 || feed.NumStopTimes() == 0 {
		t.Fatalf("suspiciously empty feed: %d stops, %d stop_times", len(feed.Stops), feed.NumStopTimes())
	}

	path := filepath.Join(t.TempDir(), "real.gob")
	if err := feed.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(feed, loaded) {
		t.Error("round-tripped real feed differs from original")
	}
}
