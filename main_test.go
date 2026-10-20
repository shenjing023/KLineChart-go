package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestViewShowsLatestBar(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	last := a.candles[len(a.candles)-1]
	if !tt.HasText("K-line") || !tt.HasText("600 bars") || !tt.HasText("Last") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if !tt.HasText(last.Time.UTC().Format("2006-01-02 15:04")) {
		t.Fatalf("HUD missing last bar time, texts %q", tt.Texts())
	}
	if !a.fitted || a.plotW <= 0 {
		t.Fatalf("first frame should measure the plot, fitted=%v plotW=%v", a.fitted, a.plotW)
	}
}

func TestDragPansAndWheelZooms(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 640)
	box, ok := tt.Find("Candlestick chart")
	if !ok {
		t.Fatalf("chart box missing, texts %q", tt.Texts())
	}
	x := box.X + box.W/2
	y := box.Y + 40 // inside the price pane, above the volume strip
	start := a.vp.Offset
	width := a.vp.BarWidth

	tt.Press(x, y)
	tt.Move(x+120, y)
	tt.Release(x+120, y)
	if a.vp.Offset >= start-1 {
		t.Fatalf("drag right should show earlier bars: offset %v, start %v", a.vp.Offset, start)
	}

	tt.Scroll(x, y, 0, -100) // wheel up zooms in
	if a.vp.BarWidth <= width {
		t.Fatalf("wheel up should widen bars: %v, was %v", a.vp.BarWidth, width)
	}

	tt.Move(x, y)
	if !tt.HasText("Hover") {
		t.Fatalf("pointer over the plot should show a hover readout, texts %q", tt.Texts())
	}

	fresh := newApp()
	_ = ui.NewTester(fresh.view, 960, 640)
	if err := tt.Click("Reset view"); err != nil {
		t.Fatal(err)
	}
	if math.Abs(a.vp.Offset-fresh.vp.Offset) > 0.5 || math.Abs(a.vp.BarWidth-fresh.vp.BarWidth) > 0.5 {
		t.Fatalf("reset offset %v width %v, fresh offset %v width %v", a.vp.Offset, a.vp.BarWidth, fresh.vp.Offset, fresh.vp.BarWidth)
	}
}
