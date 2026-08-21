package main

import (
	"encoding/json"
	"math"
	"testing"
)

// PR #25. The bug was not the division -- it was trusting getFloat64's default to guard it.
// A default fires on an ABSENT key. The daemon sends averageblocktime50 PRESENT and zero on a
// fresh chain, because there are not yet 50 blocks to average. Different failure mode, so the
// default is not a guard.

// Measured on a fresh simulator (still 0 at height 5): the key arrives, so the fallback never
// runs and the caller divides by the real value, 0.
func TestLiveStats_ZeroBlockTimeDefeatsTheDefault(t *testing.T) {
	fresh := map[string]interface{}{"averageblocktime50": float64(0)}

	if got := getFloat64(fresh, "averageblocktime50", 18.0); got != 0 {
		t.Fatalf("present-and-zero must return 0, not the default; got %v", got)
	}
	// NEGATIVE CONTROL: absent is the case the default actually covers.
	if got := getFloat64(map[string]interface{}{}, "averageblocktime50", 18.0); got != 18.0 {
		t.Fatalf("absent key must return the default; got %v", got)
	}
}

func TestComputeHashrate_RefusesZeroBlockTime(t *testing.T) {
	if got := computeHashrate(1, 0); got != 0 {
		t.Fatalf("zero block time must yield 0, got %v", got)
	}
	if got := computeHashrate(1, -1); got != 0 {
		t.Fatalf("negative block time must yield 0, got %v", got)
	}
	// The guard must be inert on any live chain. Mainnet was measured at abt50 71.02.
	if got := computeHashrate(100000, 71.02); math.Abs(got-1408.05) > 0.1 {
		t.Fatalf("live-chain path must be unchanged, got %v", got)
	}
}

// The consequence, not just the value: +Inf is unserializable, so one bad field takes the
// whole stats map with it -- height, peers and supply included. This is what crashed the app.
func TestLiveStats_PayloadMarshalsOnAFreshChain(t *testing.T) {
	// Reproduce the pre-fix value to prove the test can actually fail. Via variables: the
	// compiler rejects a constant division by zero, which is precisely why this survived
	// review as source -- it only exists once the zero arrives at runtime.
	difficulty, blockTime := float64(1), float64(0)
	unguarded := difficulty / blockTime
	if !math.IsInf(unguarded, 1) {
		t.Fatal("expected +Inf -- NEGATIVE CONTROL for the whole test")
	}
	if _, err := json.Marshal(map[string]interface{}{"hashrate": unguarded}); err == nil {
		t.Fatal("json.Marshal must reject +Inf -- if this passes, the bug is unreachable and this test is worthless")
	}

	payload := map[string]interface{}{
		"height":   int64(1),
		"peers":    int64(0),
		"hashrate": computeHashrate(1, 0),
	}
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("guarded stats payload must serialize on a fresh chain: %v", err)
	}
}
