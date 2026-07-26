package raptor

import (
	"math/rand"
	"sort"
	"testing"
	"time"
)

// TestQueryLatencyP95 runs a few hundred random queries over the real
// feed and reports the latency distribution. Random stop pairs with a
// fixed seed keep it reproducible.
func TestQueryLatencyP95(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	e := realEngine(t)
	rng := rand.New(rand.NewSource(42))
	n := len(e.tt.StopIDs)

	const runs = 300
	durs := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		from := e.tt.StopIDs[rng.Intn(n)]
		to := e.tt.StopIDs[rng.Intn(n)]
		depTime := int32(6*3600 + rng.Intn(16*3600)) // 06:00..22:00
		start := time.Now()
		if _, err := e.Query(from, to, depTime, "20260805"); err != nil {
			t.Fatal(err)
		}
		durs = append(durs, time.Since(start))
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p50 := durs[runs/2]
	p95 := durs[runs*95/100]
	p99 := durs[runs*99/100]
	max := durs[runs-1]
	t.Logf("query latency over %d random queries: p50=%s p95=%s p99=%s max=%s", runs, p50, p95, p99, max)
	if p95 > 200*time.Millisecond {
		t.Errorf("p95 = %s, want < 200ms", p95)
	}
}
