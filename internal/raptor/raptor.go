// Package raptor implements the RAPTOR transit routing algorithm
// (Delling, Pajor, Werneck 2015, §3.1) over the stop-pattern timetable
// in internal/timetable.
//
// RAPTOR works in rounds: round k computes the earliest arrival at every
// stop using at most k transit legs. Each round scans only the patterns
// that serve stops improved in the previous round, then applies walking
// footpaths. The per-round labels form a Pareto set over (arrival time,
// number of transfers).
package raptor

import (
	"fmt"
	"math"

	"raptor-transit/internal/calendar"
	"raptor-transit/internal/timetable"
	"raptor-transit/internal/transfers"
)

// MaxRounds bounds the number of transit legs considered.
const MaxRounds = 6

const infinity = int32(math.MaxInt32)

// Engine holds the immutable data shared by all queries. Safe for
// concurrent use: Query allocates its own working state.
type Engine struct {
	tt *timetable.Timetable

	// footFrom[stop] lists walking links leaving stop. There are no
	// self-entries; a stop is implicitly reachable from itself at zero
	// cost (see internal/transfers).
	footFrom [][]transfers.Footpath
}

// New builds an engine from a timetable and generated footpaths.
func New(tt *timetable.Timetable, paths []transfers.Footpath) *Engine {
	e := &Engine{tt: tt, footFrom: make([][]transfers.Footpath, len(tt.StopIDs))}
	for _, fp := range paths {
		e.footFrom[fp.From] = append(e.footFrom[fp.From], fp)
	}
	return e
}

// Leg is one piece of a journey: a transit ride (Trip >= 0) or a walk
// (Trip == -1, Route empty). Times are seconds since midnight of the
// query date; a leg on an overnight trip boarded from the previous
// service day still reports today-clock times.
type Leg struct {
	Route     string
	Trip      int32
	FromStop  int32
	ToStop    int32
	Departure int32
	Arrival   int32
}

// Journey is one Pareto-optimal itinerary.
type Journey struct {
	Legs      []Leg
	Departure int32 // departure from the source stop
	Arrival   int32 // arrival at the target stop
	Transfers int   // transit legs minus one (walks don't count)
}

// parent records how a stop's round-k label was achieved, for journey
// reconstruction. prevRound is the round whose label at leg.FromStop the
// leg extended.
type parent struct {
	leg       Leg
	prevRound int
	set       bool
}

// Query computes Pareto-optimal journeys (arrival time vs transfers) from
// source to target departing at or after depTime (seconds since midnight,
// 0..86399) on the given service date (YYYYMMDD). Journeys are returned
// in increasing-transfer order, each with a strictly earlier arrival than
// the previous; empty slice if nothing is reachable within MaxRounds.
func (e *Engine) Query(sourceID, targetID string, depTime int32, date string) ([]Journey, error) {
	source, ok := e.tt.StopIdx(sourceID)
	if !ok {
		return nil, fmt.Errorf("unknown source stop %q", sourceID)
	}
	target, ok := e.tt.StopIdx(targetID)
	if !ok {
		return nil, fmt.Errorf("unknown target stop %q", targetID)
	}
	day, err := calendar.ParseDate(date)
	if err != nil {
		return nil, fmt.Errorf("bad date %q (want YYYYMMDD): %w", date, err)
	}

	// A trip is catchable today if its service runs today, or if its
	// service ran YESTERDAY and the relevant times exceed 86400: GTFS
	// timetables overnight trips on the day they started, so yesterday's
	// 25:30:00 departure is 01:30:00 on today's clock.
	activeToday := calendar.ActiveServices(e.tt.Services, day)
	activeYesterday := calendar.ActiveServices(e.tt.Services, day.AddDate(0, 0, -1))

	nStops := len(e.tt.StopIDs)
	// tauPrev: labels of the previous round (boarding reads these — the
	// textbook k-1 array). tau: labels of the current round. tauBest:
	// best over all rounds, for local pruning.
	tauPrev := make([]int32, nStops)
	tau := make([]int32, nStops)
	tauBest := make([]int32, nStops)
	for i := range tau {
		tauPrev[i], tau[i], tauBest[i] = infinity, infinity, infinity
	}
	parents := make([][]parent, MaxRounds+1)
	for k := range parents {
		parents[k] = make([]parent, nStops)
	}

	tauPrev[source], tau[source], tauBest[source] = depTime, depTime, depTime
	marked := map[int32]bool{source: true}

	// Round 0: walking from the source counts as part of the initial
	// state, not as a transfer.
	for _, fp := range e.footFrom[source] {
		arr := depTime + fp.Seconds
		if arr < tauBest[fp.To] {
			tauPrev[fp.To], tau[fp.To], tauBest[fp.To] = arr, arr, arr
			parents[0][fp.To] = parent{
				leg:       Leg{Trip: -1, FromStop: source, ToStop: fp.To, Departure: depTime, Arrival: arr},
				prevRound: 0, set: true,
			}
			marked[fp.To] = true
		}
	}

	var journeys []Journey
	bestTargetArrival := infinity

	for k := 1; k <= MaxRounds && len(marked) > 0; k++ {
		// Patterns serving any marked stop, with the earliest marked
		// position on each (positions before it cannot have improved).
		queue := map[int32]int32{}
		for s := range marked {
			for _, ps := range e.tt.StopPatterns[s] {
				if pos, ok := queue[ps.Pattern]; !ok || ps.Pos < pos {
					queue[ps.Pattern] = ps.Pos
				}
			}
		}
		marked = map[int32]bool{}
		improvedTarget := false

		for pi, startPos := range queue {
			p := &e.tt.Patterns[pi]
			trip := -1       // index into p.Trips of the trip we're riding
			var offset int32 // 0 = today's trip, -86400 = yesterday's overnight
			var boardPos int
			var boardDep int32

			for pos := int(startPos); pos < len(p.Stops); pos++ {
				stop := p.Stops[pos]

				// Alight if riding improves this stop. Target pruning:
				// arrivals at or past the best target arrival can't
				// contribute to the Pareto set.
				if trip >= 0 {
					if arr := p.Arrival(trip, pos) + offset; arr < tauBest[stop] && arr < bestTargetArrival {
						tau[stop] = arr
						tauBest[stop] = arr
						parents[k][stop] = parent{
							leg: Leg{
								Route:     e.tt.TripRouteShortName(p.Trips[trip]),
								Trip:      p.Trips[trip],
								FromStop:  p.Stops[boardPos],
								ToStop:    stop,
								Departure: boardDep,
								Arrival:   arr,
							},
							prevRound: k - 1, set: true,
						}
						marked[stop] = true
						if stop == target {
							bestTargetArrival = arr
							improvedTarget = true
						}
					}
				}

				// Board (or upgrade to) the earliest catchable trip using
				// the previous round's label at this stop.
				if ready := tauPrev[stop]; ready != infinity {
					if t, off, dep := e.earliestTrip(p, pos, ready, activeToday, activeYesterday); t >= 0 {
						if trip < 0 || dep < p.Departure(trip, pos)+offset {
							trip, offset, boardPos, boardDep = t, off, pos, dep
						}
					}
				}
			}
		}

		// Footpath relaxation from stops improved by transit this round.
		// Iterate a snapshot: walks found here must not chain further
		// within the round (a chained walk would exceed maxMeters).
		transitMarked := make([]int32, 0, len(marked))
		for s := range marked {
			transitMarked = append(transitMarked, s)
		}
		for _, s := range transitMarked {
			base := tau[s]
			for _, fp := range e.footFrom[s] {
				arr := base + fp.Seconds
				if arr < tauBest[fp.To] && arr < bestTargetArrival {
					tau[fp.To] = arr
					tauBest[fp.To] = arr
					parents[k][fp.To] = parent{
						leg:       Leg{Trip: -1, FromStop: s, ToStop: fp.To, Departure: base, Arrival: arr},
						prevRound: k, set: true,
					}
					marked[fp.To] = true
					if fp.To == target {
						bestTargetArrival = arr
						improvedTarget = true
					}
				}
			}
		}

		if improvedTarget {
			j, err := e.reconstruct(parents, source, target, k, depTime)
			if err != nil {
				return nil, err
			}
			journeys = append(journeys, j)
		}

		// Roll labels: next round boards using this round's results.
		copy(tauPrev, tau)
	}

	return journeys, nil
}

