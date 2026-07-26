# Project context

Status, decisions, and next steps for raptor-transit. Update this doc at the
end of each phase so anyone (or any AI assistant) picking up the project can
get current without spelunking through git history.

_Last updated: 2026-07-26 (end of Phase 2)._

## Where the project stands

| Phase | Scope | Status |
|---|---|---|
| 1 | GTFS parser, ingest command, project scaffold | **Done** (reviewed; two defects found and fixed) |
| 2 | Compact timetable, walking transfers, RAPTOR engine, `cmd/route` CLI, validation harness | **Done** |
| 3 | HTTP API (`internal/api`, `cmd/server`), stop-name search, nicer output | Not started |

Verified numbers as of Phase 2 (full KCM feed, SPR26 service):

- Feed: 6,446 stops / 157 GTFS routes / 62,614 trips / 2,167,134 stop times
- 532 stop patterns; timetable gob 17.8 MB; ingest ~6 s end-to-end
- 14,342 walking footpaths (200 m, 1.2 m/s)
- Query latency p50 ≈ 14 ms, p95 ≈ 28 ms (300 random queries); CLI ~55 MB RSS
- Ground truth: 6/6 verified cases pass; 6 cases still unverified (see TODO)

## Key design decisions (and why)

1. **stdlib only.** Learning project — every line should be readable without
   chasing library docs. Revisit only if a real need appears (e.g. an rtree
   for larger walking radii).
2. **Stop patterns, not GTFS route_ids.** RAPTOR assumes all trips of a route
   share one stop sequence; KCM route_ids don't (express/short-turn variants).
   Trips are regrouped by exact ordered stop sequence. Caught in Phase 1
   review — do not build anything new on `route_id` grouping.
3. **Times are seconds-since-midnight ints that may exceed 86,400.** GTFS
   timetables owl trips on the day they started (25:30:00 = 1:30 AM next
   day). The engine scans both the query date and the previous service date.
4. **No self-footpaths in transfers output.** The engine treats every stop as
   reachable from itself at zero cost; the footpath list only holds real
   walks. Footpaths are NOT transitively closed — RAPTOR applies them each
   round, so multi-hop walks emerge across rounds.
5. **Deterministic builds.** Gob randomizes map ordering, so persisted structs
   contain only sorted slices; lookup maps are unexported and rebuilt on load.
   Byte-identical rebuilds are tested.
6. **Ground truth is sacred.** Expected journey results come from the raw feed
   (awk, independent of our parser) or from humans checking Google Maps /
   OneBusAway. Never let the code under test generate its own expectations.

## Feed gotchas (full detail in FEED_NOTES.md)

- **No `transfers.txt`** — that's why `internal/transfers` exists.
- `calendar.txt` is fully populated (21 services); `calendar_dates.txt` only
  modifies them. One service (4557) is all-zero weekdays + exceptions only.
- Max time in feed: 29:30:00. ~76k stop_times rows exceed 24:00:00.
- `route_long_name` is always empty — display via `route_short_name`.
- No parent stations; every stop is a plain stop.
- Feed window: 2026-07-20 → 2027-03-26. Queries outside it correctly find
  nothing — refresh the feed (`make fetch && make ingest`) when it lapses.

## TODO / next steps

1. **[HUMAN] Verify the 6 remaining ground-truth cases** in
   `testdata/ground_truth.json` (transfer, late-night, Saturday journeys):
   look each up in Google Maps transit or OneBusAway, fill in
   `expected_routes` + `expected_arrival`, set `verified: true`. The harness
   (`TestGroundTruth`) picks them up automatically. ~30 minutes; this is the
   main guard on transfer-logic correctness.
2. **Phase 3 — HTTP API**: `GET /route?from=&to=&at=&date=` on `cmd/server`,
   JSON journeys, load timetable+footpaths once at startup (engine is
   concurrency-safe). `internal/api` and `internal/calendar` are empty stubs
   reserved for this.
3. **Stop-name search**: users know "Westlake Station", not stop 1120.
   Prefix/fuzzy match over `StopNames`, expose in CLI and API.
4. **Later ideas**: range queries (rRAPTOR), fare/accessibility surfacing
   (wheelchair fields are parsed), feed auto-refresh, map output via
   shapes.txt.

## Development notes

- Go 1.23.4 at `~/.local/go` on the original dev box
  (`export PATH=$HOME/.local/go/bin:$PATH`).
- Real-feed tests need `data/google_transit.zip` (`make fetch`); they skip
  cleanly when it's absent, so CI without the feed still passes.
- The full `-race` suite takes ~4 minutes (real-feed round-trips dominate).
  `go test -short ./...` for the quick loop.
- `data/` is gitignored (feed + gobs are regenerable); `docs/FEED_NOTES.md`
  is the committed copy of the feed analysis.
