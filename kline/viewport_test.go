package kline

import (
	"math"
	"testing"
	"time"
)

func TestPanZoomClamp(t *testing.T) {
	const plotW = 400.0
	const n = 200
	v := Viewport{BarWidth: DefaultBarWidth}
	v.FitRight(n, plotW)
	// Last bar sits OverscrollBars in from the right, so Offset is at
	// the clamp's maximum.
	if got, want := v.Offset, float64(n)-plotW/v.BarWidth+float64(OverscrollBars); math.Abs(got-want) > 1e-9 {
		t.Fatalf("fit offset %v, want %v", got, want)
	}

	before := v.Offset
	v.Pan(80, plotW, n) // drag right, look at earlier bars
	if v.Offset >= before-80/v.BarWidth+0.01 {
		t.Fatalf("pan right offset %v, before %v", v.Offset, before)
	}
	v.Pan(-1e9, plotW, n) // drag far left, clamp at the right edge
	if math.Abs(v.Offset-before) > 1e-6 {
		t.Fatalf("clamp after over-pan %v, want %v", v.Offset, before)
	}

	anchor := 100.0
	idx := v.Offset + anchor/v.BarWidth
	width := v.BarWidth
	v.Zoom(-80, anchor, plotW, n) // wheel up, zoom in
	if v.BarWidth <= width {
		t.Fatalf("zoom in width %v, was %v", v.BarWidth, width)
	}
	if got := v.Offset + anchor/v.BarWidth; math.Abs(got-idx) > 1e-6 {
		t.Fatalf("zoom moved index under anchor: got %v want %v", got, idx)
	}
	v.Zoom(1e9, anchor, plotW, n)
	if v.BarWidth != MinBarWidth {
		t.Fatalf("zoom out stopped at %v", v.BarWidth)
	}
}

func TestIndexAndVisible(t *testing.T) {
	v := Viewport{Offset: 10.25, BarWidth: 8}
	i, ok := v.IndexAt(0, 100)
	if !ok || i != 10 {
		t.Fatalf("index at left edge %d ok=%v", i, ok)
	}
	i, ok = v.IndexAt(8, 100)
	if !ok || i != 11 {
		t.Fatalf("index one bar in %d ok=%v", i, ok)
	}
	if _, ok = v.IndexAt(-1000, 100); ok {
		t.Fatal("index far left should miss")
	}
	first, last := v.Visible(100, 80)
	// offset 10.25, width 80, bar 8: floor(10.25)=10 through
	// ceil(10.25+10)-1=20. Bar 20 still crosses the right edge.
	if first != 10 || last != 20 {
		t.Fatalf("visible %d..%d", first, last)
	}
	if left := v.BarLeft(10); math.Abs(left-(-0.25*8)) > 1e-9 {
		t.Fatalf("bar left %v", left)
	}
}

func TestPriceDomainAndTime(t *testing.T) {
	cs := []Candle{
		{Time: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Open: 10, High: 12, Low: 9, Close: 11},
		{Time: time.Date(2024, 1, 2, 1, 0, 0, 0, time.UTC), Open: 11, High: 14, Low: 10, Close: 13},
		{Time: time.Date(2024, 1, 2, 2, 0, 0, 0, time.UTC), Open: 13, High: 13, Low: 8, Close: 9},
	}
	lo, hi := PriceDomain(cs, 0, 2, []float64{math.NaN(), 15, 9})
	if lo >= 8 || hi <= 15 {
		t.Fatalf("domain %v..%v should pad outside 8..15", lo, hi)
	}
	flat := []Candle{{High: 5, Low: 5}}
	lo, hi = PriceDomain(flat, 0, 0)
	if hi <= lo {
		t.Fatalf("flat domain %v..%v", lo, hi)
	}
	if got := TimeAt(cs, 1.5); !got.Equal(cs[1].Time.Add(30 * time.Minute)) {
		t.Fatalf("mid time %s", got)
	}
	if got := TimeAt(cs, -1); !got.Equal(cs[0].Time.Add(-time.Hour)) {
		t.Fatalf("extrapolated time %s", got)
	}
	if got := VolumeMax(cs, 0, 2); got != 1 {
		t.Fatalf("zero volumes should floor at 1, got %v", got)
	}
	cs[1].Volume = 100
	if VolumeMax(cs, 0, 2) <= 100 {
		t.Fatal("volume max should pad above 100")
	}
}
