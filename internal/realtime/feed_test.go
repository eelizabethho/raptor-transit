package realtime

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"
)

// buildFeed constructs a GTFS-Realtime message in memory, so the tests need
// no network and no checked-in binary fixture.
func buildFeed(t *testing.T, ents ...*gtfs.FeedEntity) []byte {
	t.Helper()
	ver, inc := "2.0", gtfs.FeedHeader_FULL_DATASET
	ts := uint64(1788121099)
	msg := &gtfs.FeedMessage{
		Header: &gtfs.FeedHeader{GtfsRealtimeVersion: &ver, Incrementality: &inc, Timestamp: &ts},
		Entity: ents,
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func tripUpdate(id, tripID string, rel gtfs.TripDescriptor_ScheduleRelationship, stops ...stopDelay) *gtfs.FeedEntity {
	td := &gtfs.TripDescriptor{TripId: &tripID, ScheduleRelationship: &rel}
	tu := &gtfs.TripUpdate{Trip: td}
	for _, sd := range stops {
		sd := sd
		stu := &gtfs.TripUpdate_StopTimeUpdate{StopId: &sd.stopID}
		if sd.hasArr {
			stu.Arrival = &gtfs.TripUpdate_StopTimeEvent{Delay: &sd.arr}
		}
		if sd.hasDep {
			stu.Departure = &gtfs.TripUpdate_StopTimeEvent{Delay: &sd.dep}
		}
		tu.StopTimeUpdate = append(tu.StopTimeUpdate, stu)
	}
	return &gtfs.FeedEntity{Id: &id, TripUpdate: tu}
}

type stopDelay struct {
	stopID string
	arr    int32
	dep    int32
	hasArr bool
	hasDep bool
}

func arrDelay(stopID string, d int32) stopDelay {
	return stopDelay{stopID: stopID, arr: d, hasArr: true}
}

func TestParseDelays(t *testing.T) {
	b := buildFeed(t,
		tripUpdate("e1", "T1", gtfs.TripDescriptor_SCHEDULED,
			arrDelay("S1", 0), arrDelay("S2", 240), arrDelay("S3", -120)),
	)
	snap, err := Parse(b, time.Now())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, tc := range []struct {
		stop string
		want int32
	}{{"S1", 0}, {"S2", 240}, {"S3", -120}} {
		got, ok := snap.ArrivalDelay("T1", tc.stop)
		if !ok {
			t.Errorf("no delay for %s", tc.stop)
			continue
		}
		if got != tc.want {
			t.Errorf("delay(%s) = %d, want %d", tc.stop, got, tc.want)
		}
	}
	if _, ok := snap.ArrivalDelay("T1", "nope"); ok {
		t.Error("unknown stop reported a delay")
	}
	if _, ok := snap.ArrivalDelay("nope", "S1"); ok {
		t.Error("unknown trip reported a delay")
	}
}

// A live sample contained a stop reporting 2489 seconds early. Values that
// far outside the plausible band are data faults, and letting one through
// would have the router recommend a bus that has already gone.
func TestParseRejectsImplausibleDelays(t *testing.T) {
	b := buildFeed(t,
		tripUpdate("e1", "T1", gtfs.TripDescriptor_SCHEDULED,
			arrDelay("keep", 300),
			arrDelay("tooEarly", -2489),
			arrDelay("tooLate", MaxLateSeconds+1),
		),
	)
	snap, err := Parse(b, time.Now())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := snap.ArrivalDelay("T1", "keep"); !ok {
		t.Error("plausible delay was dropped")
	}
	for _, s := range []string{"tooEarly", "tooLate"} {
		if d, ok := snap.ArrivalDelay("T1", s); ok {
			t.Errorf("implausible delay for %s was kept (%d)", s, d)
		}
	}
	if snap.Stats.Rejected != 2 {
		t.Errorf("Stats.Rejected = %d, want 2", snap.Stats.Rejected)
	}
}

func TestParseCanceledTrip(t *testing.T) {
	b := buildFeed(t,
		tripUpdate("e1", "GONE", gtfs.TripDescriptor_CANCELED, arrDelay("S1", 60)),
		tripUpdate("e2", "RUNS", gtfs.TripDescriptor_SCHEDULED, arrDelay("S1", 60)),
	)
	snap, err := Parse(b, time.Now())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !snap.IsCanceled("GONE") {
		t.Error("canceled trip not recorded")
	}
	if snap.IsCanceled("RUNS") {
		t.Error("scheduled trip reported canceled")
	}
	if _, ok := snap.ArrivalDelay("GONE", "S1"); ok {
		t.Error("canceled trip contributed a delay")
	}
}

// A loop route visits the same stop twice on one trip. The overlay keys on
// (trip, stop), so the first value wins and the collision is counted rather
// than silently overwriting.
func TestParseDuplicateStopKeepsFirst(t *testing.T) {
	b := buildFeed(t,
		tripUpdate("e1", "LOOP", gtfs.TripDescriptor_SCHEDULED,
			arrDelay("S1", 60), arrDelay("S1", 600)),
	)
	snap, err := Parse(b, time.Now())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d, _ := snap.ArrivalDelay("LOOP", "S1"); d != 60 {
		t.Errorf("delay = %d, want the first value 60", d)
	}
	if snap.Stats.DuplicateStops != 1 {
		t.Errorf("Stats.DuplicateStops = %d, want 1", snap.Stats.DuplicateStops)
	}
}

func TestParseSkipsUnjoinableUpdates(t *testing.T) {
	noStop := stopDelay{stopID: "", arr: 60, hasArr: true}
	b := buildFeed(t,
		tripUpdate("e1", "", gtfs.TripDescriptor_SCHEDULED, arrDelay("S1", 60)),
		tripUpdate("e2", "T2", gtfs.TripDescriptor_SCHEDULED, noStop),
	)
	snap, err := Parse(b, time.Now())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(snap.arrival) != 0 {
		t.Errorf("kept %d unjoinable updates, want 0", len(snap.arrival))
	}
}

func TestParseGarbageIsAnError(t *testing.T) {
	if _, err := Parse([]byte("this is not protobuf at all, not even close"), time.Now()); err == nil {
		t.Error("expected an error decoding garbage")
	}
}

func TestNilSnapshotIsSafe(t *testing.T) {
	var s *Snapshot
	if _, ok := s.ArrivalDelay("T", "S"); ok {
		t.Error("nil snapshot reported a delay")
	}
	if s.IsCanceled("T") {
		t.Error("nil snapshot reported a cancellation")
	}
}

// TestLiveFeed exercises the real KCM endpoint. It skips rather than fails
// when the network is unavailable, so CI without egress still passes.
func TestLiveFeed(t *testing.T) {
	if testing.Short() {
		t.Skip("network test; -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	snap, err := Fetch(ctx, &http.Client{Timeout: 60 * time.Second}, KCMTripUpdatesURL)
	if err != nil {
		t.Skipf("live feed unreachable: %v", err)
	}
	if snap.Stats.TripUpdates == 0 {
		t.Error("live feed carried no trip updates")
	}
	if snap.Stats.ArrivalDelays == 0 {
		t.Error("live feed carried no arrival delays")
	}
	t.Logf("live: %d trips, %d stop-time updates, %d delays, %d rejected, %d canceled, feed_time=%s",
		snap.Stats.TripUpdates, snap.Stats.StopTimeUpdates, snap.Stats.ArrivalDelays,
		snap.Stats.Rejected, snap.Stats.CanceledTrips, snap.FeedTime.Format(time.RFC3339))
}
