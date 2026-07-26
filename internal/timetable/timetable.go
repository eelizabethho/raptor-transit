// Package timetable converts a parsed GTFS feed (internal/gtfs) into the
// compact, integer-indexed structure that the RAPTOR algorithm scans.
//
// The central idea is the "stop pattern": trips are grouped by their exact
// ordered sequence of stops, NOT by GTFS route_id. One GTFS route (e.g. a
// King County Metro route number) mixes express, short-turn, and branch
// variants that visit different stops, which would break RAPTOR's
// assumption that every trip of a "route" serves the same stops in the
// same order. Grouping by the literal stop sequence restores that
// guarantee, at the cost of many more patterns than GTFS routes.
//
// A Timetable fully replaces gtfs.Feed at query time: it carries stop
// names/coordinates for display, service calendars for date filtering,
// and every stop time exactly once (inside pattern time arrays).
package timetable

import (
	"slices"
	"time"
)

// Timetable is the RAPTOR-ready form of a GTFS feed.
//
// Design constraint: gob encodes map entries in Go's randomized map
// iteration order, so a struct containing exported maps produces
// different bytes on every Save. To keep builds byte-for-byte
// deterministic, all persisted fields are slices sorted at build time;
// the string->index lookup maps are unexported (gob skips unexported
// fields) and rebuilt after Build and Load.
type Timetable struct {
	// Stops, indexed by stopIdx (dense int32 in sorted stop_id order).
	StopIDs   []string  // stopIdx -> GTFS stop_id
	StopNames []string  // stopIdx -> display name
	StopLats  []float64 // stopIdx -> latitude
	StopLons  []float64 // stopIdx -> longitude

	// Trips, indexed by tripIdx (dense int32 in sorted trip_id order).
	TripIDs      []string // tripIdx -> GTFS trip_id
	TripRouteIDs []string // tripIdx -> GTFS route_id (for display lookups)

	// Patterns ordered by first appearance while scanning trips in
	// sorted trip_id order (deterministic).
	Patterns []Pattern

	// StopPatterns[stopIdx] lists every (pattern, position) that serves
	// the stop. A looping pattern that visits the same stop twice
	// contributes one entry per visit.
	StopPatterns [][]PatternStop

	// Routes and Services sorted by ID (slices, not maps — see the
	// determinism note on the struct).
	Routes   []Route
	Services []Service

	// Derived lookups, rebuilt by buildLookups; never serialized.
	stopIndex    map[string]int32
	tripIndex    map[string]int32
	routeIndex   map[string]int32
	serviceIndex map[string]int32
}

// Pattern is one unique ordered stop sequence and all trips that follow
// it — a "route" in RAPTOR terminology.
type Pattern struct {
	// Stops is the ordered stop sequence, stored once for all trips.
	Stops []int32

	// Trips holds tripIdx values sorted ascending by departure time at
	// Stops[0]. RAPTOR relies on this order to binary-search for the
	// earliest catchable trip.
	Trips []int32

	// RouteID is the GTFS route_id of the pattern's first trip, kept for
	// display. (If trips from different GTFS routes ever shared an
	// identical stop sequence they would share a pattern; use
	// Timetable.TripRouteIDs for the authoritative per-trip route.)
	RouteID string

	// ServiceIDs[i] is the GTFS service_id of Trips[i], used to filter
	// trips by operating date.
	ServiceIDs []string

	// Arrivals and Departures are flat time arrays: the time for trip i
	// at stop position p lives at index i*len(Stops)+p. Values are
	// seconds since midnight of the service day and may exceed 86400
	// for overnight trips (GTFS 25:30:00 = 91800). Each stop time from
	// the feed is stored exactly once, here.
	Arrivals   []int32
	Departures []int32
}

// NumTrips returns the number of trips following this pattern.
func (p *Pattern) NumTrips() int { return len(p.Trips) }

// Arrival returns the arrival time of the trip at position trip (index
// into p.Trips) at stop position pos (index into p.Stops).
func (p *Pattern) Arrival(trip, pos int) int32 {
	return p.Arrivals[trip*len(p.Stops)+pos]
}

