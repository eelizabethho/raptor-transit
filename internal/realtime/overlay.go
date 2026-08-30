package realtime

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"raptor-transit/internal/timetable"
)

// Overlay adapts a Snapshot to raptor.Delays, translating the engine's
// dense timetable indices into the GTFS ids the feed speaks.
//
// It holds no mutable state: the snapshot it wraps is immutable, so an
// Overlay is safe to share across goroutines for the life of a query.
type Overlay struct {
	tt   *timetable.Timetable
	snap *Snapshot
}

// NewOverlay wraps a snapshot for use as a raptor.Delays. A nil snapshot
// yields an overlay that reports no delays.
func NewOverlay(tt *timetable.Timetable, snap *Snapshot) *Overlay {
	return &Overlay{tt: tt, snap: snap}
}

// Delay implements raptor.Delays. Unknown stop events report 0 — "no
// information", which the engine treats as running to schedule. That is a
// deliberate choice: a missing update must not be read as a guarantee, and
// the alternative (refusing to route) would make the API less useful than
// the schedule alone.
//
// Nil-receiver-safe, so a typed-nil *Overlay passed as a raptor.Delays
// behaves like no overlay rather than panicking.
func (o *Overlay) Delay(trip, stop int32) int32 {
	if o == nil || o.snap == nil {
		return 0
	}
	if trip < 0 || int(trip) >= len(o.tt.TripIDs) || stop < 0 || int(stop) >= len(o.tt.StopIDs) {
		return 0
	}
	d, ok := o.snap.ArrivalDelay(o.tt.TripIDs[trip], o.tt.StopIDs[stop])
	if !ok {
		return 0
	}
	return d
}

// Snapshot returns the wrapped snapshot, for handlers that want to report
// feed age or stats alongside the result.
func (o *Overlay) Snapshot() *Snapshot {
	if o == nil {
		return nil
	}
	return o.snap
}

// Poller keeps a current Snapshot by refetching the feed on an interval.
//
// The current snapshot is published through an atomic pointer: readers
// load it without locking and never observe a half-built one, because a
// snapshot is fully constructed before it is stored and never mutated
// afterwards.
type Poller struct {
	url      string
	interval time.Duration
	client   *http.Client

	cur atomic.Pointer[Snapshot]
}

// NewPoller creates a poller. It does not start fetching; call Run.
func NewPoller(url string, interval time.Duration, client *http.Client) *Poller {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Poller{url: url, interval: interval, client: client}
}

// Current returns the most recent snapshot, or nil before the first
// successful fetch.
func (p *Poller) Current() *Snapshot { return p.cur.Load() }

// Refresh performs one fetch and publishes the result on success.
func (p *Poller) Refresh(ctx context.Context) error {
	snap, err := Fetch(ctx, p.client, p.url)
	if err != nil {
		return err
	}
	p.cur.Store(snap)
	return nil
}

// Run polls until ctx is cancelled. A failed fetch is logged and the
// previous snapshot is kept rather than cleared: slightly stale delays
// beat none, and the consumer can judge staleness via Snapshot.Age.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		if err := p.Refresh(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("realtime refresh failed", "url", p.url, "error", err)
		} else if s := p.Current(); s != nil {
			slog.Info("realtime refreshed",
				"trips", s.Stats.TripUpdates,
				"stop_time_updates", s.Stats.StopTimeUpdates,
				"rejected", s.Stats.Rejected,
				"canceled", s.Stats.CanceledTrips,
				"feed_time", s.FeedTime.Format(time.RFC3339),
			)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
