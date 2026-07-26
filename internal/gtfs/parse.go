package gtfs

import (
	"archive/zip"
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// ParseZip opens a GTFS zip file and parses the files this project needs.
// Optional files (calendar.txt, calendar_dates.txt) may be absent or
// header-only; required files must exist.
func ParseZip(path string) (*Feed, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open gtfs zip: %w", err)
	}
	defer zr.Close()

	feed := &Feed{
		Stops:           map[string]Stop{},
		Routes:          map[string]Route{},
		Trips:           map[string]Trip{},
		Services:        map[string]Service{},
		StopTimesByTrip: map[string][]StopTime{},
		StopTimesByStop: map[string][]StopTime{},
		TripRoute:       map[string]string{},
	}

	// Files can live at the zip root or under a single top-level folder;
	// match by base name.
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		name := f.Name
		if i := strings.LastIndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		if name != "" {
			files[name] = f
		}
	}

	required := []string{"stops.txt", "routes.txt", "trips.txt", "stop_times.txt"}
	for _, name := range required {
		if files[name] == nil {
			return nil, fmt.Errorf("gtfs zip missing required file %s", name)
		}
	}

	if err := parseFile(files["stops.txt"], feed.parseStopRow); err != nil {
		return nil, fmt.Errorf("stops.txt: %w", err)
	}
	if err := parseFile(files["routes.txt"], feed.parseRouteRow); err != nil {
		return nil, fmt.Errorf("routes.txt: %w", err)
	}
	if err := parseFile(files["trips.txt"], feed.parseTripRow); err != nil {
		return nil, fmt.Errorf("trips.txt: %w", err)
	}
	if err := parseFile(files["stop_times.txt"], feed.parseStopTimeRow); err != nil {
		return nil, fmt.Errorf("stop_times.txt: %w", err)
	}
	// calendar.txt and calendar_dates.txt are both optional, but at least
	// one must contribute service definitions for the feed to be usable.
	if f := files["calendar.txt"]; f != nil {
		if err := parseFile(f, feed.parseCalendarRow); err != nil {
			return nil, fmt.Errorf("calendar.txt: %w", err)
		}
	}
	if f := files["calendar_dates.txt"]; f != nil {
		if err := parseFile(f, feed.parseCalendarDateRow); err != nil {
			return nil, fmt.Errorf("calendar_dates.txt: %w", err)
		}
	}
	if len(feed.Services) == 0 {
		return nil, fmt.Errorf("feed defines no services (calendar.txt and calendar_dates.txt empty or missing)")
	}

	feed.buildIndexes()
	return feed, nil
}

// row gives header-keyed access to one CSV record, so parsing is driven by
// column names rather than positions (feeds vary in column order and which
// optional columns they include).
type row struct {
	cols map[string]int
	rec  []string
}

// get returns the trimmed value for a column, or "" if the column is
// missing from this feed entirely.
func (r row) get(name string) string {
	i, ok := r.cols[name]
	if !ok || i >= len(r.rec) {
		return ""
	}
	return strings.TrimSpace(r.rec[i])
}

// parseFile streams one CSV file from the zip through fn, one row at a time.
func parseFile(zf *zip.File, fn func(row) error) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	cr := csv.NewReader(rc)
	cr.ReuseRecord = true
	// Some feeds have ragged rows (trailing columns omitted); tolerate them.
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err == io.EOF {
		return nil // header-only or fully empty file: nothing to parse
	}
	if err != nil {
		return err
	}
	cols := make(map[string]int, len(header))
	for i, h := range header {
		h = strings.TrimSpace(h)
		if i == 0 {
			// Strip a UTF-8 byte order mark; many transit agencies export
			// GTFS from Windows tools that add one.
			h = strings.TrimPrefix(h, "\ufeff")
		}
		cols[h] = i
	}

	for line := 2; ; line++ {
		rec, err := cr.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if err := fn(row{cols: cols, rec: rec}); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
	}
}

func (f *Feed) parseStopRow(r row) error {
	id := r.get("stop_id")
	if id == "" {
		return fmt.Errorf("empty stop_id")
	}
	// A malformed coordinate must be an error, not a silent (0,0): Phase 2
	// derives walking transfers from these coords, and a stop at (0,0)
	// would quietly poison nearest-neighbor distances.
	lat, err := strconv.ParseFloat(r.get("stop_lat"), 64)
	if err != nil {
		return fmt.Errorf("stop %s: bad stop_lat %q", id, r.get("stop_lat"))
	}
	lon, err := strconv.ParseFloat(r.get("stop_lon"), 64)
	if err != nil {
		return fmt.Errorf("stop %s: bad stop_lon %q", id, r.get("stop_lon"))
	}
	f.Stops[id] = Stop{ID: id, Name: r.get("stop_name"), Lat: lat, Lon: lon}
	return nil
}

