package kline

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSplitStackPlacesMACDUnderVolume(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 800, H: 500}
	panes := SplitStack(r, true, true, false)
	if panes.Price.H <= 0 || panes.Volume.H <= 0 || panes.MACD.H <= 0 || panes.Time.H <= 0 {
		t.Fatalf("empty band: %+v", panes)
	}
	if panes.Volume.Y < panes.Price.Y+panes.Price.H {
		t.Fatalf("volume should sit under price: %+v %+v", panes.Price, panes.Volume)
	}
	if panes.MACD.Y < panes.Volume.Y+panes.Volume.H {
		t.Fatalf("MACD should sit under volume: %+v %+v", panes.Volume, panes.MACD)
	}
	if panes.Time.Y < panes.MACD.Y+panes.MACD.H-0.5 {
		t.Fatalf("time axis should sit under MACD: time %+v macd %+v", panes.Time, panes.MACD)
	}
	if panes.MACD.W != panes.Price.W || panes.Volume.W != panes.Price.W {
		t.Fatal("price, volume, and MACD should share a width")
	}
	if panes.MACDAxis.X != panes.Price.X+panes.Price.W || panes.MACDAxis.H != panes.MACD.H {
		t.Fatalf("MACD axis %+v plot %+v", panes.MACDAxis, panes.MACD)
	}

	noTime := SplitStack(r, false, true, false)
	if noTime.Time.H != 0 {
		t.Fatalf("time axis belongs on the bottom pane only: %+v", noTime.Time)
	}
	if noTime.MACD.H <= 0 || noTime.MACD.Y < noTime.Volume.Y+noTime.Volume.H {
		t.Fatalf("MACD without the time axis: %+v", noTime)
	}

	off := SplitStack(r, true, false, false)
	legacy := Split(r)
	if off.Price != legacy.Price || off.Volume != legacy.Volume || off.MACD != legacy.MACD || off.KDJ != legacy.KDJ || off.Time != legacy.Time || off.PriceAxis != legacy.PriceAxis || off.VolumeAxis != legacy.VolumeAxis || off.MACDAxis != legacy.MACDAxis || off.KDJAxis != legacy.KDJAxis || len(off.Extra) != 0 || len(legacy.Extra) != 0 {
		t.Fatalf("MACD off should match Split\n got %+v\nwant %+v", off, legacy)
	}
}

// splitBoth is the two-sub-pane geometry. A tall box splits by share; a
// box too short for the shares falls back so the price plot keeps at
// least one DIP and every band stays positive.
func TestSplitBothKeepsEveryBandPositive(t *testing.T) {
	for _, tc := range []struct {
		name string
		rect ui.Rect
	}{
		{"tall", ui.Rect{X: 0, Y: 0, W: 800, H: 800}},
		{"medium", ui.Rect{X: 0, Y: 0, W: 800, H: 500}},
		{"short", ui.Rect{X: 0, Y: 0, W: 800, H: 200}},
		{"tiny", ui.Rect{X: 0, Y: 0, W: 200, H: 60}},
		// Shorter than the gaps between the four bands: the shares do
		// not fit, so the fallback has to keep the price plot at 1 DIP.
		{"collapsed", ui.Rect{X: 0, Y: 0, W: 800, H: 24}},
		{"narrow", ui.Rect{X: 0, Y: 0, W: 60, H: 400}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panes := SplitStack(tc.rect, true, true, true)
			if panes.Price.H < 1 {
				t.Fatalf("price height %v", panes.Price.H)
			}
			for _, band := range []struct {
				name string
				r    ui.Rect
			}{
				{"volume", panes.Volume},
				{"macd", panes.MACD},
				{"kdj", panes.KDJ},
			} {
				if band.r.W < 0 || band.r.H < 0 {
					t.Fatalf("%s rect is negative: %+v", band.name, band.r)
				}
			}
			// The bands that have a height stay ordered top to bottom.
			if panes.Volume.H > 0 && panes.Volume.Y < panes.Price.Y+panes.Price.H {
				t.Fatalf("volume overlaps price: price %+v volume %+v", panes.Price, panes.Volume)
			}
			if panes.MACD.H > 0 && panes.MACD.Y < panes.Volume.Y+panes.Volume.H {
				t.Fatalf("MACD overlaps volume: volume %+v macd %+v", panes.Volume, panes.MACD)
			}
			if panes.KDJ.H > 0 && panes.KDJ.Y < panes.MACD.Y+panes.MACD.H {
				t.Fatalf("KDJ overlaps MACD: macd %+v kdj %+v", panes.MACD, panes.KDJ)
			}
			if panes.Price.W < 1 {
				t.Fatalf("price width %v", panes.Price.W)
			}
			// The axis column sits right of the plot on every band.
			if panes.PriceAxis.X != panes.Price.X+panes.Price.W {
				t.Fatalf("price axis %+v plot %+v", panes.PriceAxis, panes.Price)
			}
			if panes.KDJAxis.H != panes.KDJ.H || panes.MACDAxis.H != panes.MACD.H {
				t.Fatalf("axis heights: macd %+v/%+v kdj %+v/%+v", panes.MACD, panes.MACDAxis, panes.KDJ, panes.KDJAxis)
			}
		})
	}
}

