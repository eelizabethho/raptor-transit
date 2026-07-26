package gtfs

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip builds a GTFS zip from filename -> content, for malformed-feed
// tests.
func writeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "feed.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func validFiles() map[string]string {
	return map[string]string{
		"stops.txt":      "stop_id,stop_name,stop_lat,stop_lon\nS1,First,47.6,-122.3\n",
		"routes.txt":     "route_id,route_short_name,route_type\nR1,10,3\n",
		"trips.txt":      "route_id,service_id,trip_id\nR1,SVC,T1\n",
		"stop_times.txt": "trip_id,arrival_time,departure_time,stop_id,stop_sequence\nT1,08:00:00,08:00:00,S1,1\n",
		"calendar.txt":   "service_id,monday,tuesday,wednesday,thursday,friday,saturday,sunday,start_date,end_date\nSVC,1,1,1,1,1,0,0,20260101,20261231\n",
	}
}

func TestMalformedFeeds(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{
			name: "bad stop_lat errors instead of silent 0,0",
			mutate: func(f map[string]string) {
				f["stops.txt"] = "stop_id,stop_name,stop_lat,stop_lon\nS1,First,47.6x,-122.3\n"
			},
			wantErr: "bad stop_lat",
		},
		{
			name: "bad stop_lon errors",
			mutate: func(f map[string]string) {
				f["stops.txt"] = "stop_id,stop_name,stop_lat,stop_lon\nS1,First,47.6,\n"
			},
			wantErr: "bad stop_lon",
		},
		{
			name: "empty stop_id in stop_times rejected",
			mutate: func(f map[string]string) {
				f["stop_times.txt"] = "trip_id,arrival_time,departure_time,stop_id,stop_sequence\nT1,08:00:00,08:00:00,,1\n"
			},
			wantErr: "empty stop_id",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := validFiles()
			c.mutate(files)
			_, err := ParseZip(writeZip(t, files))
			if err == nil {
				t.Fatalf("ParseZip succeeded, want error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error = %v, want it to contain %q", err, c.wantErr)
			}
		})
	}

	// The unmutated baseline must still parse.
	if _, err := ParseZip(writeZip(t, validFiles())); err != nil {
		t.Fatalf("baseline valid feed failed: %v", err)
	}
}
