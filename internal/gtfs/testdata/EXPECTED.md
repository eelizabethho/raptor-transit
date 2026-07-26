# Expected parse results for the `mini/` GTFS fixture

Hand-written, internally consistent GTFS feed. Same content exists in two forms:

- `testdata/mini/*.txt` — plain directory of files
- `testdata/mini.zip` — identical files at the zip root (no subdirectory)

A parser must produce identical results from both.

## Encoding quirks (deliberate — the parser must handle these)

| File | Quirk |
|---|---|
| `routes.txt` | Starts with a **UTF-8 BOM** (`EF BB BF`). Header must still parse as `route_id,...`, not `﻿route_id`. |
| `stops.txt` | Uses **CRLF** (`\r\n`) line endings. All other files use LF. |

## Entity counts

| Entity | Count | IDs |
|---|---|---|
| stops | 5 | `S1`, `S2`, `S3`, `S4`, `S5` |
| routes | 2 | `R1`, `R2` (both `route_type=3`, bus) |
| trips | 3 | `R1_T1` (route R1, WKDY), `R1_T2` (route R1, WKDY), `R2_OWL1` (route R2, SPECIAL) |
| stop_times | 11 | 5 rows for `R1_T1`, 3 for `R1_T2`, 3 for `R2_OWL1` |
| calendar rows | 1 | `WKDY` |
| calendar_dates rows | 4 | 3× add for `SPECIAL`, 1× remove for `WKDY` |

Distinct service_ids referenced by trips: `WKDY`, `SPECIAL` (2 total).

## Overnight trip (times past midnight)

`R2_OWL1` is the overnight trip. GTFS allows times > 24:00:00; they mean
"seconds since noon-minus-12h of the **service day**" (effectively next
calendar day). Expected seconds-since-midnight integers:

| trip_id | stop_id | stop_sequence | arrival_time | arrival (sec) | departure_time | departure (sec) |
|---|---|---|---|---|---|---|
| R2_OWL1 | S1 | 1 | 25:30:00 | **91800** | 25:30:00 | **91800** |
| R2_OWL1 | S2 | 2 | 25:38:00 | 92280 | 25:38:30 | 92310 |
| R2_OWL1 | S4 | 3 | 25:45:00 | **92700** | 25:45:00 | **92700** |

All `R1_T1` / `R1_T2` times are normal daytime values (e.g. `08:00:00` = 28800).

## Services: calendar vs calendar_dates

- **From `calendar.txt`:** `WKDY` only. Mon–Fri = 1, Sat/Sun = 0,
  valid 2026-01-01 through 2026-12-31.
- **From `calendar_dates.txt` only (no calendar.txt row):** `SPECIAL`.
  Added (`exception_type=1`) on exactly three dates:
  `20260704`, `20261225`, `20261231`.
- **Removal exception:** `WKDY` removed (`exception_type=2`) on `20261225`
  (Christmas holiday).

## Active services on concrete dates

| Date | Day of week | Active services | Why |
|---|---|---|---|
| 2026-03-10 | Tuesday | `WKDY` | Normal weekday; no exceptions. |
| 2026-07-04 | Saturday | `SPECIAL` | WKDY inactive (saturday=0); SPECIAL added via calendar_dates. |
| 2026-12-25 | Friday | `SPECIAL` | WKDY would be active (friday=1) but is removed by exception_type=2; SPECIAL added. |
| 2026-12-31 | Thursday | `WKDY`, `SPECIAL` | WKDY active as a normal weekday; SPECIAL also added — both active same day. |
| 2026-03-15 | Sunday | (none) | WKDY inactive (sunday=0); no SPECIAL exception. |

Consequences for trips:

- On 2026-03-10: trips `R1_T1`, `R1_T2` run; `R2_OWL1` does not.
- On 2026-12-25: only `R2_OWL1` runs (departing 25:30:00, i.e. 01:30 on 2026-12-26 wall-clock).
- On 2026-12-31: all three trips run.

## Cross-reference guarantees (verified)

- Every `trip_id` in `stop_times.txt` exists in `trips.txt`.
- Every `stop_id` in `stop_times.txt` exists in `stops.txt`.
- Every `route_id` in `trips.txt` exists in `routes.txt`.
- Every `service_id` in `trips.txt` exists in `calendar.txt` or `calendar_dates.txt`.
- `stop_sequence` is strictly increasing within each trip; arrival <= departure at each stop; times non-decreasing along each trip.
