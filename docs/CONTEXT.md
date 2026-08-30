# Project context

Status, decisions, and next steps for raptor-transit. Update this doc at the
end of each phase so anyone (or any AI assistant) picking up the project can
get current without spelunking through git history.

_Last updated: 2026-08-30 (end of Phase 3)._

## Where the project stands

| Phase | Scope | Status |
|---|---|---|
| 1 | GTFS parser, ingest command, project scaffold | **Done** (reviewed; two defects found and fixed) |
| 2 | Compact timetable, walking transfers, RAPTOR engine, `cmd/route` CLI, validation harness | **Done** |
| 3 | HTTP API (`internal/api`, `cmd/server`), stop-name search, `internal/calendar` | **Done** |
| 3.5 | Verify the 14 remaining ground-truth cases | Not started — **do before Phase 4** |
| 4 | GTFS-Realtime ingestion (`internal/realtime`) | Not started |
| 5 | GCP pipeline: RT polling, Pub/Sub, BigQuery delay history | Not started |
| 6 | PyTorch reliability model, journey reranking | Not started (needs months of Phase 5 data) |
| 7 | Calendar-aware "when should I leave" (arrive-by queries) | Not started |

Verified numbers as of Phase 2 (full KCM feed, SPR26 service):

- Feed: 6,446 stops / 157 GTFS routes / 62,614 trips / 2,167,134 stop times
- 532 stop patterns; timetable gob 17.8 MB; ingest ~6 s end-to-end
- 14,342 walking footpaths (200 m, 1.2 m/s)
- Query latency p50 ≈ 14 ms, p95 ≈ 28 ms (300 random queries); CLI ~55 MB RSS
- Ground truth: 6/6 verified cases pass; **14 of 20 cases still unverified**
  (all the transfer cases — see TODO 1)
- API: `/route`, `/stops`, `/healthz`; startup loads timetable + footpaths once

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
6. **Wire types are separate from engine types.** `internal/api` defines its
   own JSON structs rather than tagging `raptor.Journey`. The engine's
   seconds-since-midnight int32s are right for the algorithm and hostile to a
   client; the translation layer lets each side change without dragging the
   other. Times go out as `HH:MM:SS` keeping hours past 24 intact
   (`"25:30:00"` + `next_day: true`), never wrapped.
7. **Empty results are 200, not 404.** A well-formed query with no reachable
   journey returns `"journeys": []`. 400 is for bad input (including an
   ambiguous stop name, which returns its candidates); 404 only for a stop
   that matches nothing. Encode as `[]`, never `null`.
8. **Ambiguous stop names are never resolved by guessing.** Several KCM stops
   share a name; silently planning from the wrong one is worse than an error.
   Both API and CLI list the candidates and ask.
9. **Ground truth is sacred.** Expected journey results come from the raw feed
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

Full phase-by-phase plan lives in the project doc `raptor-transit-roadmap.md`;
this is the short form.

1. **[HUMAN] Verify the 14 remaining ground-truth cases** in
   `testdata/ground_truth.json` — every transfer case is among them, so
   transfer logic currently has no oracle. Look each up in Google Maps transit
   or OneBusAway, fill in `expected_routes` + `expected_arrival`, set
   `verified: true`; `TestGroundTruth` picks them up automatically. Do this
   **before** Phase 4: realtime and (later) ML reranking both change which
   transfers the engine picks, and without the oracle you won't be able to
   tell a regression from a pre-existing bug.
2. **Phase 4 — GTFS-Realtime**: poll KCM's TripUpdates feed, apply delays as
   a swappable overlay over the immutable timetable (never mutate it — that's
   what makes the engine shareable), expose `?realtime=true`. Note this is
   where the stdlib-only rule dies: GTFS-RT is protobuf and there is no
   stdlib path. Record that decision here when it happens.
3. **Phase 5 — GCP**: Cloud Run poller -> Pub/Sub -> BigQuery delay history.
   Mostly about *starting the clock*, since Phase 6 needs months of data.
   Also automate feed refresh: the current feed window ends 2027-03-26.
4. **Phase 6 — PyTorch**: on-time probability model, served out-of-process
   and called from Go, reranking the RAPTOR Pareto set. Keep a per-(route,
   hour) historical-median baseline in the repo and report honestly if the
   net doesn't beat it. Split by time, not randomly.
5. **Phase 7 — calendar awareness**: arrive-by queries (binary search over
   departure times first; reverse RAPTOR only if that's too slow), calendar
   event ingestion, address -> nearest-stop geocoding reusing the footpath
   grid's spatial bucketing.
6. **Smaller ideas**: range queries (rRAPTOR), fare/accessibility surfacing
   (wheelchair fields are already parsed), map output via shapes.txt.

## Development notes

- Go 1.23.4 at `~/.local/go` on the original dev box
  (`export PATH=$HOME/.local/go/bin:$PATH`).
- Real-feed tests need `data/google_transit.zip` (`make fetch`); they skip
  cleanly when it's absent, so CI without the feed still passes.
- The full `-race` suite takes ~4 minutes (real-feed round-trips dominate).
  `go test -short ./...` for the quick loop.
- `data/` is gitignored (feed + gobs are regenerable); `docs/FEED_NOTES.md`
  is the committed copy of the feed analysis.
- The API tests build their timetable from `internal/gtfs/testdata/mini.zip`,
  so they need no feed and run in milliseconds. Real-feed API testing is
  still manual: `make ingest && go run ./cmd/server`, then curl.
- `internal/gtfs/types.go` and `internal/timetable/timetable.go` are flagged
  by newer gofmt versions than the one they were written with. Left alone so
  the diff stays about Phase 3; worth a standalone formatting commit.
