package transfers

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
)

// Default parameters used throughout the tests, matching what real callers
// pass.
const (
	testMaxMeters = 200.0
	testWalkSpeed = 1.2
)

func TestHaversineKnownDistances(t *testing.T) {
	// Expected values computed independently in Python with the same
	// formula (mean Earth radius 6371000 m) and hardcoded here.
	tests := []struct {
		name                   string
		lat1, lon1, lat2, lon2 float64
		want                   float64 // meters
	}{
		{"~100m due north", 47.6000, -122.3300, 47.60089932, -122.3300, 99.99982143007546},
		{"~150m due east", 47.6000, -122.3300, 47.6000, -122.3280, 149.95800904274475},
		{"~211m due north", 47.6000, -122.3300, 47.6019, -122.3300, 211.27036062467675},
		{"zero distance", 47.6, -122.33, 47.6, -122.33, 0},
	}
	for _, tt := range tests {
		got := haversineMeters(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
		if math.Abs(got-tt.want) > 1e-6 {
			t.Errorf("%s: haversineMeters() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGenerateDistanceThresholdAndWalkTime(t *testing.T) {
	// Three stops:
	//   0 -- 1: ~99.9998 m apart  -> included, ceil(99.9998/1.2) = 84 s
	//   0 -- 2: ~211.27 m apart   -> excluded (over 200 m)
	//   1 -- 2: ~111.27 m apart   -> included (both due north of stop 0)
	lats := []float64{47.6000, 47.60089932, 47.6019}
	lons := []float64{-122.3300, -122.3300, -122.3300}

	paths, skipped := Generate(lats, lons, testMaxMeters, testWalkSpeed)
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}

	d12 := haversineMeters(lats[1], lons[1], lats[2], lons[2])
	want := []Footpath{
		{From: 0, To: 1, Seconds: 84},
		{From: 1, To: 0, Seconds: 84},
		{From: 1, To: 2, Seconds: int32(math.Ceil(d12 / testWalkSpeed))},
		{From: 2, To: 1, Seconds: int32(math.Ceil(d12 / testWalkSpeed))},
	}
	if !reflect.DeepEqual(paths, want) {
		t.Errorf("Generate() = %v, want %v", paths, want)
	}
}

func TestGenerateWalkTimeCeil(t *testing.T) {
	// ~149.958 m apart: 149.958.../1.2 = 124.965... -> ceil = 125 s.
	lats := []float64{47.6000, 47.6000}
	lons := []float64{-122.3300, -122.3280}

	paths, _ := Generate(lats, lons, testMaxMeters, testWalkSpeed)
	if len(paths) != 2 {
		t.Fatalf("got %d footpaths, want 2", len(paths))
	}
	if paths[0].Seconds != 125 {
		t.Errorf("walk seconds = %d, want 125 (ceil of 124.96...)", paths[0].Seconds)
	}
}

func TestGenerateFiltersBadCoordinates(t *testing.T) {
	lats := []float64{
		47.6051, // 0: valid (downtown Seattle)
		0,       // 1: (0,0) placeholder -> skipped
		47.6052, // 2: valid, ~11 m from stop 0
		35.0,    // 3: out of bbox (too far south) -> skipped
		47.6,    // 4: valid lat but lon out of bbox -> skipped
	}
	lons := []float64{-122.3365, 0, -122.3365, -122.3, -100.0}

	paths, skipped := Generate(lats, lons, testMaxMeters, testWalkSpeed)
	if skipped != 3 {
		t.Errorf("skipped = %d, want 3", skipped)
	}
	for _, p := range paths {
		for _, id := range []int32{1, 3, 4} {
			if p.From == id || p.To == id {
				t.Errorf("skipped stop %d appears in footpath %+v", id, p)
			}
		}
	}
	// Stops 0 and 2 are ~11 m apart, so exactly one symmetric pair remains.
	if len(paths) != 2 {
		t.Errorf("got %d footpaths, want 2: %v", len(paths), paths)
	}
}

func TestGenerateNoSelfTransfers(t *testing.T) {
	lats, lons := randomStops(200)
	paths, _ := Generate(lats, lons, testMaxMeters, testWalkSpeed)
	for _, p := range paths {
		if p.From == p.To {
			t.Fatalf("self-transfer emitted: %+v", p)
		}
	}
}

func TestGenerateSymmetry(t *testing.T) {
	lats, lons := randomStops(500)
	paths, _ := Generate(lats, lons, testMaxMeters, testWalkSpeed)
	if len(paths) == 0 {
		t.Fatal("expected some footpaths from 500 stops in a small box")
	}
	seconds := make(map[[2]int32]int32, len(paths))
	for _, p := range paths {
		seconds[[2]int32{p.From, p.To}] = p.Seconds
	}
	for _, p := range paths {
		back, ok := seconds[[2]int32{p.To, p.From}]
		if !ok {
			t.Fatalf("missing reverse of %+v", p)
		}
		if back != p.Seconds {
			t.Fatalf("asymmetric seconds: %d->%d is %d s but reverse is %d s",
				p.From, p.To, p.Seconds, back)
		}
	}
}

func TestGenerateMatchesBruteForce(t *testing.T) {
	// The grid is an optimization only: its output must be exactly equal
	// to the naive all-pairs reference on the same input.
	lats, lons := randomStops(500)

	got, _ := Generate(lats, lons, testMaxMeters, testWalkSpeed)
	want := bruteForce(lats, lons, testMaxMeters, testWalkSpeed)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("grid output differs from brute force: got %d footpaths, want %d",
			len(got), len(want))
	}
}

func TestGenerateDeterministic(t *testing.T) {
	lats, lons := randomStops(500)
	a, skippedA := Generate(lats, lons, testMaxMeters, testWalkSpeed)
	b, skippedB := Generate(lats, lons, testMaxMeters, testWalkSpeed)
	if skippedA != skippedB {
		t.Fatalf("skipped counts differ: %d vs %d", skippedA, skippedB)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two runs on identical input produced different output")
	}
}

// randomStops generates n stops in a ~2 km box around downtown Seattle,
// dense enough that plenty of pairs land within 200 m. The seed is fixed
// so every test run sees the same coordinates.
func randomStops(n int) (lats, lons []float64) {
	rng := rand.New(rand.NewSource(42))
	lats = make([]float64, n)
	lons = make([]float64, n)
	for i := range lats {
		lats[i] = 47.60 + rng.Float64()*0.018   // ~2 km of latitude
		lons[i] = -122.34 + rng.Float64()*0.027 // ~2 km of longitude at 47.6N
	}
	return lats, lons
}

// bruteForce is the O(n^2) reference implementation: same filtering,
// distance, walk time, and ordering rules as Generate, minus the grid.
func bruteForce(lats, lons []float64, maxMeters, walkSpeed float64) []Footpath {
	var paths []Footpath
	for i := range lats {
		if !inServiceArea(lats[i], lons[i]) {
			continue
		}
		for j := range lats {
			if j == i || !inServiceArea(lats[j], lons[j]) {
				continue
			}
			d := haversineMeters(lats[i], lons[i], lats[j], lons[j])
			if d > maxMeters {
				continue
			}
			paths = append(paths, Footpath{
				From:    int32(i),
				To:      int32(j),
				Seconds: int32(math.Ceil(d / walkSpeed)),
			})
		}
	}
	// i-major, j-minor iteration already yields (From, To) order, but the
	// nil-vs-empty distinction matters for DeepEqual, so mirror Generate.
	return paths
}
