package transfers

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestRealFeedStats runs Generate over the actual KCM stops.txt and logs
// summary statistics. It reads the CSV directly (rather than through
// internal/gtfs) so this package stays free of internal dependencies. It
// skips when the feed isn't present (data/ is gitignored), so CI and
// fresh clones still pass.
func TestRealFeedStats(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	path := filepath.Join("..", "..", "data", "gtfs", "stops.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("real feed not present at %s; run `make fetch` first", path)
	}
	defer f.Close()

	lats, lons := readStopCoords(t, f)

	paths, skipped := Generate(lats, lons, 200, 1.2)

	maxSecs := int32(0)
	for _, p := range paths {
		if p.Seconds > maxSecs {
			maxSecs = p.Seconds
		}
	}
	t.Logf("stops in:            %d", len(lats))
	t.Logf("stops skipped:       %d", skipped)
	t.Logf("footpaths:           %d", len(paths))
	t.Logf("mean footpaths/stop: %.2f", float64(len(paths))/float64(len(lats)-skipped))
	t.Logf("max walk seconds:    %d", maxSecs)

	if len(paths) == 0 {
		t.Error("expected at least one footpath in the real feed")
	}
	if len(paths)%2 != 0 {
		t.Errorf("footpath count %d is odd; symmetric pairs should make it even", len(paths))
	}
}

// readStopCoords parses stop_lat/stop_lon out of an open stops.txt. Columns
// are located by header name, so column order doesn't matter. Stop i in the
// returned slices is row i of the file, matching Generate's index-based ids.
func readStopCoords(t *testing.T, f *os.File) (lats, lons []float64) {
	t.Helper()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		t.Fatalf("read stops.txt header: %v", err)
	}
	col := make(map[string]int, len(header))
	for i, name := range header {
		col[name] = i
	}
	for _, required := range []string{"stop_id", "stop_lat", "stop_lon"} {
		if _, ok := col[required]; !ok {
			t.Fatalf("stops.txt missing %q column; header = %v", required, header)
		}
	}

	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read stops.txt rows: %v", err)
	}
	for _, row := range rows {
		lat, err := strconv.ParseFloat(row[col["stop_lat"]], 64)
		if err != nil {
			lat = 0 // unparsable -> (0,0), which Generate skips and counts
		}
		lon, err := strconv.ParseFloat(row[col["stop_lon"]], 64)
		if err != nil {
			lon = 0
		}
		lats = append(lats, lat)
		lons = append(lons, lon)
	}
	return lats, lons
}
