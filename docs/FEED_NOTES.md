# King County Metro GTFS Feed Notes

Acquired 2026-07-26 by automated download. Feed lives in `data/gtfs/` (zip kept at
`data/google_transit.zip`). `data/` is gitignored; this file is mirrored to
`docs/FEED_NOTES.md`.

## 1. Source and freshness

- URL used: `https://www.soundtransit.org/GTFS-KCM/google_transit.zip`
- That URL returned **HTTP 301** to `https://metro.kingcounty.gov/GTFS/google_transit.zip`;
  `curl -L` followed the redirect and the final response was HTTP 200,
  `Content-Length: 17475566` (~17.5 MB), served by Microsoft-IIS/8.5.
- Zip `Last-Modified: Mon, 20 Jul 2026 15:48:22 GMT`.
- `feed_info.txt`:
  - `feed_publisher_name`: Metro Transit
  - `feed_lang`: EN
  - `feed_start_date`: **20260720**, `feed_end_date`: **20270326**
  - `feed_version`: `SPR26-160.8-Combined-EasyLoop`
  - contact: mtdtechopscore@kingcounty.gov

Agencies in the feed (4): King County Metro (`1`), City of Seattle streetcars (`23`),
Sound Transit (`40`, 9 express bus routes), Solid Ground EZ Loop (`6289`).
Timezone for all: `America/Los_Angeles`.

## 2. Files present and row counts (excluding header)

| File | Rows |
|---|---:|
| agency.txt | 4 |
| calendar.txt | 21 |
| calendar_dates.txt | 81 |
| fare_attributes.txt | 11 |
| fare_leg_rules.txt | 5 |
| fare_media.txt | 5 |
| fare_products.txt | 32 |
| fare_rules.txt | 58 |
| fare_transfer_rules.txt | 1 |
| feed_info.txt | 1 |
| networks.txt | 4 |
| rider_categories.txt | 3 |
| route_networks.txt | 6 |
| routes.txt | 157 |
| shapes.txt | 230,463 |
| stops.txt | 6,446 |
| stop_times.txt | **2,167,134** |
| trips.txt | 62,614 |

**`transfers.txt` does NOT exist in this feed.** See section 7.

## 3. Core file columns: present vs actually populated

Counts below are non-empty values per column over ALL rows (not a sample).
Empty means `""` or blank after stripping surrounding quotes.

### stops.txt (6,446 rows, 13 columns)

| Column | Non-empty | Notes |
|---|---:|---|
| stop_id | 6,446 | all unique, no duplicates |
| stop_code | 6,446 | equals stop_id in sampled rows |
| stop_name | 6,446 | quoted strings |
| tts_stop_name | 6,446 | |
| stop_desc | **0** | always empty |
| stop_lat | 6,446 | bounds 47.1891–47.8656 |
| stop_lon | 6,446 | bounds -122.507 – -121.71 (all plausible King County) |
| zone_id | 6,446 | |
| stop_url | **0** | always empty |
| location_type | 6,446 | **all `0`** — no stations/entrances, no parent-station hierarchy |
| parent_station | **0** | always empty (consistent with location_type=0 everywhere) |
| stop_timezone | 6,446 | all America/Los_Angeles |
| wheelchair_boarding | 6,446 | 0: 83, 1: 6,159, 2: 204 |

### routes.txt (157 rows, 9 columns)

| Column | Non-empty | Notes |
|---|---:|---|
| route_id | 157 | |
| agency_id | 157 | 146 KCM, 2 Seattle streetcar, 9 Sound Transit |
| route_short_name | 157 | this is the rider-facing name |
| route_long_name | **0** | always empty — use route_short_name + route_desc |
| route_desc | 157 | e.g. "Kinnear - Downtown Seattle" |
| route_type | 157 | 3 (bus): 153, 0 (streetcar/tram): 2, 4 (ferry/water taxi): 2 |
| route_url | 157 | |
| route_color | 134 | 23 routes have no color |
| route_text_color | 156 | 1 route missing (Easy Loop 7994) |

### trips.txt (62,614 rows, 12 columns)

| Column | Non-empty | Notes |
|---|---:|---|
| route_id | 62,614 | all resolve to routes.txt |
| service_id | 62,614 | 20 distinct, all resolve to calendar.txt |
| trip_id | 62,614 | no duplicates |
| trip_headsign | 62,602 | **12 empty** — all on route 7994 "Easy Loop" |
| trip_short_name | **0** | always empty |
| direction_id | 62,602 | **12 empty** — same 12 Easy Loop trips (a loop route) |
| block_id | 62,614 | |
| shape_id | 62,614 | |
| peak_flag | **0** | KCM extension column, always empty |
| fare_id | 62,602 | 12 empty (Easy Loop again) |
| wheelchair_accessible | 62,614 | |
| bikes_allowed | 62,614 | |

### stop_times.txt (2,167,134 rows, 10 columns)

| Column | Non-empty | Notes |
|---|---:|---|
| trip_id | 2,167,134 | |
| arrival_time | 2,167,134 | **never empty** — no interpolation needed |
| departure_time | 2,167,134 | never empty; `departure < arrival` never occurs |
| stop_id | 2,167,134 | all 6,446 stops referenced; zero dangling refs; zero unused stops |
| stop_sequence | 2,167,134 | always starts at 1 and is contiguous for every trip (max−min+1 == count for all 62,614 trips) |
| stop_headsign | 489,994 | ~23% populated |
| pickup_type | 2,167,038 | 96 rows empty (the 12 Easy Loop trips); all others `0` |
| drop_off_type | 2,167,038 | same 96 rows empty; all others `0` |
| shape_dist_traveled | 2,167,134 | |
| timepoint | 2,167,134 | 1: 416,621, 0: 1,750,513 (times at non-timepoints are estimated but present) |

