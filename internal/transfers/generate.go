package transfers

import (
	"math"
	"sort"
)

// earthRadiusMeters is the mean Earth radius used by the haversine formula.
const earthRadiusMeters = 6371000.0

// Generous bounding box around King County Metro's service area. Stops
// outside it (including the common (0,0) "no coordinates" placeholder,
// which is in the Gulf of Guinea) are treated as bad data and skipped.
const (
	bboxMinLat = 46.5
	bboxMaxLat = 48.5
	bboxMinLon = -123.5
	bboxMaxLon = -121.0
)

// Footpath is a one-way walking link between two stops. From and To are
// stop indices (index i in the coordinate slices passed to Generate), and
// Seconds is the walking time. Footpaths always come in symmetric pairs:
// if (a, b, s) is present then so is (b, a, s).
type Footpath struct {
	From, To int32
	Seconds  int32
}

// Generate builds walking footpaths between every pair of distinct stops
// within maxMeters of each other (great-circle distance). Index i in lats
// and lons is stop i. walkSpeed is in meters per second; walking time is
// rounded up to whole seconds. Callers typically use maxMeters = 200 and
// walkSpeed = 1.2.
//
// The returned slice is sorted by (From, To) and contains both directions
// of every pair, so its length is always even. The second return value is
// the number of stops skipped for having coordinates outside the service
// area (see the bbox constants above); skipped stops appear in no footpath.
//
// There are deliberately no self-entries (From == To): RAPTOR must still
// treat every stop as reachable from itself at zero cost, but the caller
// handles that case directly rather than carrying thousands of redundant
// (i, i, 0) rows here.
func Generate(lats, lons []float64, maxMeters, walkSpeed float64) ([]Footpath, int) {
	// Filter out stops with unusable coordinates.
	valid := make([]int32, 0, len(lats))
	skipped := 0
	for i := range lats {
		if inServiceArea(lats[i], lons[i]) {
			valid = append(valid, int32(i))
		} else {
			skipped++
		}
	}

	// Bucket stops into a lat/lon grid so each stop only has to be
	// compared against stops in its own and the 8 neighboring cells,
	// instead of against all n stops (O(n^2) would be ~40M haversine
	// calls for KCM's ~6k stops; the grid does ~a few per stop).
	//
	// Cell size is maxMeters converted to degrees, with a small margin so
	// each cell is guaranteed to span at least maxMeters of ground
	// everywhere in the bbox. One degree of latitude is a fixed
	// ~111.2 km; one degree of longitude shrinks by cos(lat) as you move
	// away from the equator, so we scale by the smallest cos in the bbox
	// (its most poleward edge) to stay conservative. With cells at least
	// maxMeters wide, any two stops within maxMeters differ by at most
	// one cell index in each axis, so the 3x3 neighborhood scan is
	// exhaustive.
	const metersPerDegLat = math.Pi * earthRadiusMeters / 180
	maxAbsLat := math.Max(math.Abs(bboxMinLat), math.Abs(bboxMaxLat))
	cellLatDeg := maxMeters / metersPerDegLat * 1.01
	cellLonDeg := maxMeters / (metersPerDegLat * math.Cos(maxAbsLat*math.Pi/180)) * 1.01

	type cell struct{ row, col int }
	grid := make(map[cell][]int32, len(valid))
	cellOf := func(i int32) cell {
		return cell{
			row: int(math.Floor(lats[i] / cellLatDeg)),
			col: int(math.Floor(lons[i] / cellLonDeg)),
		}
	}
	for _, i := range valid {
		c := cellOf(i)
		grid[c] = append(grid[c], i)
	}

	var paths []Footpath
	for _, i := range valid {
		c := cellOf(i)
		for dr := -1; dr <= 1; dr++ {
			for dc := -1; dc <= 1; dc++ {
				for _, j := range grid[cell{c.row + dr, c.col + dc}] {
					// Handle each pair once, from its lower-indexed side,
					// so we never compute a distance twice.
					if j <= i {
						continue
					}
					d := haversineMeters(lats[i], lons[i], lats[j], lons[j])
					if d > maxMeters {
						continue
					}
					secs := int32(math.Ceil(d / walkSpeed))
					paths = append(paths,
						Footpath{From: i, To: j, Seconds: secs},
						Footpath{From: j, To: i, Seconds: secs},
					)
				}
			}
		}
	}

	sort.Slice(paths, func(a, b int) bool {
		if paths[a].From != paths[b].From {
			return paths[a].From < paths[b].From
		}
		return paths[a].To < paths[b].To
	})
	return paths, skipped
}

// inServiceArea reports whether a coordinate falls inside the service-area
// bounding box.
func inServiceArea(lat, lon float64) bool {
	return lat >= bboxMinLat && lat <= bboxMaxLat &&
		lon >= bboxMinLon && lon <= bboxMaxLon
}

// haversineMeters returns the great-circle distance in meters between two
// points given in decimal degrees.
func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const degToRad = math.Pi / 180
	phi1 := lat1 * degToRad
	phi2 := lat2 * degToRad
	sinDPhi := math.Sin((lat2 - lat1) * degToRad / 2)
	sinDLambda := math.Sin((lon2 - lon1) * degToRad / 2)
	a := sinDPhi*sinDPhi + math.Cos(phi1)*math.Cos(phi2)*sinDLambda*sinDLambda
	return 2 * earthRadiusMeters * math.Asin(math.Sqrt(a))
}
