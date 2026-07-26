package gtfs

import "time"

// Types for the subset of GTFS this project needs. Field names follow the
// GTFS spec names (stop_id -> StopID) so they're easy to cross-reference
// with https://gtfs.org/schedule/reference/.

// Stop is a row from stops.txt.
type Stop struct {
	ID   string
	Name string
	Lat  float64
	Lon  float64
}

// Route is a row from routes.txt.
type Route struct {
	ID        string
	ShortName string
	LongName  string
	Type      int
}

// Trip is a row from trips.txt.
type Trip struct {
	ID        string
	RouteID   string
	ServiceID string
}

// StopTime is a row from stop_times.txt. Arrival and Departure are
// seconds since midnight. GTFS allows times past 24:00:00 for trips that
// run overnight, so values greater than 86400 are valid and preserved
// (e.g. 25:30:00 = 91800).
type StopTime struct {
	TripID    string
	Arrival   int
	Departure int
	StopID    string
	Seq       int
}

// Service describes when a service_id runs, combining calendar.txt and
// calendar_dates.txt. A service may exist only in calendar_dates.txt
// (Weekdays all false, dates only in Added) — King County Metro does this.
type Service struct {
	ID string
	// Weekdays[time.Weekday] semantics: index 0 = Sunday ... 6 = Saturday,
	// matching Go's time.Weekday values.
	Weekdays  [7]bool
	StartDate string // YYYYMMDD, empty if service has no calendar.txt row
	EndDate   string // YYYYMMDD
	Added     map[string]bool // dates added via calendar_dates exception_type=1
	Removed   map[string]bool // dates removed via exception_type=2
}

// ActiveOn reports whether the service runs on the given date
// (YYYYMMDD) falling on the given weekday. calendar_dates exceptions
// take precedence over the calendar.txt weekday pattern, which is how a
// service defined only in calendar_dates.txt (empty pattern, Added dates
// only) resolves correctly.
func (s Service) ActiveOn(date string, weekday time.Weekday) bool {
	if s.Removed[date] {
		return false
	}
	if s.Added[date] {
		return true
	}
	if s.StartDate == "" || date < s.StartDate || date > s.EndDate {
		return false
	}
	return s.Weekdays[weekday]
}

// Feed holds everything parsed from a GTFS zip plus the indexes the
// RAPTOR phase will need. This is the single struct that gets
// gob-serialized by Save/Load.
type Feed struct {
	Stops    map[string]Stop
	Routes   map[string]Route
	Trips    map[string]Trip
	Services map[string]Service

	// StopTimes grouped per trip, ordered by stop_sequence.
	StopTimesByTrip map[string][]StopTime

	// Indexes.
	StopTimesByStop map[string][]StopTime // per stop, sorted by Departure
	TripRoute       map[string]string     // trip_id -> route_id
}

// NumStopTimes returns the total stop_times row count across all trips.
func (f *Feed) NumStopTimes() int {
	n := 0
	for _, sts := range f.StopTimesByTrip {
		n += len(sts)
	}
	return n
}
