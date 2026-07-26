package timetable

import (
	"encoding/binary"
	"sort"

	"raptor-transit/internal/gtfs"
)

// Build converts a parsed GTFS feed into a Timetable.
//
// Construction is deterministic: every map in the feed is iterated via
// sorted keys, so building the same feed twice yields byte-identical gob
// output (a Phase 1 review requirement).
func Build(feed *gtfs.Feed) *Timetable {
	tt := &Timetable{}

	// --- Stops: dense stopIdx in sorted stop_id order. ---
	stopIDs := sortedKeys(feed.Stops)
	tt.StopIDs = stopIDs
	tt.StopNames = make([]string, len(stopIDs))
	tt.StopLats = make([]float64, len(stopIDs))
	tt.StopLons = make([]float64, len(stopIDs))
	stopIdx := make(map[string]int32, len(stopIDs))
	for i, id := range stopIDs {
		s := feed.Stops[id]
		tt.StopNames[i] = s.Name
		tt.StopLats[i] = s.Lat
		tt.StopLons[i] = s.Lon
		stopIdx[id] = int32(i)
	}

	// --- Trips: dense tripIdx in sorted trip_id order. Trips with no
	// stop_times can't be ridden, so they're dropped entirely. ---
	var tripIDs []string
	for _, id := range sortedKeys(feed.Trips) {
		if len(feed.StopTimesByTrip[id]) > 0 {
			tripIDs = append(tripIDs, id)
		}
	}
	tt.TripIDs = tripIDs
	tt.TripRouteIDs = make([]string, len(tripIDs))
	for i, id := range tripIDs {
		tt.TripRouteIDs[i] = feed.Trips[id].RouteID
	}

	// --- Group trips into stop patterns. The pattern key is the exact
	// ordered stop sequence (as bytes), never the GTFS route_id: KCM
	// route_ids mix express and short-turn variants with different
	// stop lists, which would violate RAPTOR's same-stops-per-route
	// assumption. ---
	patternByKey := make(map[string]int32)
	// tripsByPattern collects each pattern's trips before time-sorting.
	var tripsByPattern [][]int32
	for i, id := range tripIDs {
		sts := feed.StopTimesByTrip[id] // already sorted by stop_sequence
		seq := make([]int32, len(sts))
		for j, st := range sts {
			seq[j] = stopIdx[st.StopID]
		}
		key := patternKey(seq)
		p, ok := patternByKey[key]
		if !ok {
			p = int32(len(tt.Patterns))
			patternByKey[key] = p
			tt.Patterns = append(tt.Patterns, Pattern{
				Stops:   seq,
				RouteID: feed.Trips[id].RouteID,
			})
			tripsByPattern = append(tripsByPattern, nil)
		}
		tripsByPattern[p] = append(tripsByPattern[p], int32(i))
	}

	// --- Per pattern: sort trips by first-stop departure, then lay out
	// times in flat arrays (index = trip*len(Stops)+pos). ---
	for p := range tt.Patterns {
		pat := &tt.Patterns[p]
		trips := tripsByPattern[p]
		sort.SliceStable(trips, func(a, b int) bool {
			da := feed.StopTimesByTrip[tripIDs[trips[a]]][0].Departure
			db := feed.StopTimesByTrip[tripIDs[trips[b]]][0].Departure
			if da != db {
				return da < db
			}
			// Tie-break on tripIdx so equal departures still order
			// deterministically.
			return trips[a] < trips[b]
		})
		pat.Trips = trips
		pat.ServiceIDs = make([]string, len(trips))
		pat.Arrivals = make([]int32, len(trips)*len(pat.Stops))
		pat.Departures = make([]int32, len(trips)*len(pat.Stops))
		for i, trip := range trips {
			id := tripIDs[trip]
			pat.ServiceIDs[i] = feed.Trips[id].ServiceID
			base := i * len(pat.Stops)
			for j, st := range feed.StopTimesByTrip[id] {
				pat.Arrivals[base+j] = int32(st.Arrival)
				pat.Departures[base+j] = int32(st.Departure)
			}
		}
	}

	// --- Stop -> patterns index. Built by walking patterns in order so
	// each stop's list is naturally sorted by (pattern, position); a
	// pattern that loops through a stop twice adds two entries. ---
	tt.StopPatterns = make([][]PatternStop, len(stopIDs))
	for p := range tt.Patterns {
		for pos, s := range tt.Patterns[p].Stops {
			tt.StopPatterns[s] = append(tt.StopPatterns[s], PatternStop{
				Pattern: int32(p),
				Pos:     int32(pos),
			})
		}
	}

	// --- Routes and Services, sorted by ID. ---
	for _, id := range sortedKeys(feed.Routes) {
		r := feed.Routes[id]
		tt.Routes = append(tt.Routes, Route{
			ID:        r.ID,
			ShortName: r.ShortName,
			LongName:  r.LongName,
		})
	}
	for _, id := range sortedKeys(feed.Services) {
		s := feed.Services[id]
		tt.Services = append(tt.Services, Service{
			ID:        s.ID,
			Weekdays:  s.Weekdays,
			StartDate: s.StartDate,
			EndDate:   s.EndDate,
			Added:     sortedKeys(s.Added),
			Removed:   sortedKeys(s.Removed),
		})
	}

	tt.buildLookups()
	return tt
}

// patternKey encodes an ordered stop sequence as a byte string usable as
// a map key. Fixed-width little-endian int32s make distinct sequences
// produce distinct keys.
func patternKey(seq []int32) string {
	buf := make([]byte, 4*len(seq))
	for i, s := range seq {
		binary.LittleEndian.PutUint32(buf[4*i:], uint32(s))
	}
	return string(buf)
}

// sortedKeys returns a map's keys in ascending order, for deterministic
// iteration (Go's built-in map iteration order is randomized). It
// returns nil (not an empty slice) for an empty map: gob drops
// zero-length slices and decodes them as nil, so storing nil keeps
// Save/Load round-trips reflect.DeepEqual-identical.
func sortedKeys[V any](m map[string]V) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
