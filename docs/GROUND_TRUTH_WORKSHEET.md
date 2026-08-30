# Ground-truth verification worksheet

**14 of the 20 cases in `testdata/ground_truth.json` are unverified, and every
transfer case is among them — so the RAPTOR engine's transfer logic currently has
no oracle.** This worksheet is the checklist for closing that gap (Phase 3.5 in
`docs/CONTEXT.md`).

## Read this first

`docs/CONTEXT.md`, decision 9 — *"Ground truth is sacred"*: expected results must
come from a source **independent of the code under test**. That means a human
reading Google Maps transit or OneBusAway, or the raw feed via `awk`. **Never run
`cmd/route`, the `/route` API, or any other part of this repo to produce these
numbers** — a test whose expectations came from the thing it tests proves nothing,
and it will happily lock in a bug as "correct" right before Phase 4 (realtime) and
Phase 6 (ML reranking) start changing which transfers the engine picks.

Times are local (America/Los_Angeles), `HH:MM:SS`. `tolerance_min` is 5 for every
case here, so the arrival time only has to be right to within 5 minutes — but
`expected_routes` must match exactly, in boarding order.

## What to fill in for each case

Once a lookup is done, edit that case's object in `testdata/ground_truth.json`:

| Field | Set it to |
|---|---|
| `expected_routes` | JSON array of `route_short_name` values in boarding order, e.g. `["49", "40"]`. Use the name the rider sees: `"E Line"`, `"D Line"`, `"G Line"`, `"C Line"` for RapidRide; `"1 Line"` / `"2 Line"` for Link. `route_long_name` is empty in this feed. |
| `expected_arrival` | Arrival time at the destination stop, `"HH:MM:SS"`. Keep hours past midnight unwrapped if the trip began on the previous service day (`"24:35:00"`, not `"00:35:00"`). |
| `verified` | `true` |
| `source` | Where the answer came from, specifically enough to re-check: `"human: Google Maps transit, checked 2026-09-01"` or `"human: OneBusAway stop 1_18120 schedule"`. |
| `transfers` | **Correct it if it is wrong.** For the 8 cases added 2026-07-27 this number is a route-overlap *guess*, not an observation. It is the count of transfers, so a 2-leg journey is `1`. |
| `description` | Drop the `TODO-HUMAN:` prefix and the speculation about which routes it "likely" uses; describe what the journey actually is. |

`TestGroundTruth` picks up `verified: true` cases automatically — no code change needed.

Rules of thumb while looking things up:

- **Match the exact stop, including the bay.** Several KCM stops share a name and
  the bay usually encodes direction. Use the OneBusAway link to confirm the stop is
  the one the ID names and that it is served in the direction you expect.
- **Query the given date and time, not "now".** The dates are chosen inside service
  29380 (weekdays through 2026-08-28) and the feed window 2026-07-20 .. 2027-03-26.
- **Take the first departure at or after the listed time**, and the journey Google
  Maps ranks first among those (earliest arrival). If two options tie on arrival,
  prefer the one with fewer transfers and note the alternative.
- **Bus-only unless a rail leg is genuinely the fastest option.** If the best
  journey uses Link or the Water Taxi, record it as-is and note that the engine may
  not model it — a mismatch there is information, not a failure to hide.
- **If Google Maps and OneBusAway disagree, trust the schedule** (OneBusAway shows
  the scheduled timetable; Google Maps may fold in walking or realtime).
- If a case turns out to have **no reasonable journey**, say so in `source` and
  leave `verified: false` rather than inventing a plausible answer.

## Cases

Google Maps `api=1` links carry origin and destination only — there is no
documented departure-time parameter for that URL form, so **set the date and time
by hand in the Maps UI** (Depart at) using the values in each table.

### 1. `transfer-capitolhill-ballard`

