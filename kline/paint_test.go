package kline

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

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