// Departure returns the departure time of the trip at position trip at
// stop position pos.
func (p *Pattern) Departure(trip, pos int) int32 {
	return p.Departures[trip*len(p.Stops)+pos]
}

// PatternStop locates one visit of a pattern to a stop: Patterns[Pattern]
// serves the stop at position Pos of its stop sequence.
type PatternStop struct {
	Pattern int32
	Pos     int32
}

// Route keeps the GTFS route fields needed for display.
type Route struct {
	ID        string
	ShortName string
	LongName  string
}

// Service describes when a service_id runs. It mirrors gtfs.Service but
// stores exception dates as sorted slices instead of maps so gob output
// stays deterministic.
type Service struct {
	ID string
	// Weekdays is indexed by time.Weekday (0 = Sunday ... 6 = Saturday).
	Weekdays  [7]bool
	StartDate string // YYYYMMDD; empty if the service has no calendar.txt row
	EndDate   string // YYYYMMDD
	Added     []string // sorted YYYYMMDD dates added via calendar_dates
	Removed   []string // sorted YYYYMMDD dates removed via calendar_dates
}

// ActiveOn reports whether the service runs on the given date (YYYYMMDD)
// falling on the given weekday. Exceptions take precedence over the
// weekday pattern, matching gtfs.Service.ActiveOn.
func (s Service) ActiveOn(date string, weekday time.Weekday) bool {
	if _, removed := slices.BinarySearch(s.Removed, date); removed {
		return false
	}
	if _, added := slices.BinarySearch(s.Added, date); added {
		return true
	}
	if s.StartDate == "" || date < s.StartDate || date > s.EndDate {
		return false
	}
	return s.Weekdays[weekday]
}

// StopIdx returns the dense index for a GTFS stop_id.
func (tt *Timetable) StopIdx(stopID string) (int32, bool) {
	idx, ok := tt.stopIndex[stopID]
	return idx, ok
}

// TripIdx returns the dense index for a GTFS trip_id.
func (tt *Timetable) TripIdx(tripID string) (int32, bool) {
	idx, ok := tt.tripIndex[tripID]
	return idx, ok
}

// RouteByID returns the display info for a GTFS route_id.
func (tt *Timetable) RouteByID(routeID string) (Route, bool) {
	i, ok := tt.routeIndex[routeID]
	if !ok {
		return Route{}, false
	}
	return tt.Routes[i], true
}

// ServiceByID returns the calendar for a GTFS service_id.
func (tt *Timetable) ServiceByID(serviceID string) (Service, bool) {
	i, ok := tt.serviceIndex[serviceID]
	if !ok {
		return Service{}, false
	}
	return tt.Services[i], true
}

// TripRouteShortName resolves a tripIdx to its GTFS route short name
// (e.g. "40" or "E Line") for display. Returns "" if the route is
// unknown.
func (tt *Timetable) TripRouteShortName(trip int32) string {
	r, ok := tt.RouteByID(tt.TripRouteIDs[trip])
	if !ok {
		return ""
	}
	return r.ShortName
}

// buildLookups (re)derives the unexported string->index maps from the
// persisted slices. Called at the end of Build and after Load.
func (tt *Timetable) buildLookups() {
	tt.stopIndex = make(map[string]int32, len(tt.StopIDs))
	for i, id := range tt.StopIDs {
		tt.stopIndex[id] = int32(i)
	}
	tt.tripIndex = make(map[string]int32, len(tt.TripIDs))
	for i, id := range tt.TripIDs {
		tt.tripIndex[id] = int32(i)
	}
	tt.routeIndex = make(map[string]int32, len(tt.Routes))
	for i, r := range tt.Routes {
		tt.routeIndex[r.ID] = int32(i)
	}
	tt.serviceIndex = make(map[string]int32, len(tt.Services))
	for i, s := range tt.Services {
		tt.serviceIndex[s.ID] = int32(i)
	}
}
