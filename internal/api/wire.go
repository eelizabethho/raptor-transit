// Package api implements the raptor-transit HTTP interface.
//
// The wire types in this file are deliberately separate from the engine's
// internal types (raptor.Journey, raptor.Leg). Those speak in
// seconds-since-midnight int32s that may exceed 86,400, which is the right
// representation for the algorithm and a hostile one for a client: nobody
// consuming JSON wants to discover that 91800 means 01:30 the next morning.
// Keeping a translation layer here means the engine can keep its compact
// encoding while the API stays readable, and neither constrains the other.
package api

import (
	"fmt"

	"raptor-transit/internal/raptor"
	"raptor-transit/internal/timetable"
)

// StopRef identifies a stop in a response.
type StopRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Leg is one piece of a journey on the wire.
type Leg struct {
	// Mode is "transit" or "walk".
	Mode string `json:"mode"`
	// Route is the rider-facing route designation (KCM's route_long_name is
	// always empty, so this is route_short_name). Empty for walking legs.
	Route string `json:"route,omitempty"`
	// TripID is the GTFS trip_id, for clients that want to cross-reference
	// the feed or (later) a realtime update. Empty for walking legs.
	TripID string `json:"trip_id,omitempty"`

	From StopRef `json:"from"`
	To   StopRef `json:"to"`

	// Clock times as HH:MM:SS on the query date. A leg that lands after
	// midnight reports the raw >24h clock (e.g. "25:30:00") and sets
	// NextDay, rather than silently wrapping to 01:30:00 and looking like
	// it arrives before it departs.
	Departure string `json:"departure"`
	Arrival   string `json:"arrival"`
	NextDay   bool   `json:"next_day,omitempty"`

	// DepartureSeconds and ArrivalSeconds are the raw values, for clients
	// doing arithmetic that shouldn't have to re-parse the clock strings.
	DepartureSeconds int32 `json:"departure_seconds"`
	ArrivalSeconds   int32 `json:"arrival_seconds"`
}

// Journey is one itinerary on the wire.
type Journey struct {
	Departure        string `json:"departure"`
	Arrival          string `json:"arrival"`
	DepartureSeconds int32  `json:"departure_seconds"`
	ArrivalSeconds   int32  `json:"arrival_seconds"`
	// DurationSeconds is Arrival minus Departure: time spent travelling,
	// from boarding the first vehicle (or starting the first walk) to
	// arrival. It deliberately excludes the wait between the requested
	// `at` time and the actual departure — two journeys leaving at
	// different times shouldn't be compared by how long the rider stood
	// at the stop. Clients wanting door-to-door can subtract
	// DepartureSeconds from the requested time themselves.
	DurationSeconds int32 `json:"duration_seconds"`
	// NextDay reports that the journey arrives after midnight on the
	// service date, mirroring the per-leg flag so a client reading only
	// journey-level times doesn't have to infer it from ArrivalSeconds.
	NextDay bool `json:"next_day,omitempty"`
	// Transfers counts transit legs minus one; walking legs don't count.
	Transfers int   `json:"transfers"`
	Legs      []Leg `json:"legs"`
}

// RouteResponse is the body of a successful GET /route.
type RouteResponse struct {
	Origin      StopRef `json:"origin"`
	Destination StopRef `json:"destination"`
	Date        string  `json:"date"`
	DepartAfter string  `json:"depart_after"`
	// Journeys is never null — an unreachable query returns [] with 200.
	Journeys []Journey `json:"journeys"`
	// Notes carries non-fatal context, e.g. that the requested date falls
	// outside the feed's service window (which is why the list is empty).
	Notes []string `json:"notes,omitempty"`
}

// StopsResponse is the body of a successful GET /stops.
type StopsResponse struct {
	Query string    `json:"query"`
	Stops []StopRef `json:"stops"`
}

// ErrorResponse is the body of any 4xx/5xx response.
type ErrorResponse struct {
	Error string `json:"error"`
	// Candidates is populated when an ambiguous stop name was the problem,
	// so a client can render a disambiguation list instead of a dead end.
	Candidates []StopRef `json:"candidates,omitempty"`
}

// clock renders seconds-since-midnight as HH:MM:SS, keeping hours past 24
// intact. It mirrors cmd/route's formatting but returns the next-day flag
// separately instead of appending a "+1d" marker to the string.
func clock(sec int32) (string, bool) {
	next := sec >= 86400
	return fmt.Sprintf("%02d:%02d:%02d", sec/3600, sec%3600/60, sec%60), next
}

// stopRef builds a StopRef from a timetable stop index, falling back to the
// stop_id when the feed has no name for the stop.
func stopRef(tt *timetable.Timetable, idx int32) StopRef {
	name := tt.StopNames[idx]
	if name == "" {
		name = tt.StopIDs[idx]
	}
	return StopRef{ID: tt.StopIDs[idx], Name: name}
}

// toWireJourney converts an engine journey into its wire form.
func toWireJourney(tt *timetable.Timetable, j raptor.Journey) Journey {
	dep, _ := clock(j.Departure)
	arr, nextDay := clock(j.Arrival)
	out := Journey{
		Departure:        dep,
		Arrival:          arr,
		DepartureSeconds: j.Departure,
		ArrivalSeconds:   j.Arrival,
		DurationSeconds:  j.Arrival - j.Departure,
		NextDay:          nextDay,
		Transfers:        j.Transfers,
		Legs:             make([]Leg, 0, len(j.Legs)),
	}
	for _, leg := range j.Legs {
		legDep, _ := clock(leg.Departure)
		legArr, nextDay := clock(leg.Arrival)
		wire := Leg{
			Mode:             "transit",
			Route:            leg.Route,
			From:             stopRef(tt, leg.FromStop),
			To:               stopRef(tt, leg.ToStop),
			Departure:        legDep,
			Arrival:          legArr,
			NextDay:          nextDay,
			DepartureSeconds: leg.Departure,
			ArrivalSeconds:   leg.Arrival,
		}
		if leg.Trip < 0 {
			// Walking legs carry no route or trip identity.
			wire.Mode = "walk"
			wire.Route = ""
		} else {
			wire.TripID = tt.TripIDs[leg.Trip]
		}
		out.Legs = append(out.Legs, wire)
	}
	return out
}
