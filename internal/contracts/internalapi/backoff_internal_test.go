package internalapi

import (
	"testing"
	"time"
)

// 退避必须落在 [基准/2, 基准]，基准按指数增长并被 MaxDelay 封顶；抖动不得让等待为负或越界。
func TestBackoffIsExponentialJitteredAndBounded(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 6, BaseDelay: 50 * time.Millisecond, MaxDelay: 300 * time.Millisecond}
	bases := []time.Duration{50, 100, 200, 300, 300, 300} // ms，第 4 次起被封顶
	for i, b := range bases {
		base := b * time.Millisecond
		for n := 0; n < 200; n++ {
			got := p.backoff(i + 1)
			if got < base/2 || got > base {
				t.Fatalf("attempt %d: backoff %v outside [%v, %v]", i+1, got, base/2, base)
			}
		}
	}
}

func TestBackoffZeroBaseMeansNoWait(t *testing.T) {
	if got := (RetryPolicy{MaxAttempts: 3}).backoff(1); got != 0 {
		t.Fatalf("backoff = %v, want 0", got)
	}
}

func TestBackoffShowsJitter(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 3, BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}
	seen := map[time.Duration]bool{}
	for n := 0; n < 100; n++ {
		seen[p.backoff(1)] = true
	}
	if len(seen) < 5 {
		t.Fatalf("only %d distinct delays in 100 draws: jitter is missing", len(seen))
	}
}
