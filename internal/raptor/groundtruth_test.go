package raptor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"raptor-transit/internal/gtfs"
	"raptor-transit/internal/timetable"
	"raptor-transit/internal/transfers"
)

type gtCase struct {
	ID             string   `json:"id"`
	Date           string   `json:"date"`
	OriginStopID   string   `json:"origin_stop_id"`
	DestStopID     string   `json:"destination_stop_id"`
	DepartureTime  string   `json:"departure_time"`
	ExpectedRoutes []string `json:"expected_routes"`
	ExpectedArr    string   `json:"expected_arrival"`
	ToleranceMin   int      `json:"tolerance_min"`
	Verified       bool     `json:"verified"`
}

type gtFile struct {
	Cases []gtCase `json:"cases"`
}

// realEngine builds the engine from the real feed; skips when absent.
func realEngine(t testing.TB) *Engine {
	zipPath := filepath.Join("..", "..", "data", "google_transit.zip")
	if _, err := os.Stat(zipPath); err != nil {
		t.Skipf("real feed not present at %s", zipPath)
	}
	feed, err := gtfs.ParseZip(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	tt := timetable.Build(feed)
	paths, _ := transfers.Generate(tt.StopLats, tt.StopLons, 200, 1.2)
	return New(tt, paths)
}

// TestGroundTruth runs every case in testdata/ground_truth.json and
// prints a pass/fail table. Verified cases must pass; unverified cases
// (expected values not yet human-checked) are reported but never fail
// the build. Dates in YYYYMMDD are derived from the file's YYYY-MM-DD.
func TestGroundTruth(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	e := realEngine(t)

	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "ground_truth.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gt gtFile
	if err := json.Unmarshal(raw, &gt); err != nil {
		t.Fatal(err)
	}

	t.Logf("%-38s %-10s %-9s %-9s %-9s %s", "case", "status", "expected", "actual", "delta", "routes")
	pass, fail, unverified := 0, 0, 0
	for _, c := range gt.Cases {
		date := c.Date[0:4] + c.Date[5:7] + c.Date[8:10]
		if !c.Verified || c.OriginStopID == "" || c.ExpectedArr == "" {
			unverified++
			t.Logf("%-38s %-10s (awaiting human verification)", c.ID, "UNVERIFIED")
			continue
		}
		dep, err := gtfs.ParseTime(c.DepartureTime)
		if err != nil {
			t.Errorf("%s: bad departure_time: %v", c.ID, err)
			continue
		}
		wantArr, err := gtfs.ParseTime(c.ExpectedArr)
		if err != nil {
			t.Errorf("%s: bad expected_arrival: %v", c.ID, err)
			continue
		}

		js, err := e.Query(c.OriginStopID, c.DestStopID, int32(dep), date)
		if err != nil {
			fail++
			t.Errorf("%-38s FAIL query error: %v", c.ID, err)
			continue
		}
		if len(js) == 0 {
			fail++
			t.Errorf("%-38s FAIL no journey found (expected arrival %s)", c.ID, c.ExpectedArr)
			continue
		}
		best := js[len(js)-1] // last journey has the earliest arrival
		delta := int(best.Arrival) - wantArr
		tolSec := c.ToleranceMin * 60
		// The ground-truth arrival pins a specific scheduled trip; RAPTOR
		// may find a strictly better option, which is a pass (early is
		// fine, late beyond tolerance is not).
		ok := delta <= tolSec
		routes := ""
		for _, l := range best.Legs {
			if l.Trip >= 0 {
				if routes != "" {
					routes += "+"
				}
				routes += l.Route
			}
		}
		status := "PASS"
		if !ok {
			status = "FAIL"
		}
		t.Logf("%-38s %-10s %-9s %-9s %+d min   %s",
			c.ID, status, c.ExpectedArr, fmtClock(best.Arrival), delta/60, routes)
		if ok {
			pass++
		} else {
			fail++
			t.Errorf("%s: arrived %s, expected %s ±%dmin", c.ID, fmtClock(best.Arrival), c.ExpectedArr, c.ToleranceMin)
		}
	}
	t.Logf("ground truth: %d pass, %d fail, %d unverified (of %d cases)",
		pass, fail, unverified, len(gt.Cases))
}

// TestImpossibleQueries checks honest "no journey" behavior.
func TestImpossibleQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	e := realEngine(t)

	// Date outside the feed window: no services active.
	js, err := e.Query("10911", "1120", 8*3600, "20990101")
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 0 {
		t.Errorf("query outside feed window returned %d journeys, want 0", len(js))
	}

	// Unknown stops error rather than returning empty.
	if _, err := e.Query("no-such-stop", "1120", 8*3600, "20260805"); err == nil {
		t.Error("unknown source stop: want error")
	}
}

func fmtClock(sec int32) string {
	suffix := ""
	if sec >= 86400 {
		sec -= 86400
		suffix = "+1d"
	}
	return fmt.Sprintf("%02d:%02d:%02d%s", sec/3600, sec%3600/60, sec%60, suffix)
}

// BenchmarkQuery measures full-feed query latency across a mix of
// origin/destination pairs. Run with:
//
//	go test -bench Query -benchtime 100x -run XXX ./internal/raptor/
func BenchmarkQuery(b *testing.B) {
	e := realEngine(b)
	pairs := [][2]string{
		{"10911", "1120"}, // U District -> downtown
		{"619", "35317"},  // Pioneer Square -> Northgate
		{"10658", "626"},  // Sand Point -> downtown
		{"8402", "2370"},  // Mount Baker -> Queen Anne
		{"1610", "16103"}, // downtown -> Aurora Village
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := pairs[i%len(pairs)]
		if _, err := e.Query(p[0], p[1], 8*3600, "20260805"); err != nil {
			b.Fatal(err)
		}
	}
}