func (f *Feed) parseRouteRow(r row) error {
	id := r.get("route_id")
	if id == "" {
		return fmt.Errorf("empty route_id")
	}
	rtype, _ := strconv.Atoi(r.get("route_type"))
	f.Routes[id] = Route{
		ID:        id,
		ShortName: r.get("route_short_name"),
		LongName:  r.get("route_long_name"),
		Type:      rtype,
	}
	return nil
}

func (f *Feed) parseTripRow(r row) error {
	id := r.get("trip_id")
	if id == "" {
		return fmt.Errorf("empty trip_id")
	}
	f.Trips[id] = Trip{ID: id, RouteID: r.get("route_id"), ServiceID: r.get("service_id")}
	return nil
}

func (f *Feed) parseStopTimeRow(r row) error {
	tripID := r.get("trip_id")
	if tripID == "" {
		return fmt.Errorf("empty trip_id")
	}
	arr, err := ParseTime(r.get("arrival_time"))
	if err != nil {
		return fmt.Errorf("arrival_time: %w", err)
	}
	dep, err := ParseTime(r.get("departure_time"))
	if err != nil {
		return fmt.Errorf("departure_time: %w", err)
	}
	seq, err := strconv.Atoi(r.get("stop_sequence"))
	if err != nil {
		return fmt.Errorf("stop_sequence: %w", err)
	}
	if r.get("stop_id") == "" {
		return fmt.Errorf("trip %s seq %d: empty stop_id", tripID, seq)
	}
	f.StopTimesByTrip[tripID] = append(f.StopTimesByTrip[tripID], StopTime{
		TripID:    tripID,
		Arrival:   arr,
		Departure: dep,
		StopID:    r.get("stop_id"),
		Seq:       seq,
	})
	return nil
}

func (f *Feed) parseCalendarRow(r row) error {
	id := r.get("service_id")
	if id == "" {
		return fmt.Errorf("empty service_id")
	}
	svc := f.service(id)
	// Go's time.Weekday: Sunday=0 ... Saturday=6.
	days := []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
	for i, day := range days {
		svc.Weekdays[i] = r.get(day) == "1"
	}
	svc.StartDate = r.get("start_date")
	svc.EndDate = r.get("end_date")
	f.Services[id] = svc
	return nil
}

func (f *Feed) parseCalendarDateRow(r row) error {
	id := r.get("service_id")
	if id == "" {
		return fmt.Errorf("empty service_id")
	}
	date := r.get("date")
	svc := f.service(id)
	switch r.get("exception_type") {
	case "1":
		svc.Added[date] = true
	case "2":
		svc.Removed[date] = true
	default:
		return fmt.Errorf("service %s date %s: bad exception_type %q", id, date, r.get("exception_type"))
	}
	f.Services[id] = svc
	return nil
}

// service returns the existing Service for id or a fresh one with maps
// initialized. Services can be introduced by either calendar file in
// either order.
func (f *Feed) service(id string) Service {
	if svc, ok := f.Services[id]; ok {
		return svc
	}
	return Service{ID: id, Added: map[string]bool{}, Removed: map[string]bool{}}
}

// ParseTime converts a GTFS HH:MM:SS time to seconds since midnight.
// Hours may exceed 23 for overnight trips (e.g. "25:30:00" -> 91800);
// H:MM:SS (single-digit hour) is also accepted. An empty string returns
// -1 with no error: stop_times may legitimately omit times on
// non-timepoint rows.
func ParseTime(s string) (int, error) {
	if s == "" {
		return -1, nil
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("bad time %q", s)
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	sec, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || h < 0 || m < 0 || m > 59 || sec < 0 || sec > 59 {
		return 0, fmt.Errorf("bad time %q", s)
	}
	return h*3600 + m*60 + sec, nil
}

// buildIndexes sorts per-trip stop_times by stop_sequence and derives the
// lookup structures RAPTOR needs.
func (f *Feed) buildIndexes() {
	for tripID, sts := range f.StopTimesByTrip {
		sort.Slice(sts, func(i, j int) bool { return sts[i].Seq < sts[j].Seq })
		f.StopTimesByTrip[tripID] = sts
	}

	for id, trip := range f.Trips {
		f.TripRoute[id] = trip.RouteID
	}

	for _, sts := range f.StopTimesByTrip {
		for _, st := range sts {
			f.StopTimesByStop[st.StopID] = append(f.StopTimesByStop[st.StopID], st)
		}
	}
	for stopID, sts := range f.StopTimesByStop {
		sort.Slice(sts, func(i, j int) bool { return sts[i].Departure < sts[j].Departure })
		f.StopTimesByStop[stopID] = sts
	}
}
