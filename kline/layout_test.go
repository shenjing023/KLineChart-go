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
	if off != legacy {
		t.Fatalf("MACD off should match Split\n got %+v\nwant %+v", off, legacy)
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
