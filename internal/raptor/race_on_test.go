//go:build race

package raptor

// raceEnabled reports whether the race detector is compiled in. Latency
// assertions are skipped under -race: its instrumentation slows code
// ~10x, so wall-clock thresholds would only measure the detector.
const raceEnabled = true
