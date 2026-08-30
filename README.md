# raptor-transit

A transit trip planner for Seattle, written from scratch in Go. Ask it "how do
I get from the U District to downtown, leaving at 8:00 AM?" and it answers with
real King County Metro itineraries — which route to board, where, and when you
arrive:

```
$ ./bin/route -from 10911 -to 1120 -at 08:00:00 -date 20260805
Option 1 — arrive 08:32:00, 0 transfer(s)
  08:00:00  route 49     U District Station - Bay 3 -> Pine St & 4th Ave (arrive 08:32:00)
```

The routing engine implements **RAPTOR** (Round-bAsed Public Transit Optimized
Router, [Delling, Pajor & Werneck 2015](https://www.microsoft.com/en-us/research/publication/round-based-public-transit-routing/)),
the algorithm family used by real-world journey planners. Data comes from King
County Metro's public [GTFS](https://gtfs.org/) feed (~6,400 stops, ~62,600
trips, ~2.1M scheduled stop times).

## Tech stack

- **Go 1.22+ standard library only.** Zero third-party dependencies — CSV
  parsing via `encoding/csv`, persistence via `encoding/gob`, the zip feed via
  `archive/zip`, HTTP via `net/http`, logging via `log/slog`. This is
  deliberate: the project doubles as a Go learning exercise, so everything in
  the repo is plain, readable stdlib Go.
- **Data:** GTFS static feed from King County Metro (redistributed by Sound
  Transit). Not committed to the repo; `make fetch` downloads it.
- **Tooling:** Makefile, `go test -race`, `go vet`; multi-stage distroless
  Dockerfile for the HTTP server.

## How it works

The pipeline has three stages — parse, preprocess, query:

```
GTFS zip ──> internal/gtfs ──> internal/timetable ──> internal/raptor ──> itineraries
  (feed)      (parse CSV)      (compact structures)      (query engine)
                                        +
                              internal/transfers
                              (walking links between
                               nearby stops)
```

### 1. Parse (`internal/gtfs`)

Streams the GTFS CSV files out of the zip and into Go structs. Handles the
messy realities of agency feeds: UTF-8 BOMs, CRLF line endings, optional
columns (header-driven, not positional), and times past midnight — GTFS
represents a 1:30 AM owl-service departure as `25:30:00` on the *previous*
service day, so all times are stored as plain seconds-since-midnight ints that
may exceed 86,400. Malformed rows (bad coordinates, empty IDs) fail loudly
rather than parsing to silent zero values.

### 2. Preprocess (`internal/timetable`, `internal/transfers`)

RAPTOR's core invariant is that every trip of a "route" visits the same stops
in the same order — but GTFS `route_id`s break that promise (a single KCM route
number mixes express, short-turn, and branch variants). `internal/timetable`
therefore regroups trips by their **exact ordered stop sequence**: on the
current feed, 157 GTFS routes explode into 532 true stop patterns. Everything
is flattened into dense int32 arrays (each stop time stored exactly once,
~17 MB serialized vs ~130 MB for the raw parse) with byte-deterministic gob
output.

KCM publishes no `transfers.txt`, so `internal/transfers` synthesizes walking
links: every pair of stops within 200 m (haversine) gets a footpath at 1.2 m/s
walking speed. A lat/lon grid keeps generation near-linear instead of O(n²)
over all ~6.4k stops; the grid's output is unit-tested to match a brute-force
reference exactly. Current feed: 14,342 footpaths.

### 3. Query (`internal/raptor`)

Classic RAPTOR rounds: round *k* computes the earliest arrival at every stop
using at most *k* buses. Each round scans only the patterns that serve stops
improved in the previous round, then relaxes walking footpaths, with local and
target pruning. Service calendars (`calendar.txt` + `calendar_dates.txt`
exceptions) filter which trips run on the query date, and the engine also
scans the *previous* service day so owl trips timetabled past 24:00:00 are
catchable after midnight. The result is a Pareto set — for each number of
transfers, the best achievable arrival — with full journey reconstruction via
parent pointers.

Measured on the full KCM feed (300 random queries): **p50 ≈ 14 ms, p95 ≈ 28 ms**
per query; the query CLI peaks at ~55 MB RSS end-to-end.

### 4. Serve (`internal/api`, `internal/stopsearch`, `cmd/server`)

`cmd/server` loads the timetable and generates footpaths once at startup —
the load and grid build together take far longer than the ~14 ms query, so
doing them per request would dominate the response. `raptor.Engine` allocates
its own working state per call and shares nothing mutable, so one instance
serves every request without locking.

Riders know "Westlake Station", not stop 1120, so `internal/stopsearch`
matches over a normalized name form (lowercased, punctuation dropped,
whitespace collapsed) and ranks exact > prefix > substring. Both the API and
the CLI accept a stop name or a stop_id in either position. An ambiguous name
is reported with its candidates rather than resolved by guessing — quietly
planning from the wrong "Station" is worse than an error.

```
$ curl -s 'localhost:8080/route?from=U+District+Station&to=Westlake&at=08:00:00&date=20260901'
{
  "origin":      {"id": "10911", "name": "U District Station - Bay 3"},
  "destination": {"id": "1120",  "name": "Pine St & 4th Ave"},
  "date": "20260901", "depart_after": "08:00:00",
  "journeys": [{
    "departure": "08:00:00", "arrival": "08:32:00",
    "duration_seconds": 1920, "transfers": 0,
    "legs": [{"mode": "transit", "route": "49", "trip_id": "...",
              "from": {...}, "to": {...},
              "departure": "08:00:00", "arrival": "08:32:00"}]
  }]
}
```

| Endpoint | Purpose |
|---|---|
| `GET /route?from=&to=&at=&date=` | Journeys. `from`/`to` take a stop_id or a name; `at` is `HH:MM:SS`, `date` is `YYYYMMDD`. |
| `GET /stops?q=&limit=` | Stop-name search, for autocomplete. |
| `GET /healthz` | Liveness. Only 200 once the timetable is loaded. |

Status codes carry a deliberate distinction: **400** for malformed or missing
parameters and ambiguous stop names, **404** for a stop that matches nothing,
and **200 with `"journeys": []`** when the query is well formed but nothing is
reachable. "No bus goes there" is an answer, not an error. When the empty
result is caused by a date outside the feed's service window — easy to hit,
since the current feed ends 2027-03-26 — the response says so in `notes`.

Wire times are `HH:MM:SS` strings that keep hours past 24 intact
(`"25:30:00"` with `"next_day": true`) rather than wrapping to `01:30:00` and
appearing to arrive before departure. Raw seconds are included alongside for
clients doing arithmetic.

## Repository layout

| Path | What it is |
|---|---|
| `cmd/ingest` | Parses the feed zip → `data/gtfs.gob` + `data/timetable.gob` |
| `cmd/route` | Terminal trip-planning queries (see example above) |
| `cmd/server` | HTTP API server (`/route`, `/stops`, `/healthz`) |
| `internal/gtfs` | GTFS zip/CSV parser and raw feed model |
| `internal/timetable` | Compact stop-pattern timetable the engine scans |
| `internal/transfers` | Walking-footpath generation from stop coordinates |
| `internal/raptor` | The RAPTOR query engine |
| `internal/api` | HTTP handlers and the JSON wire format |
| `internal/calendar` | Which GTFS services run on a given date |
| `internal/stopsearch` | Stop-name lookup ("Westlake Station" -> stop index) |
| `testdata/ground_truth.json` | Known-good journeys used as an oracle for engine correctness |
| `docs/FEED_NOTES.md` | Detailed analysis of the live KCM feed's quirks |
| `docs/CONTEXT.md` | Project status, decisions, and next steps |

## Getting started

Requires Go 1.22+ on PATH. (On the original dev machine it's installed at
`~/.local/go`: `export PATH=$HOME/.local/go/bin:$PATH`.)

```sh
make fetch    # download the KCM GTFS feed (~17.5 MB) into data/
make ingest   # parse it and write data/*.gob (~6 s)
make build    # compile everything into bin/
make test     # full test suite with the race detector (needs the feed for
              # real-data tests; they skip gracefully without it)

# Plan a trip from the terminal. -from/-to take a stop name or a stop_id
# (KCM stop numbers are the ones printed on the bus stop sign).
./bin/route -from 10911 -to 1120 -at 08:00:00 -date 20260805
./bin/route -from "U District Station" -to Westlake -at 08:00:00 -date 20260805

# Or serve the same thing over HTTP.
./bin/server                       # listens on :8080
curl -s 'localhost:8080/stops?q=Westlake'
curl -s 'localhost:8080/route?from=10911&to=1120&at=08:00:00&date=20260805'
```

## Testing strategy

- **Fixture tests** against a tiny hand-written GTFS feed
  (`internal/gtfs/testdata/mini/`) with documented expected results —
  including an overnight trip and a calendar-exception-only service.
- **Real-feed tests** (skipped when `data/` is empty): parse the full feed,
  assert pattern invariants over all 532 patterns, gob round-trip equality.
- **Ground truth** (`testdata/ground_truth.json`): journeys with known correct
  answers. Direct-trip cases were extracted from the raw schedule
  independently of the parser; transfer cases are human-verified against
  Google Maps/OneBusAway. The engine must match arrivals within tolerance.
- **Property checks**: the footpath grid is asserted equal to an O(n²)
  brute-force reference; builds are asserted byte-deterministic.
- **API tests** drive the real mux through `httptest` against the fixture
  feed — status-code semantics, the overnight-clock encoding, and
  `journeys: []` rather than `null`. A concurrency test fires 40 simultaneous
  requests at the shared engine, which is the assertion that lock-free
  sharing is actually safe under `-race`.
