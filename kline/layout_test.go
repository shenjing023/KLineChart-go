package kline

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSplitStackPlacesMACDUnderVolume(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 800, H: 500}
	panes := SplitStack(r, true, true)
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

	noTime := SplitStack(r, false, true)
	if noTime.Time.H != 0 {
		t.Fatalf("time axis belongs on the bottom pane only: %+v", noTime.Time)
	}
	if noTime.MACD.H <= 0 || noTime.MACD.Y < noTime.Volume.Y+noTime.Volume.H {
		t.Fatalf("MACD without the time axis: %+v", noTime)
	}

	off := SplitStack(r, true, false)
	legacy := Split(r)
	if off != legacy {
		t.Fatalf("MACD off should match Split\n got %+v\nwant %+v", off, legacy)
	}
}
