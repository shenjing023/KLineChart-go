package kline

import (
	"math"
	"testing"
)

func TestSyntheticPagingUsesFixedHistoryBoundary(t *testing.T) {
	p := Period{Span: 1, Unit: UnitHour}
	for _, page := range []int{1, 17, 200, 800} {
		src := &SyntheticSource{Extra: 30}
		bars, more, err := src.GetBars(BarRequest{Symbol: "600519", Period: p, Kind: KindInit, Limit: 600})
		if err != nil || len(bars) != 600 || !more {
			t.Fatalf("init: %d %v %v", len(bars), more, err)
		}
		for more {
			older, next, err := src.GetBars(BarRequest{Symbol: "600519", Period: p, Kind: KindForward, Limit: page, Anchor: bars[0].Time})
			if err != nil || len(older) == 0 || len(older) > page {
				t.Fatalf("page size %d: got %d bars, err %v", page, len(older), err)
			}
			if !older[len(older)-1].Time.Before(bars[0].Time) || older[len(older)-1].Close != bars[0].Open {
				t.Fatal("history seam changed time or price")
			}
			bars = append(older, bars...)
			more = next
			if len(bars) > 630 {
				t.Fatal("history boundary grew during paging")
			}
		}
		if len(bars) != 630 {
			t.Fatalf("page size %d lost history: %d, want 630", page, len(bars))
		}
	}
}

func TestSyntheticHistoryCapAvoidsExtraOverflow(t *testing.T) {
	src := &SyntheticSource{Extra: math.MaxInt}
	bars, more, err := src.GetBars(BarRequest{Symbol: "A", Period: Period{Span: 1, Unit: UnitHour}, Kind: KindInit, Limit: math.MaxInt})
	if err != nil || more || len(bars) != maxSyntheticBars {
		t.Fatalf("overflowing extra: %d bars, more %v, err %v", len(bars), more, err)
	}
}
