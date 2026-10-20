package kline

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLayoutPanesMatchBuiltinSplit(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 800, H: 500}
	cases := []struct {
		inds      []string
		macd, kdj bool
	}{
		{[]string{}, false, false},
		{[]string{"MACD"}, true, false},
		{[]string{"KDJ"}, false, true},
		{[]string{"MACD", "KDJ"}, true, true},
		{[]string{"MA", "MACD", "BOLL", "KDJ"}, true, true},
	}
	for _, tc := range cases {
		c := NewChart(ChartOptions{Indicators: tc.inds, Symbols: []string{"ALPHA"}})
		got := c.layoutPanes(r, true)
		want := SplitStack(r, true, tc.macd, tc.kdj)
		if got.Price != want.Price || got.Volume != want.Volume || got.MACD != want.MACD || got.KDJ != want.KDJ || got.Time != want.Time || got.PriceAxis != want.PriceAxis || got.VolumeAxis != want.VolumeAxis || got.MACDAxis != want.MACDAxis || got.KDJAxis != want.KDJAxis {
			t.Fatalf("inds %v\ngot %+v\nwant %+v", tc.inds, got, want)
		}
		if len(got.Extra) != 0 || len(got.ExtraAxis) != 0 {
			t.Fatalf("inds %v leaked extra %+v", tc.inds, got.Extra)
		}
	}
}

func TestSplitManyStacksSubPanes(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 800, H: 640}
	panes := splitMany(r, true, 3)
	if panes.Price.H <= 1 || panes.Volume.H <= 1 || panes.Time.H <= 0 || len(panes.Extra) != 3 {
		t.Fatalf("bands %+v", panes)
	}
	if panes.Volume.Y < panes.Price.Y+panes.Price.H {
		t.Fatal("volume overlaps price")
	}
	prev := panes.Volume
	for i, band := range panes.Extra {
		if band.H <= 1 || band.Y < prev.Y+prev.H || band.W != panes.Price.W {
			t.Fatalf("sub %d %+v prev %+v", i, band, prev)
		}
		if panes.ExtraAxis[i].X != band.X+band.W || panes.ExtraAxis[i].H != band.H {
			t.Fatalf("axis %d %+v", i, panes.ExtraAxis[i])
		}
		prev = band
	}
	if panes.Time.Y < prev.Y+prev.H-0.5 {
		t.Fatalf("time %+v last %+v", panes.Time, prev)
	}
}
