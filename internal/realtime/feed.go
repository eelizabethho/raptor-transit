// Package realtime ingests a GTFS-Realtime TripUpdates feed and exposes it
// as an immutable delay overlay over the static timetable.
//
// The overlay is never merged into the timetable. timetable.Timetable is
// shared, unsynchronised, by every concurrent request precisely because it
// is read-only after load; writing live delays into it would make that
// sharing unsound. Instead a Snapshot is built off to the side, published
// atomically, and consulted at query time.
//
// This package is where the project's stdlib-only rule ends: GTFS-Realtime
// is protobuf and there is no stdlib path to decoding it. See
// docs/CONTEXT.md.
package realtime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"
)

// KCMTripUpdatesURL is King County Metro's public TripUpdates feed.
const KCMTripUpdatesURL = "https://s3.amazonaws.com/kcm-alerts-realtime-prod/tripupdates.pb"

// Delay bounds. Feeds carry garbage: a live KCM sample contained a stop
// reporting 2489 seconds *early*, which is a data fault rather than a bus
// 41 minutes ahead of schedule. Values outside these bounds are dropped so
// one bad row can't make the router recommend a departure that already
// left, or invent one far in the future.
const (
	MaxEarlySeconds = -600  // 10 min early
	MaxLateSeconds  = 10800 // 3 hours late
)

// Key identifies one scheduled stop event: a GTFS trip_id plus stop_id.
//
// stop_id rather than stop_sequence, because the static timetable does not
// retain the feed's stop_sequence values — only the ordered position of
// each stop in its pattern — and GTFS does not require sequences to be
// 1-based or contiguous, so pos+1 is not a safe join key across agencies.
//
// The cost is loop routes: a trip visiting the same stop twice gets one
// delay for both visits. Stats.DuplicateStops counts how often that
// happens so the limitation is measurable rather than silent.
type Key struct {
	TripID string
	StopID string
}

// Snapshot is an immutable set of delays parsed from one feed poll. It is
// safe for concurrent readers; updating means building a new Snapshot and
// swapping the pointer, never mutating one in place.
type Snapshot struct {
	// FeedTime is the header timestamp: when the agency generated the feed.
	FeedTime time.Time
	// FetchedAt is when we retrieved it, for staleness checks that don't
	// trust the agency clock.
	FetchedAt time.Time

	arrival   map[Key]int32
	departure map[Key]int32
	// canceled holds trips the feed reports as not running.
	canceled map[string]bool

	// Counters describing what the parse saw, for logging and for the
	// Phase 5 delay-history pipeline.
	Stats Stats
}

// Stats summarises one parsed feed.
type Stats struct {
	Entities        int
	TripUpdates     int
	StopTimeUpdates int
	ArrivalDelays   int
	DepartureDelays int
	CanceledTrips   int
	// Rejected counts stop-time updates dropped for an out-of-bounds delay.
	Rejected int
	// DuplicateStops counts stop-time updates for a (trip, stop) already
	// seen — a loop route revisiting a stop. The first value wins.
	DuplicateStops int
}

// ArrivalDelay returns the live arrival delay in seconds for a scheduled
// stop event, and whether the feed had one. A missing entry means "no
// information", which callers must treat as on-schedule rather than
// on-time-guaranteed.
func (s *Snapshot) ArrivalDelay(tripID, stopID string) (int32, bool) {
	if s == nil {
		return 0, false
	}
	d, ok := s.arrival[Key{tripID, stopID}]
	return d, ok
}

// DepartureDelay is ArrivalDelay for the departure side.
func (s *Snapshot) DepartureDelay(tripID, stopID string) (int32, bool) {
	if s == nil {
		return 0, false
	}
	d, ok := s.departure[Key{tripID, stopID}]
	return d, ok
}

// IsCanceled reports whether the feed says this trip is not running.
func (s *Snapshot) IsCanceled(tripID string) bool {
	if s == nil {
		return false
	}
	return s.canceled[tripID]
}

// Age returns how long ago the feed was fetched.
func (s *Snapshot) Age(now time.Time) time.Duration {
	if s == nil {
		return 0
	}
	return now.Sub(s.FetchedAt)
}

// Parse decodes a GTFS-Realtime FeedMessage into a Snapshot.
func Parse(b []byte, fetchedAt time.Time) (*Snapshot, error) {
	var msg gtfs.FeedMessage
	if err := proto.Unmarshal(b, &msg); err != nil {
		return nil, fmt.Errorf("realtime: decode feed: %w", err)
	}
	if v := msg.GetHeader().GetGtfsRealtimeVersion(); v != "" && v != "2.0" && v != "1.0" {
		return nil, fmt.Errorf("realtime: unsupported gtfs-rt version %q", v)
	}

	snap := &Snapshot{
		FeedTime:  time.Unix(int64(msg.GetHeader().GetTimestamp()), 0).UTC(),
		FetchedAt: fetchedAt,
		arrival:   make(map[Key]int32),
		departure: make(map[Key]int32),
		canceled:  make(map[string]bool),
	}
	snap.Stats.Entities = len(msg.GetEntity())

	for _, e := range msg.GetEntity() {
		tu := e.GetTripUpdate()
		if tu == nil {
			continue
		}
		snap.Stats.TripUpdates++
		tripID := tu.GetTrip().GetTripId()
		if tripID == "" {
			// Without a trip_id we cannot join to the static feed. KCM
			// always sets it; other agencies use route+start_time, which
			// would need a resolver we don't have.
			continue
		}
		if tu.GetTrip().GetScheduleRelationship() == gtfs.TripDescriptor_CANCELED {
			snap.canceled[tripID] = true
			snap.Stats.CanceledTrips++
			continue
		}

		for _, stu := range tu.GetStopTimeUpdate() {
			snap.Stats.StopTimeUpdates++
			stopID := stu.GetStopId()
			if stopID == "" {
				// Without a stop_id we cannot join to the timetable; the
				// alternative key (stop_sequence) is not retained there.
				continue
			}
			k := Key{tripID, stopID}
			if _, seen := snap.arrival[k]; seen {
				snap.Stats.DuplicateStops++
				continue
			}
			if arr := stu.GetArrival(); arr != nil && arr.Delay != nil {
				if d, ok := clampDelay(arr.GetDelay()); ok {
					snap.arrival[k] = d
					snap.Stats.ArrivalDelays++
				} else {
					snap.Stats.Rejected++
				}
			}
			if dep := stu.GetDeparture(); dep != nil && dep.Delay != nil {
				if d, ok := clampDelay(dep.GetDelay()); ok {
					snap.departure[k] = d
					snap.Stats.DepartureDelays++
				} else {
					snap.Stats.Rejected++
				}
			}
		}
	}
	return snap, nil
}

// clampDelay rejects values outside the plausible band. It returns the
// delay unchanged when acceptable; it deliberately does not saturate to the
// bound, since a nonsense value shouldn't become a merely-implausible one.
func clampDelay(d int32) (int32, bool) {
	if d < MaxEarlySeconds || d > MaxLateSeconds {
		return 0, false
	}
	return d, true
}

// Fetch retrieves and parses the feed at url.
func Fetch(ctx context.Context, client *http.Client, url string) (*Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("realtime: fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("realtime: fetch %s: status %s", url, resp.Status)
	}
	// The KCM feed is ~900 KB; cap the read so a misbehaving endpoint
	// can't exhaust memory.
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("realtime: read %s: %w", url, err)
	}
	return Parse(b, time.Now().UTC())
}