| | |
|---|---|
| **From** | E John St & Broadway  E - Bay 1 — stop `29270` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_29270)) |
| **To** | NW Market St & Ballard Ave NW — stop `18120` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_18120)) |
| **Depart** | 2026-08-05 (Wednesday) at **08:00** |
| **Expected transfers** | 1 (confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=E%20John%20St%20%26%20Broadway%20E%20-%20Bay%201%2C%20Seattle%2C%20WA&destination=NW%20Market%20St%20%26%20Ballard%20Ave%20NW%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ TODO-HUMAN: Capitol Hill (Broadway & E John) to Ballard (NW Market St & Ballard Ave NW), ~8am weekday. Likely 49/8 then 40 or D Line, 1 transfer.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 2. `transfer-westseattle-udistrict`

| | |
|---|---|
| **From** | SW Alaska St & California Ave SW - Bay 1 — stop `19862` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_19862)) |
| **To** | U District Station - Bay 3 — stop `10911` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_10911)) |
| **Depart** | 2026-08-05 (Wednesday) at **08:30** |
| **Expected transfers** | 2 (confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=SW%20Alaska%20St%20%26%20California%20Ave%20SW%20-%20Bay%201%2C%20Seattle%2C%20WA&destination=U%20District%20Station%20-%20Bay%203%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ TODO-HUMAN: West Seattle Junction to U District, ~8:30am weekday. Likely C Line then Link or 44/49-ish, 1-2 transfers.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 3. `transfer-fremont-columbiacity`

| | |
|---|---|
| **From** | Fremont Ave N & N 34th St — stop `26510` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_26510)) |
| **To** | Rainier Ave S & S Alaska St — stop `8310` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_8310)) |
| **Depart** | 2026-08-05 (Wednesday) at **12:00** |
| **Expected transfers** | 2 (confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=Fremont%20Ave%20N%20%26%20N%2034th%20St%2C%20Seattle%2C%20WA&destination=Rainier%20Ave%20S%20%26%20S%20Alaska%20St%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ TODO-HUMAN: Fremont (N 34th & Fremont Ave) to Columbia City, ~noon weekday. Probably 2 transfers (downtown + Link or 7/50).

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 4. `transfer-greenwood-seattlecenter`

| | |
|---|---|
| **From** | Greenwood Ave N & N 85th St — stop `6640` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_6640)) |
| **To** | Queen Anne Ave N & W Mercer St — stop `2672` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_2672)) |
| **Depart** | 2026-08-05 (Wednesday) at **18:00** |
| **Expected transfers** | 1 (confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=Greenwood%20Ave%20N%20%26%20N%2085th%20St%2C%20Seattle%2C%20WA&destination=Queen%20Anne%20Ave%20N%20%26%20W%20Mercer%20St%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ TODO-HUMAN: Greenwood (N 85th & Greenwood Ave) to Seattle Center, ~6pm weekday. Likely 5 then D Line or walk, 1 transfer.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 5. `transfer-latenight-udistrict-downtown`

| | |
|---|---|
| **From** | U District Station - Bay 3 — stop `10911` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_10911)) |
| **To** | Pine St & 4th Ave — stop `1120` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_1120)) |
| **Depart** | 2026-08-05 (Wednesday) at **23:30** |
| **Expected transfers** | 0 (confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=U%20District%20Station%20-%20Bay%203%2C%20Seattle%2C%20WA&destination=Pine%20St%20%26%204th%20Ave%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ TODO-HUMAN: late-night U District to downtown, ~11:30pm weekday. Tests sparse night service; may be direct via 49 owl or require E Line.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 6. `transfer-saturday-ballard-beacon`

| | |
|---|---|
| **From** | NW Market St & Ballard Ave NW — stop `18120` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_18120)) |
| **To** | Beacon Ave S & S Lander St — stop `3810` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_3810)) |
| **Depart** | 2026-08-08 (Saturday) at **10:00** |
| **Expected transfers** | 2 (confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=NW%20Market%20St%20%26%20Ballard%20Ave%20NW%2C%20Seattle%2C%20WA&destination=Beacon%20Ave%20S%20%26%20S%20Lander%20St%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ TODO-HUMAN: Saturday service (svc 6574/36712 patterns differ from weekday): Ballard to Beacon Hill, ~10am Saturday 2026-08-08.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 7. `early-seatac-downtown`

| | |
|---|---|
| **From** | Tukwila International Blvd Station - Bay 2 — stop `60922` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_60922)) |
| **To** | 3rd Ave & Pike St — stop `433` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_433)) |
| **Depart** | 2026-08-05 (Wednesday) at **05:30** |
| **Expected transfers** | 0 (guess — confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=Tukwila%20International%20Blvd%20Station%20-%20Bay%202%2C%20Seattle%2C%20WA&destination=3rd%20Ave%20%26%20Pike%20St%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ Likely direct (route 124 serves both stops). Early-morning airport-to-downtown commute at 5:30am.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 8. `transfer-fremont-capitolhill`

| | |
|---|---|
| **From** | Fremont Ave N & N 34th St — stop `26510` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_26510)) |
| **To** | E John St & Broadway  E - Bay 1 — stop `29270` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_29270)) |
| **Depart** | 2026-08-05 (Wednesday) at **09:00** |
| **Expected transfers** | 1 (guess — confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=Fremont%20Ave%20N%20%26%20N%2034th%20St%2C%20Seattle%2C%20WA&destination=E%20John%20St%20%26%20Broadway%20E%20-%20Bay%201%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ Likely 1 transfer: Fremont routes (31/32/40/62) don't reach Capitol Hill; expect transfer downtown or via 8.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 9. `transfer-greenlake-downtown`

| | |
|---|---|
| **From** | East Green Lake Dr N & Meridian Ave N — stop `35691` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_35691)) |
| **To** | 3rd Ave & Pike St — stop `575` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_575)) |
| **Depart** | 2026-08-05 (Wednesday) at **10:30** |
| **Expected transfers** | 1 (guess — confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=East%20Green%20Lake%20Dr%20N%20%26%20Meridian%20Ave%20N%2C%20Seattle%2C%20WA&destination=3rd%20Ave%20%26%20Pike%20St%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ Likely 1 transfer: Green Lake is route-45-only; expect 45 then Link or 62/70 south. Well-known Google Maps query.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 10. `transfer-queenanne-northgate`

| | |
|---|---|
| **From** | Queen Anne Ave N & W Mercer St — stop `2672` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_2672)) |
| **To** | Northgate Station - Bay 1 — stop `35317` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_35317)) |
| **Depart** | 2026-08-05 (Wednesday) at **08:15** |
| **Expected transfers** | 1 (guess — confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=Queen%20Anne%20Ave%20N%20%26%20W%20Mercer%20St%2C%20Seattle%2C%20WA&destination=Northgate%20Station%20-%20Bay%201%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ Likely 1 transfer: no shared route between Lower Queen Anne (1/2/8/13/32/D) and Northgate (40/61/67/75/345/348/365); expect D Line or 8 then 40/Link.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 11. `transfer-madisonvalley-udistrict`

| | |
|---|---|
| **From** | E Madison St & 23rd Ave E — stop `121` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_121)) |
| **To** | U District Station - Bay 3 — stop `10911` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_10911)) |
| **Depart** | 2026-08-05 (Wednesday) at **14:00** |
| **Expected transfers** | 1 (guess — confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=E%20Madison%20St%20%26%2023rd%20Ave%20E%2C%20Seattle%2C%20WA&destination=U%20District%20Station%20-%20Bay%203%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ Likely 1 transfer: G Line stop at 23rd & Madison, transfer to 48 north to U District. Tests new RapidRide G.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 12. `transfer-mountbaker-westseattle`

| | |
|---|---|
| **From** | Mount Baker Transit Center - Bay 2 — stop `8402` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_8402)) |
| **To** | SW Alaska St & California Ave SW - Bay 1 — stop `19862` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_19862)) |
| **Depart** | 2026-08-05 (Wednesday) at **11:00** |
| **Expected transfers** | 1 (guess — confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=Mount%20Baker%20Transit%20Center%20-%20Bay%202%2C%20Seattle%2C%20WA&destination=SW%20Alaska%20St%20%26%20California%20Ave%20SW%20-%20Bay%201%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ Likely 1 transfer: Mount Baker TC (8/48) to Alaska Junction (C Line); expect transfer downtown or SODO. Crosstown south-end trip.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 13. `transfer-lakecity-fremont`

| | |
|---|---|
| **From** | Lake City Way NE & NE 125th St — stop `76710` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_76710)) |
| **To** | Fremont Ave N & N 34th St — stop `26510` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_26510)) |
| **Depart** | 2026-08-05 (Wednesday) at **16:30** |
| **Expected transfers** | 1 (guess — confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=Lake%20City%20Way%20NE%20%26%20NE%20125th%20St%2C%20Seattle%2C%20WA&destination=Fremont%20Ave%20N%20%26%20N%2034th%20St%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ Likely 1 transfer: Lake City (61/72/322/372/522) to Fremont (31/32/40/62); expect 61 to Northgate then 40, or 372 to U District then 31/32.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

### 14. `transfer-wallingford-beaconhill`

| | |
|---|---|
| **From** | N 45th St & Wallingford Ave N — stop `17310` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_17310)) |
| **To** | Beacon Ave S & S Lander St — stop `3810` ([OneBusAway](https://pugetsound.onebusaway.org/where/standard/stop.action?id=1_3810)) |
| **Depart** | 2026-08-05 (Wednesday) at **17:30** |
| **Expected transfers** | 1 (guess — confirm) |
| **Tolerance** | 5 min on arrival |
| **Google Maps** | [open transit directions](https://www.google.com/maps/dir/?api=1&origin=N%2045th%20St%20%26%20Wallingford%20Ave%20N%2C%20Seattle%2C%20WA&destination=Beacon%20Ave%20S%20%26%20S%20Lander%20St%2C%20Seattle%2C%20WA&travelmode=transit) |

> _Note in the JSON:_ Likely 1 transfer (maybe 2): Wallingford (44/62) to Beacon Hill (36/60/107); expect 62 downtown then 36. PM peak.

```
routes:      ____________________________   (boarding order, e.g. 49 -> 40)
arrival:     __ __ : __ __ : __ __
transfers:   ____   source: ______________________________________
```

## How to fill this in

Take case 1, `transfer-capitolhill-ballard`. Say Google Maps shows the 08:04 route
8 from E John St & Broadway E, transferring at Denny Way & Westlake to the D Line,
arriving at NW Market St & Ballard Ave NW at 08:57. Before:

```json
{
  "id": "transfer-capitolhill-ballard",
  "description": "TODO-HUMAN: Capitol Hill (Broadway & E John) to Ballard (NW Market St & Ballard Ave NW), ~8am weekday. Likely 49/8 then 40 or D Line, 1 transfer.",
  "date": "2026-08-05",
  "origin_stop_id": "29270",
  "origin_stop_name": "E John St & Broadway  E - Bay 1",
  "destination_stop_id": "18120",
  "destination_stop_name": "NW Market St & Ballard Ave NW",
  "departure_time": "08:00:00",
  "expected_routes": [],
  "expected_arrival": "",
  "tolerance_min": 5,
  "transfers": 1,
  "verified": false,
  "source": "TODO-human (Google Maps / OneBusAway)"
}
```

After — the shape matches the already-verified `direct-49-morning` entry, which
carries real `expected_routes` / `expected_arrival` values, `verified: true`, and a
`source` naming exactly where the answer came from:

```json
{
  "id": "transfer-capitolhill-ballard",
  "description": "Capitol Hill to Ballard via route 8 and the D Line, 1 transfer, weekday AM peak.",
  "date": "2026-08-05",
  "origin_stop_id": "29270",
  "origin_stop_name": "E John St & Broadway  E - Bay 1",
  "destination_stop_id": "18120",
  "destination_stop_name": "NW Market St & Ballard Ave NW",
  "departure_time": "08:00:00",
  "expected_routes": [
    "8",
    "D Line"
  ],
  "expected_arrival": "08:57:00",
  "tolerance_min": 5,
  "transfers": 1,
  "verified": true,
  "source": "human: Google Maps transit, depart 2026-08-05 08:00, checked 2026-09-01"
}
```

(Those routes and that 08:57 are an **illustration of the edit**, not a result —
do the lookup.)

Then run the suite; the newly-verified cases are picked up automatically:

```
go test ./... -run TestGroundTruth -v
```

A failure here is a finding, not a chore: it means either the lookup or the engine
is wrong. Re-check the lookup once, and if it holds, the bug is in the engine —
leave the ground truth alone and fix the code.

## Things to watch for

These were flagged when the worksheet was generated; resolve them as you go.

| Case(s) | Issue |
|---|---|
| `early-seatac-downtown`, `transfer-latenight-udistrict-downtown` | `transfers: 0` — despite the `transfer-` prefix on the second, both are expected to be **direct**. If the real answer needs a transfer, update `transfers` rather than forcing a direct trip. |
| `early-seatac-downtown` (dest `433`) vs `transfer-greenlake-downtown` (dest `575`) | Two different stop IDs share the name **3rd Ave & Pike St**. Ambiguous names are exactly what `docs/CONTEXT.md` decision 8 refuses to guess at — check both OneBusAway pages and make sure each case uses the stop on the correct side of 3rd Ave for its approach direction. |
| `transfer-capitolhill-ballard`, `transfer-fremont-capitolhill` | Stop name `E John St & Broadway  E - Bay 1` contains a double space. Harmless in JSON, but it was collapsed to a single space in the Google Maps links above. |
| `direct-49-morning` / `transfer-westseattle-udistrict` / `transfer-madisonvalley-udistrict` | `U District Station - Bay 3` is used as an origin in one case and a destination in others. A bay is direction-specific; confirm arriving buses actually serve Bay 3 rather than another bay at the same station. |
| `transfer-saturday-ballard-beacon` | The only Saturday case (2026-08-08). Make sure the Maps query is set to that Saturday — weekday results will silently differ. |
| All 8 cases added 2026-07-27 | Per the file's own `_readme`, their `transfers` value is a route-overlap heuristic guess and `expected_routes` / `expected_arrival` are `null` (not `[]` / `""` as in the six older TODO cases). Filling them in normalizes that. |

Dates check out: 2026-08-05 is a Wednesday and 2026-08-08 a Saturday, both inside
service 29380 (weekdays through 2026-08-28) and the feed window
2026-07-20 .. 2027-03-26. Stop names and IDs could not be cross-checked against
`stops.txt` here — `data/` is gitignored and the feed is not present — so treat
the OneBusAway link as the authority on what each ID actually is.
