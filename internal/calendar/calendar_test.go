package calendar

import (
	"path/filepath"
	"testing"

	"raptor-transit/internal/gtfs"
	"raptor-transit/internal/timetable"
)

// Fixture calendars (internal/gtfs/testdata/EXPECTED.md):
//   WKDY:    Mon-Fri, 20260101..20261231, minus 20261225.
//   SPECIAL: no calendar.txt row; added on 20260704, 20261225, 20261231.

func fixtureServices(t *testing.T) []timetable.Service {
	t.Helper()
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatal(err)
	}
	return timetable.Build(feed).Services
}

func TestActiveServices(t *testing.T) {
	services := fixtureServices(t)

	cases := []struct {
		name string
		date string
		want []string
	}{
		{"weekday runs WKDY", "20260310", []string{"WKDY"}}, // Tuesday
		{"weekend runs nothing", "20260314", nil},           // Saturday
		{"exception-only service", "20260704", []string{"SPECIAL"}},
		// Christmas removes WKDY and adds SPECIAL: an exception must
		// override the weekly pattern in both directions on one date.
		{"exception overrides weekday bitmap", "20261225", []string{"SPECIAL"}}, // a Friday
		{"before feed window", "20251231", nil},
		{"after feed window", "20270101", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			day, err := ParseDate(c.date)
			if err != nil {
				t.Fatal(err)
			}
			got := ActiveServices(services, day)
			if len(got) != len(c.want) {
				t.Fatalf("ActiveServices(%s) = %v, want %v", c.date, keys(got), c.want)
			}
			for _, id := range c.want {
				if !got[id] {
					t.Errorf("ActiveServices(%s) missing %s (got %v)", c.date, id, keys(got))
				}
			}
		})
	}
}

func TestInFeedWindow(t *testing.T) {
	services := fixtureServices(t)
	cases := []struct {
		date string
		want bool
	}{
		{"20260310", true},  // weekday, WKDY
		{"20260704", true},  // Saturday, but SPECIAL is added
		{"20260314", false}, // ordinary Saturday
		{"20300101", false}, // long past the feed window
	}
	for _, c := range cases {
		day, err := ParseDate(c.date)
		if err != nil {
			t.Fatal(err)
		}
		if got := InFeedWindow(services, day); got != c.want {
			t.Errorf("InFeedWindow(%s) = %v, want %v", c.date, got, c.want)
		}
	}
}

func TestParseDateRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"", "March", "2026-03-10", "20261301", "202603100"} {
		if _, err := ParseDate(bad); err == nil {
			t.Errorf("ParseDate(%q) succeeded, want error", bad)
		}
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
