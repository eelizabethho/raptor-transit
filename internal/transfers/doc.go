// Package transfers synthesizes walking footpaths between nearby stops.
//
// King County Metro's GTFS feed ships no transfers.txt, but RAPTOR needs
// footpaths to model walking between stops (e.g. crossing the street to a
// stop on the other side). This package generates them from stop
// coordinates alone: every pair of stops within a maximum walking distance
// gets a footpath in both directions.
//
// Two deliberate omissions, both handled by the RAPTOR caller:
//
//   - No self-transfers. A stop is trivially reachable from itself at zero
//     cost, and RAPTOR must treat it that way, but emitting ~7000 redundant
//     (i, i, 0) entries would just bloat the table. The caller handles the
//     self case directly, which is cleaner than filtering it here.
//
//   - No transitive closure. Classic RAPTOR preprocessing sometimes closes
//     footpaths transitively (if A→B and B→C are walkable, add A→C), but
//     our RAPTOR applies footpaths at the end of every round, so multi-hop
//     walks emerge naturally across rounds. Each individual footpath here
//     is a direct walk of at most maxMeters.
package transfers