// A narrow box shrinks the axis column instead of the plot.
func TestSplitBothNarrowBoxShrinksAxis(t *testing.T) {
	wide := SplitStack(ui.Rect{W: 800, H: 500}, true, true, true)
	narrow := SplitStack(ui.Rect{W: 80, H: 500}, true, true, true)
	if narrow.PriceAxis.W >= wide.PriceAxis.W {
		t.Fatalf("narrow axis %v, wide axis %v", narrow.PriceAxis.W, wide.PriceAxis.W)
	}
	if narrow.Price.W < 1 {
		t.Fatalf("narrow plot %v", narrow.Price.W)
	}
	if narrow.Price.W+narrow.PriceAxis.W > 80 {
		t.Fatalf("narrow box overflows: plot %v axis %v", narrow.Price.W, narrow.PriceAxis.W)
	}
}

func TestSplitStackPlacesKDJUnderMACD(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 800, H: 500}
	both := SplitStack(r, true, true, true)
	if both.Price.H <= 0 || both.Volume.H <= 0 || both.MACD.H <= 0 || both.KDJ.H <= 0 || both.Time.H <= 0 {
		t.Fatalf("empty band: %+v", both)
	}
	if both.Volume.Y < both.Price.Y+both.Price.H || both.MACD.Y < both.Volume.Y+both.Volume.H {
		t.Fatalf("price, volume, MACD order %+v", both)
	}
	if both.KDJ.Y < both.MACD.Y+both.MACD.H {
		t.Fatalf("KDJ should sit under MACD: macd %+v kdj %+v", both.MACD, both.KDJ)
	}
	if both.Time.Y < both.KDJ.Y+both.KDJ.H-0.5 {
		t.Fatalf("time axis should sit under KDJ: time %+v kdj %+v", both.Time, both.KDJ)
	}
	if both.KDJ.W != both.Price.W || both.KDJAxis.X != both.Price.X+both.Price.W || both.KDJAxis.H != both.KDJ.H {
		t.Fatalf("KDJ axis %+v plot %+v", both.KDJAxis, both.KDJ)
	}

	only := SplitStack(r, true, false, true)
	if only.MACD.H != 0 {
		t.Fatalf("MACD off should leave no MACD band: %+v", only.MACD)
	}
	if only.KDJ.H <= 0 || only.KDJ.Y < only.Volume.Y+only.Volume.H || only.Time.Y < only.KDJ.Y+only.KDJ.H-0.5 {
		t.Fatalf("KDJ only should sit under volume: %+v", only)
	}
	macdOnly := SplitStack(r, true, true, false)
	if macdOnly.KDJ.H != 0 || macdOnly.KDJAxis.H != 0 {
		t.Fatalf("KDJ off should leave no KDJ band: %+v", macdOnly.KDJ)
	}
}
