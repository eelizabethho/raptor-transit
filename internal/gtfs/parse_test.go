package gtfs

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

// Expected values come from testdata/EXPECTED.md, which documents the
// hand-written mini fixture.

func loadMini(t *testing.T) *Feed {
	t.Helper()
	feed, err := ParseZip(filepath.Join("testdata", "mini.zip"))
	if err != nil {
		t.Fatalf("ParseZip: %v", err)
	}
	return feed
}

func TestParseCounts(t *testing.T) {
	feed := loadMini(t)
	if got := len(feed.Stops); got != 5 {
		t.Errorf("stops = %d, want 5", got)
	}
	if got := len(feed.Routes); got != 2 {
		t.Errorf("routes = %d, want 2", got)
	}
	if got := len(feed.Trips); got != 3 {
		t.Errorf("trips = %d, want 3", got)
	}
	if got := feed.NumStopTimes(); got != 11 {
		t.Errorf("stop_times = %d, want 11", got)
	}
	if got := len(feed.Services); got != 2 {
		t.Errorf("services = %d, want 2", got)
	}
}

// routes.txt carries a UTF-8 BOM and stops.txt uses CRLF; both must parse
// with clean header names and values.
func TestBOMAndCRLF(t *testing.T) {
	feed := loadMini(t)
	r, ok := feed.Routes["R1"]
	if !ok {
		t.Fatalf("route R1 missing — BOM likely corrupted the route_id header; got routes %v", feed.Routes)
	}
	if r.Type != 3 {
		t.Errorf("R1 route_type = %d, want 3", r.Type)
	}
	s, ok := feed.Stops["S1"]
	if !ok {
		t.Fatal("stop S1 missing — CRLF handling broken?")
	}
	if s.Name == "" || s.Lat == 0 {
		t.Errorf("S1 fields not populated: %+v", s)
	}
}

func TestOvernightTimesPreserved(t *testing.T) {
	feed := loadMini(t)
	sts := feed.StopTimesByTrip["R2_OWL1"]
	if len(sts) != 3 {
		t.Fatalf("R2_OWL1 stop_times = %d, want 3", len(sts))
	}
	// 25:30:00 = 91800, 25:45:00 = 92700 — both > 86400 and must not be
	// rejected or wrapped.
	if sts[0].Departure != 91800 {
		t.Errorf("first departure = %d, want 91800", sts[0].Departure)
	}
	if sts[2].Arrival != 92700 {
		t.Errorf("last arrival = %d, want 92700", sts[2].Arrival)
	}
	for _, st := range sts {
		if st.Arrival <= 86400 {
			t.Errorf("overnight arrival %d at seq %d not preserved as >86400", st.Arrival, st.Seq)
		}
	}
}

func TestParseTime(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"08:00:00", 28800, false},
		{"25:30:00", 91800, false},
		{"8:05:30", 29130, false},
		{"", -1, false},
		{"12:60:00", 0, true},
		{"noon", 0, true},
		{"12:00", 0, true},
	}
	for _, c := range cases {
		got, err := ParseTime(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseTime(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("ParseTime(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestServiceResolution(t *testing.T) {
	feed := loadMini(t)

	special, ok := feed.Services["SPECIAL"]
	if !ok {
		t.Fatal("SPECIAL service missing — calendar_dates-only services not parsed")
	}
	if special.StartDate != "" {
		t.Errorf("SPECIAL has StartDate %q, want empty (no calendar.txt row)", special.StartDate)
	}

	wkdy := feed.Services["WKDY"]
	cases := []struct {
		svc     Service
		date    string
		weekday time.Weekday
		want    bool
		why     string
	}{
		{wkdy, "20260310", time.Tuesday, true, "normal weekday"},
		{special, "20260310", time.Tuesday, false, "no exception that day"},
		{wkdy, "20260704", time.Saturday, false, "saturday=0"},
		{special, "20260704", time.Saturday, true, "added via calendar_dates"},
		{wkdy, "20261225", time.Friday, false, "removed by exception_type=2"},
		{special, "20261225", time.Friday, true, "added via calendar_dates"},
		{wkdy, "20261231", time.Thursday, true, "normal weekday"},
		{special, "20261231", time.Thursday, true, "added — both active same day"},
		{wkdy, "20260315", time.Sunday, false, "sunday=0"},
		{wkdy, "20270101", time.Friday, false, "past end_date"},
	}
	for _, c := range cases {
		if got := c.svc.ActiveOn(c.date, c.weekday); got != c.want {
			t.Errorf("%s.ActiveOn(%s) = %v, want %v (%s)", c.svc.ID, c.date, got, c.want, c.why)
		}
	}
}

func TestIndexes(t *testing.T) {
	feed := loadMini(t)

	if got := feed.TripRoute["R2_OWL1"]; got != "R2" {
		t.Errorf("TripRoute[R2_OWL1] = %q, want R2", got)
	}

	for stopID, sts := range feed.StopTimesByStop {
		if !sort.SliceIsSorted(sts, func(i, j int) bool { return sts[i].Departure < sts[j].Departure }) {
			t.Errorf("StopTimesByStop[%s] not sorted by departure", stopID)
		}
	}
	for tripID, sts := range feed.StopTimesByTrip {
		if !sort.SliceIsSorted(sts, func(i, j int) bool { return sts[i].Seq < sts[j].Seq }) {
			t.Errorf("StopTimesByTrip[%s] not sorted by stop_sequence", tripID)
		}
	}

	// Every stop_times row lands in the by-stop index exactly once.
	n := 0
	for _, sts := range feed.StopTimesByStop {
		n += len(sts)
	}
	if n != feed.NumStopTimes() {
		t.Errorf("by-stop index has %d rows, want %d", n, feed.NumStopTimes())
	}
}

func TestGobRoundTrip(t *testing.T) {
	feed := loadMini(t)
	path := filepath.Join(t.TempDir(), "mini.gob")
	if err := feed.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(feed, got) {
		t.Error("round-tripped feed differs from original")
	}
}
