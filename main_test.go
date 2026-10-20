package main

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func TestViewShowsLatestBar(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 720)
	last := a.panes[0].candles[len(a.panes[0].candles)-1]
	if !tt.HasText("K-line") || !tt.HasText("600 bars") || !tt.HasText("ALPHA") || !tt.HasText("BETA") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if !tt.HasText(last.Time.UTC().Format("2006-01-02 15:04")) {
		t.Fatalf("HUD missing last bar time, texts %q", tt.Texts())
	}
	if !strings.Contains(a.readout(0), "Last") || !strings.Contains(a.readout(1), "Last") {
		t.Fatalf("readouts %q %q", a.readout(0), a.readout(1))
	}
	if !a.fitted || a.plotW <= 0 {
		t.Fatalf("first frame should measure the plot, fitted=%v plotW=%v", a.fitted, a.plotW)
	}
	if a.panes[0].candles[len(a.panes[0].candles)-1].Close == a.panes[1].candles[len(a.panes[1].candles)-1].Close {
		t.Fatal("the two series should not be the same walk")
	}
}

func TestDragPansAndWheelZooms(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 720)
	top := mustBox(t, tt, "ALPHA chart")
	bot := mustBox(t, tt, "BETA chart")
	x := top.X + top.W/2
	y := top.Y + 24
	start := a.vp.Offset
	width := a.vp.BarWidth

	tt.Press(x, y)
	tt.Move(x+120, y)
	tt.Release(x+120, y)
	if a.vp.Offset >= start-1 {
		t.Fatalf("drag on the upper pane should pan the shared viewport: offset %v, start %v", a.vp.Offset, start)
	}

	// The lower pane drives the same viewport.
	lowerStart := a.vp.Offset
	lx, ly := bot.X+40, bot.Y+24
	tt.Press(lx, ly)
	tt.Move(lx+100, ly)
	tt.Release(lx+100, ly)
	if a.vp.Offset >= lowerStart-1 {
		t.Fatalf("drag on the lower pane should pan the same viewport: offset %v, before %v", a.vp.Offset, lowerStart)
	}

	tt.Scroll(bot.X+bot.W/2, bot.Y+30, 0, -100) // wheel on BETA zooms both
	if a.vp.BarWidth <= width {
		t.Fatalf("wheel on the lower pane should widen the shared bars: %v, was %v", a.vp.BarWidth, width)
	}

	tt.Move(x, y)
	if !tt.HasText("Hover") {
		t.Fatalf("pointer over the plot should show a hover readout, texts %q", tt.Texts())
	}

	fresh := newApp()
	_ = ui.NewTester(fresh.view, 960, 720)
	if err := tt.Click("Reset view"); err != nil {
		t.Fatal(err)
	}
	if math.Abs(a.vp.Offset-fresh.vp.Offset) > 0.5 || math.Abs(a.vp.BarWidth-fresh.vp.BarWidth) > 0.5 {
		t.Fatalf("reset offset %v width %v, fresh offset %v width %v", a.vp.Offset, a.vp.BarWidth, fresh.vp.Offset, fresh.vp.BarWidth)
	}
}

func TestPanesShareCrosshair(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 720)
	top := mustBox(t, tt, "ALPHA chart")
	bot := mustBox(t, tt, "BETA chart")
	if math.Abs(float64(top.X-bot.X)) > 1 || math.Abs(float64(top.W-bot.W)) > 1 {
		t.Fatalf("panes should share a horizontal frame: top %+v bot %+v", top, bot)
	}

	x := top.X + 140
	tt.Move(x, top.Y+20)
	idx := a.crossIndex
	if idx < 0 || a.crossPane != 0 {
		t.Fatalf("hover on ALPHA index %d pane %d", idx, a.crossPane)
	}
	assertSameBar(t, tt, a, idx)

	// The same local x on BETA is the same bar, because the viewport is shared.
	tt.Move(x, bot.Y+20)
	if a.crossIndex != idx || a.crossPane != 1 {
		t.Fatalf("hover on BETA index %d pane %d, want index %d", a.crossIndex, a.crossPane, idx)
	}
	assertSameBar(t, tt, a, idx)

	tt.Move(10, 10) // outside both plots
	if a.crossIndex != -1 {
		t.Fatalf("leaving the panes should clear the crosshair, index %d", a.crossIndex)
	}
	if strings.Contains(a.readout(0), "Hover") || strings.Contains(a.readout(1), "Hover") {
		t.Fatalf("readouts stayed on hover: %q %q", a.readout(0), a.readout(1))
	}
}

func assertSameBar(t *testing.T, tt *ui.Tester, a *app, idx int) {
	t.Helper()
	if a.panes[0].candles[idx].Time != a.panes[1].candles[idx].Time {
		t.Fatalf("bar %d times differ: %s vs %s", idx, a.panes[0].candles[idx].Time, a.panes[1].candles[idx].Time)
	}
	r0, r1 := a.readout(0), a.readout(1)
	when := a.panes[0].candles[idx].Time.UTC().Format("2006-01-02 15:04")
	if !strings.Contains(r0, "Hover") || !strings.Contains(r1, "Hover") || !strings.Contains(r0, when) || !strings.Contains(r1, when) {
		t.Fatalf("linked readouts:\n%s\n%s", r0, r1)
	}
	p0 := kline.FormatPrice(a.panes[0].candles[idx].Close)
	p1 := kline.FormatPrice(a.panes[1].candles[idx].Close)
	if p0 == p1 {
		t.Fatalf("closes match at %d: %s", idx, p0)
	}
	if !strings.Contains(r0, "C "+p0) || !strings.Contains(r1, "C "+p1) {
		t.Fatalf("each pane should show its own close:\n%s\n%s", r0, r1)
	}
	if !tt.HasText(r0) || !tt.HasText(r1) {
		t.Fatalf("headers missing linked readouts, texts %q", tt.Texts())
	}
}

func mustBox(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing %q, texts %q", label, tt.Texts())
	}
	return r
}
