package service_test

import (
	"testing"

	"github.com/liuyu6610/couponkill-cloud-native/internal/coupon/service"
)

func TestDistributeSpreadsRemainderOverFirstBuckets(t *testing.T) {
	cases := []struct {
		total int64
		n     int
		want  []int64
	}{
		{total: 50, n: 8, want: []int64{7, 7, 6, 6, 6, 6, 6, 6}},
		{total: 1600, n: 8, want: []int64{200, 200, 200, 200, 200, 200, 200, 200}},
		{total: 0, n: 4, want: []int64{0, 0, 0, 0}},
		{total: 3, n: 8, want: []int64{1, 1, 1, 0, 0, 0, 0, 0}},
		{total: 7, n: 1, want: []int64{7}},
	}
	for _, tc := range cases {
		got := service.Distribute(tc.total, tc.n)
		if len(got) != len(tc.want) {
			t.Fatalf("Distribute(%d,%d) len=%d want %d", tc.total, tc.n, len(got), len(tc.want))
		}
		var sum int64
		for i, v := range got {
			sum += v
			if v != tc.want[i] {
				t.Errorf("Distribute(%d,%d)[%d]=%d want %d", tc.total, tc.n, i, v, tc.want[i])
			}
		}
		if sum != tc.total {
			t.Errorf("Distribute(%d,%d) sums to %d", tc.total, tc.n, sum)
		}
	}
}

func TestDistributeRejectsNonPositiveBucketCount(t *testing.T) {
	if got := service.Distribute(10, 0); got != nil {
		t.Fatalf("n=0 must yield nil, got %v", got)
	}
	if got := service.Distribute(-5, 4); got != nil {
		t.Fatalf("negative total must yield nil, got %v", got)
	}
}