All times match `HH:MM:SS` with a 2-digit (zero-padded) hour — no `H:MM:SS` variants.

### calendar.txt (21 rows) — NOT empty

**calendar.txt is fully populated; KCM does NOT use the calendar_dates-only pattern
in this feed.** All 10 columns non-empty on all 21 rows. All 20 service_ids used by
trips exist in calendar.txt; calendar_dates.txt only modifies them.

- One calendar row (`4557`) has all-zero weekdays — activated purely via
  calendar_dates exceptions (a hybrid pattern; still requires reading calendar.txt
  for its date range).
- One calendar service (`150490`) is defined but referenced by **zero trips**
  (harmless; skip it).
- Trip volume is dominated by 6 service_ids (weekday/Sat/Sun for the current and
  next service change): 155767 (12,620 trips), 29380 (11,229), 30098 (9,272),
  29348 (8,936), 6614 (8,901), 6574 (8,431). The rest are small holiday/special
  patterns (2–936 trips).

### calendar_dates.txt (81 rows)

All 3 columns (`service_id,date,exception_type`) fully populated. Covers only 5
service_ids (150490, 18868, 10512, 4557, plus overlaps) with a mix of type 1 (added)
and type 2 (removed). All its service_ids also appear in calendar.txt.

### transfers.txt

Absent. See section 7.

## 4. Times past midnight (> 24:00:00)

- **arrival_time >= 24:00:00: 76,056 rows; departure_time >= 24:00:00: 76,057 rows**
  (~3.5% of stop_times).
- **Max time seen: 29:30:00** (both arrival and departure) — so a router must
  handle at least +5.5h past midnight; owl service bleeds well into the next
  service day.
- Distinct trips containing a >= 24h time: **2,782**.
- Example trip_ids with >24h times: `350247350`, `350247351`, `466967530`,
  `466967531`, `473804250`, `473804251`; the 29:30:00 max occurs on trips
  `849295841`, `849295271`, `848943491`.
- Implication for RAPTOR: parse times as seconds-since-noon-minus-12h (i.e. plain
  seconds since service-day midnight, allowing >86,400), and when routing near/after
  midnight, consider trips from both the query date and the previous service date.

## 5. CSV parsing notes

- Files use a mix of quoted and unquoted fields (e.g. `stop_name`, `route_short_name`,
  `trip_headsign` are quoted; ids and numbers are not).
- **No embedded commas anywhere in the core files**: naive comma-splitting yields a
  constant field count on every row of stops (13), routes (9), trips (12),
  stop_times (10). A simple `strings.Split` parser works, but strip surrounding
  quotes. (Using `encoding/csv` is still the safer choice for future feed updates.)
- No BOM issues observed; headers are plain ASCII.

## 6. Referential integrity (all clean)

- 0 duplicate stop_ids; 0 duplicate trip_ids.
- 0 stop_ids in stop_times missing from stops.txt; 0 stops unused by stop_times.
- 0 route_ids in trips missing from routes.txt.
- 0 service_ids in trips missing from calendar.txt.
- stop_sequence starts at 1 and is contiguous for every one of 62,614 trips.
- departure_time >= arrival_time on every row.

## 7. transfers.txt is missing — footpaths must be generated

There is no transfers.txt at all (0 rows vs 6,446 stops). RAPTOR requires a
footpath/transfer set, so we must synthesize it, e.g. connect stop pairs within a
walking radius (typical: 100–500 m crow-flies via haversine on stop_lat/stop_lon,
with an assumed walking speed) and make the resulting graph transitively closed
(or handle multi-hop walks in the algorithm). Since `location_type` is always 0 and
`parent_station` is always empty, there is also no station grouping to exploit —
every transfer is a plain stop-to-stop walk.

## 8. Other oddities relevant to a RAPTOR router

1. **No parent stations / station hierarchy** — all 6,446 stops are simple stops
   (location_type=0, parent_station empty). Simplifies the stop model.
2. **route_long_name always empty** — display names must come from
   route_short_name (+ route_desc).
3. **Easy Loop (route_id 7994, agency 6289)** is the consistently degenerate case:
   its 12 trips have empty trip_headsign, direction_id, fare_id, and its 96
   stop_times rows have empty pickup_type/drop_off_type. Treat empty
   pickup/drop_off as 0 (regular) and empty direction_id as "unknown".
4. **Non-bus route_types exist**: 2 streetcar (0) and 2 water taxi (4) routes in
   addition to 153 buses. Don't assume route_type == 3.
5. **timepoint=0 on ~81% of rows** — those times are interpolated by KCM but are
   always present, so the router can use them directly.
6. **Feed validity window** is 2026-07-20 through 2027-03-26; service pattern
   changes on 2026-08-29/31 (visible in calendar start/end dates). Queries outside
   the window will find no service.
7. **peak_flag and trip_short_name** columns exist but are always empty; ignore.
8. `arrival_time != departure_time` on only 492 rows (dwell time is almost never
   modeled); a router may treat arrival==departure with negligible error, but the
   492 rows are honored for free if both fields are read.
9. shapes.txt (230k points) is present and fully referenced by trips (shape_id
   always set) — useful for map display, not needed for routing.