// reconstruct walks parent pointers back from target at round k to the
// source, then reverses the legs.
func (e *Engine) reconstruct(parents [][]parent, source, target int32, k int, depTime int32) (Journey, error) {
	var rev []Leg
	stop, round := target, k
	for stop != source {
		// The label used at (stop, round) was set at the highest round
		// r <= round with a parent entry.
		r := round
		for r >= 0 && !parents[r][stop].set {
			r--
		}
		if r < 0 {
			return Journey{}, fmt.Errorf("reconstruction: no parent for stop %s at round %d", e.tt.StopIDs[stop], round)
		}
		p := parents[r][stop]
		rev = append(rev, p.leg)
		stop, round = p.leg.FromStop, p.prevRound
		if len(rev) > 2*(MaxRounds+2) {
			return Journey{}, fmt.Errorf("reconstruction: parent cycle at stop %s", e.tt.StopIDs[stop])
		}
	}

	legs := make([]Leg, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		legs = append(legs, rev[i])
	}
	transit := 0
	for _, l := range legs {
		if l.Trip >= 0 {
			transit++
		}
	}
	j := Journey{
		Legs:      legs,
		Departure: depTime,
		Arrival:   legs[len(legs)-1].Arrival,
		Transfers: transit - 1,
	}
	if len(legs) > 0 {
		j.Departure = legs[0].Departure
	}
	return j, nil
}

// earliestTrip finds the earliest catchable trip of pattern p at stop
// position pos departing at or after ready (today-clock seconds). It
// considers today's trips as-is and yesterday's overnight trips shifted
// by -86400. Returns (index into p.Trips, day offset, departure on
// today's clock), or (-1, 0, 0).
//
// Trips are sorted by first-stop departure and GTFS patterns don't
// overtake in practice, but today's and yesterday's orderings interleave,
// so a full scan comparing both interpretations keeps this simple and
// correct. Patterns have few trips (tens to low hundreds), so the linear
// scan is cheap; binary search is a later optimization if profiles say so.
func (e *Engine) earliestTrip(p *timetable.Pattern, pos int, ready int32, today, yesterday map[string]bool) (int, int32, int32) {
	best, bestOff, bestDep := -1, int32(0), infinity
	for t := 0; t < p.NumTrips(); t++ {
		dep := p.Departure(t, pos)
		if dep >= ready && dep < bestDep && today[p.ServiceIDs[t]] {
			best, bestOff, bestDep = t, 0, dep
		}
		if ydep := dep - 86400; ydep >= 0 && ydep >= ready && ydep < bestDep && yesterday[p.ServiceIDs[t]] {
			best, bestOff, bestDep = t, -86400, ydep
		}
	}
	if best < 0 {
		return -1, 0, 0
	}
	return best, bestOff, bestDep
}
