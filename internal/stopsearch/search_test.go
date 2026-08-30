package stopsearch

import (
	"errors"
	"path/filepath"
	"testing"

	"raptor-transit/internal/gtfs"
	"raptor-transit/internal/timetable"
)

// Fixture stops (internal/gtfs/testdata/mini/stops.txt):
//   S1 Pike St & 3rd Ave      S2 Westlake Station
//   S3 Capitol Hill Station   S4 University District Station
//   S5 Northgate Station

func fixtureIndex(t *testing.T) *Index {
	t.Helper()
	feed, err := gtfs.ParseZip(filepath.Join("..", "gtfs", "testdata", "mini.zip"))
	if err != nil {
		t.Fatal(err)
	}
	return New(timetable.Build(feed))
}

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Westlake Station", "westlake station"},
		{"Pike St & 3rd Ave", "pike st 3rd ave"},
		{"  PIKE ST  &  3RD AVE  ", "pike st 3rd ave"},
		{"3rd/4th", "3rd 4th"},
		{"NE 45th St & 15th Ave NE", "ne 45th st 15th ave ne"},
		{"", ""},
		{"   ", ""},
		{"&&&", ""},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSearch(t *testing.T) {
	idx := fixtureIndex(t)

	cases := []struct {
		name  string
		query string
		limit int
		want  []string // expected stop_ids, in order
	}{
		{"exact name", "Westlake Station", 10, []string{"S2"}},
		{"prefix", "West", 10, []string{"S2"}},
		{"case insensitive", "wEsTlAkE", 10, []string{"S2"}},
		{"punctuation ignored", "pike st and 3rd", 10, nil}, // "and" is a real token, not punctuation
		{"ampersand normalized", "Pike St 3rd Ave", 10, []string{"S1"}},
		{"substring", "District", 10, []string{"S4"}},
		// "Station" is a suffix of three stops; shortest name wins, then
		// stop_id. Northgate(17) < Westlake(17, but S2<S5) < Capitol Hill(21).
		{"multiple substring hits ordered", "Station", 10, []string{"S2", "S5", "S3", "S4"}},
		{"limit respected", "Station", 2, []string{"S2", "S5"}},
		{"no match", "Ballard", 10, nil},
		{"blank query returns nothing", "   ", 10, nil},
		{"zero limit returns nothing", "Station", 0, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := idx.Search(c.query, c.limit)
			if len(got) != len(c.want) {
				t.Fatalf("Search(%q) returned %d results %v, want %d %v",
					c.query, len(got), ids(got), len(c.want), c.want)
			}
			for i, id := range c.want {
				if got[i].StopID != id {
					t.Errorf("Search(%q)[%d] = %s (%s), want %s",
						c.query, i, got[i].StopID, got[i].Name, id)
				}
			}
		})
	}
}

// Prefix matches must outrank substring matches even when the substring hit
// has a shorter name — quality dominates length in the ordering.
func TestSearchPrefixBeatsSubstring(t *testing.T) {
	idx := fixtureIndex(t)
	got := idx.Search("capitol", 10)
	if len(got) != 1 || got[0].StopID != "S3" {
		t.Fatalf("got %v, want [S3]", ids(got))
	}
	if got[0].Score != rankPrefix {
		t.Errorf("score = %d, want rankPrefix (%d)", got[0].Score, rankPrefix)
	}
}

func TestResolve(t *testing.T) {
	idx := fixtureIndex(t)

	t.Run("stop_id wins outright", func(t *testing.T) {
		id, _, err := idx.Resolve("S2")
		if err != nil || id != "S2" {
			t.Fatalf("got (%q, %v), want (S2, nil)", id, err)
		}
	})

	t.Run("unique name resolves", func(t *testing.T) {
		id, _, err := idx.Resolve("Capitol Hill")
		if err != nil || id != "S3" {
			t.Fatalf("got (%q, %v), want (S3, nil)", id, err)
		}
	})

	t.Run("exact name beats other matches", func(t *testing.T) {
		id, _, err := idx.Resolve("Westlake Station")
		if err != nil || id != "S2" {
			t.Fatalf("got (%q, %v), want (S2, nil)", id, err)
		}
	})

	t.Run("ambiguous name reports candidates", func(t *testing.T) {
		// Four stops end in "Station"; all match at substring rank, so no
		// single one may be chosen silently.
		id, candidates, err := idx.Resolve("Station")
		if !errors.Is(err, ErrAmbiguous) {
			t.Fatalf("got (%q, %v), want ErrAmbiguous", id, err)
		}
		if len(candidates) < 2 {
			t.Errorf("got %d candidates, want at least 2", len(candidates))
		}
	})

	t.Run("no match", func(t *testing.T) {
		if _, _, err := idx.Resolve("Ballard"); !errors.Is(err, ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		if _, _, err := idx.Resolve("  "); !errors.Is(err, ErrEmpty) {
			t.Errorf("got %v, want ErrEmpty", err)
		}
	})
}

func ids(ms []Match) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.StopID
	}
	return out
}
