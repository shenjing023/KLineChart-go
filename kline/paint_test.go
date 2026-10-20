package kline

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestApplyIndicatorStyleColorsOpenPanes(t *testing.T) {
	macd := NewMACD(12, 26, 9)
	kdj := NewKDJ(9, 3, 3)
	macd.DIFColor, macd.DEAColor, macd.Up, macd.Down, macd.Zero = ui.Color{}, ui.Color{}, ui.Color{}, ui.Color{}, ui.Color{}
	kdj.KColor, kdj.DColor, kdj.JColor = ui.Color{}, ui.Color{}, ui.Color{}
	style := DarkDefault()
	applyIndicatorStyle([]Indicator{macd, kdj}, Panes{}, style)
	if macd.DIFColor != (ui.Color{}) || kdj.KColor != (ui.Color{}) {
		t.Fatal("a closed sub-pane should not take theme colors")
	}
	open := Panes{MACD: ui.Rect{W: 10, H: 40}, KDJ: ui.Rect{W: 10, H: 40}}
	applyIndicatorStyle([]Indicator{macd, kdj}, open, style)
	if macd.DIFColor != style.MACDDIF || macd.DEAColor != style.MACDDEA || macd.Up != style.Up || macd.Zero != style.MACDZero {
		t.Fatalf("MACD colors %+v", macd)
	}
	if kdj.KColor != style.KDJK || kdj.DColor != style.KDJD || kdj.JColor != style.KDJJ {
		t.Fatalf("KDJ colors %+v", kdj)
	}
}

func TestSplitGivesPriceAndVolume(t *testing.T) {
	panes := Split(ui.Rect{X: 10, Y: 20, W: 800, H: 500})
	if panes.Price.W <= 0 || panes.Price.H <= 0 {
		t.Fatalf("price pane %+v", panes.Price)
	}
	if panes.Volume.Y <= panes.Price.Y+panes.Price.H {
		t.Fatalf("volume should sit below price: %+v %+v", panes.Price, panes.Volume)
	}
	if panes.PriceAxis.X != panes.Price.X+panes.Price.W {
		t.Fatalf("price axis %+v", panes.PriceAxis)
	}
	if panes.Volume.W != panes.Price.W {
		t.Fatal("volume and price plots should share a width")
	}
	if panes.Time.Y < panes.Volume.Y+panes.Volume.H-0.5 {
		t.Fatalf("time axis %+v volume %+v", panes.Time, panes.Volume)
	}
	if panes.Time.H <= 0 {
		t.Fatal("Split should keep a time axis")
	}
	noTime := SplitNoTime(ui.Rect{X: 10, Y: 20, W: 800, H: 500})
	if noTime.Time.H != 0 {
		t.Fatalf("SplitNoTime time axis %+v", noTime.Time)
	}
	if noTime.Volume.Y <= noTime.Price.Y+noTime.Price.H || noTime.Price.W != panes.Price.W {
		t.Fatalf("SplitNoTime panes %+v", noTime)
	}
}
