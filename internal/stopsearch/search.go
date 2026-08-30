// Package stopsearch resolves human-typed stop names to timetable stop
// indices.
//
// Riders know "Westlake Station", not stop 1120. GTFS stop names are also
// inconsistently punctuated across a feed ("Pike St & 3rd Ave", "Pike St and
// 3rd Ave", "PIKE ST  &  3RD AVE"), so matching is done over a normalized
// form: lowercased, punctuation dropped, whitespace collapsed.
//
// Ranking is deliberately simple and explainable — exact, then prefix, then
// substring, ties broken by shorter name and then by stop_id for stable
// output. There is no fuzzy/edit-distance matching: on a 6,400-stop feed the
// cheap rules cover the realistic typing patterns, and a scoring function
// nobody can predict is worse than none. Revisit if real queries prove
// otherwise.
package stopsearch

import (
	"sort"
	"strings"
	"unicode"

	"raptor-transit/internal/timetable"
)

// Match is one search hit.
type Match struct {
	Index  int32  // timetable stop index
	StopID string // GTFS stop_id
	Name   string // display name, as it appears in the feed
	Score  int    // lower is better; see the rank constants
}

// Match quality, in preference order.
const (
	rankExact = iota
	rankPrefix
	rankSubstring
)

// Index is a normalized view over a timetable's stop names. It is built
// once at startup and is read-only thereafter, so it is safe for concurrent
// use by HTTP handlers.
type Index struct {
	tt *timetable.Timetable

	// norm[i] is the normalized name of stop index i, parallel to
	// tt.StopNames. Stops with an empty name keep an empty entry and never
	// match by name (they are still reachable by ID).
	norm []string
}

// New builds a search index over a timetable's stops.
func New(tt *timetable.Timetable) *Index {
	idx := &Index{tt: tt, norm: make([]string, len(tt.StopNames))}
	for i, name := range tt.StopNames {
		idx.norm[i] = Normalize(name)
	}
	return idx
}

// Normalize reduces a name to its comparable form: lowercase, punctuation
// replaced by spaces, runs of whitespace collapsed, ends trimmed.
//
// Punctuation becomes a space rather than being deleted so that "3rd/4th"
// tokenizes the way a reader expects instead of becoming "3rd4th".
func Normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := true // leading spaces are suppressed
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		default:
			if !space {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Search returns up to limit stops matching the query, best first. A blank
// query returns nothing rather than the whole feed.
func (idx *Index) Search(query string, limit int) []Match {
	q := Normalize(query)
	if q == "" || limit <= 0 {
		return nil
	}

	var out []Match
	for i, n := range idx.norm {
		if n == "" {
			continue
		}
		score := -1
		switch {
		case n == q:
			score = rankExact
		case strings.HasPrefix(n, q):
			score = rankPrefix
		case strings.Contains(n, q):
			score = rankSubstring
		}
		if score < 0 {
			continue
		}
		out = append(out, Match{
			Index:  int32(i),
			StopID: idx.tt.StopIDs[i],
			Name:   idx.tt.StopNames[i],
			Score:  score,
		})
	}

	// Stable ordering: quality, then shorter names (a query that is most of
	// the name is a better hit than one buried in a long one), then stop_id
	// so identical names never reorder between runs.
	sort.Slice(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if x.Score != y.Score {
			return x.Score < y.Score
		}
		if len(x.Name) != len(y.Name) {
			return len(x.Name) < len(y.Name)
		}
		return x.StopID < y.StopID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Resolve turns user input into a single stop_id.
//
// An exact stop_id always wins — IDs are unambiguous and some KCM stop names
// are shared by a dozen stops, so a rider who typed an ID means it. Otherwise
// the input is treated as a name: a unique best match resolves, and anything
// else is reported so the caller can ask the user to choose rather than
// silently picking one.
func (idx *Index) Resolve(input string) (stopID string, candidates []Match, err error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", nil, ErrEmpty
	}
	if _, ok := idx.tt.StopIdx(trimmed); ok {
		return trimmed, nil, nil
	}

	matches := idx.Search(trimmed, 10)
	if len(matches) == 0 {
		return "", nil, ErrNotFound
	}
	// A single best-tier match resolves outright; several equally good ones
	// are genuinely ambiguous.
	if len(matches) == 1 || matches[0].Score < matches[1].Score {
		return matches[0].StopID, matches, nil
	}
	return "", matches, ErrAmbiguous
}
