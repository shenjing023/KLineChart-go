package kline

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func TestFibToolNameMatchesOverlayKind(t *testing.T) {
	a := NewChart(ChartOptions{})
	if ToolFib != OverlayFibonacci {
		t.Fatalf("ToolFib %q and OverlayFibonacci %q are different strings", ToolFib, OverlayFibonacci)
	}
	if err := a.SetTool(OverlayFibonacci); err != nil {
		t.Fatalf("SetTool(%q): %v", OverlayFibonacci, err)
	}
	if got := a.Tool(); got != OverlayFibonacci {
		t.Fatalf("Tool() %q, drawing kind %q", got, OverlayFibonacci)
	}
	if err := a.SetTool("fib"); err != nil {
		t.Fatal(err)
	}
	if got := a.Tool(); got != OverlayFibonacci {
		t.Fatalf("legacy name selected %q", got)
	}
	err := RegisterOverlay(OverlaySpec{
		Kind:    "fib",
		Anchors: 1,
		Draw: func(*ui.Painter, ui.Rect, Overlay, Viewport, chart.Linear, []Candle, Style) {
		},
		Hit: func(Overlay, float64, float64, float64, float64, Viewport, chart.Linear, []Candle) (int, bool) {
			return 0, false
		},
	})
	if err == nil {
		t.Fatal(`RegisterOverlay("fib") should stay reserved`)
	}
}

func TestChartPriceDecimalsAndLocation(t *testing.T) {
	digits := 4
	loc := time.FixedZone("UTC+8", 8*3600)
	a := NewChart(ChartOptions{
		Symbols:       []string{"ALPHA"},
		PriceDecimals: &digits,
		Location:      loc,
	})
	bar, ok := a.Bar("ALPHA", a.Bars()-1)
	if !ok {
		t.Fatal("no bar")
	}
	got := a.Readout("ALPHA")
	wantTime := bar.Time.In(loc).Format("2006-01-02 15:04")
	wantPx := FormatPriceDigits(bar.Close, 4)
	if !strings.Contains(got, wantTime) || !strings.Contains(got, "C "+wantPx) {
		t.Fatalf("readout %q\nwant time %s and C %s", got, wantTime, wantPx)
	}

	neg := -3
	b := NewChart(ChartOptions{Symbols: []string{"ALPHA"}, PriceDecimals: &neg})
	bar, _ = b.Bar("ALPHA", b.Bars()-1)
	got = b.Readout("ALPHA")
	if !strings.Contains(got, bar.Time.UTC().Format("2006-01-02 15:04")) || !strings.Contains(got, "C "+FormatPrice(bar.Close)) {
		t.Fatalf("default format changed: %q", got)
	}
}

func TestClickBeforeFirstPaintIsKept(t *testing.T) {
	a := NewChart(ChartOptions{Symbols: []string{"ALPHA"}, Indicators: []string{}})
	if len(a.painted) != 0 {
		t.Fatal("paint ran before the click")
	}
	if err := a.SetTool(ToolHLine); err != nil {
		t.Fatal(err)
	}
	ev := ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: 80, Y: 30}
	if !a.onChartInput(0, ev) {
		t.Fatal("click before the first paint was refused")
	}
	if len(a.panes[0].overlays.Items()) != 0 {
		t.Fatal("line placed before a price rect exists")
	}
	_ = ui.NewTester(a.View, 900, 640)
	items := a.panes[0].overlays.Items()
	if len(items) != 1 || items[0].Kind != OverlayHLine {
		t.Fatalf("first click dropped: %+v", items)
	}
}

func TestClickAfterIndicatorChangeUsesNewPriceRect(t *testing.T) {
	a := NewChart(ChartOptions{Symbols: []string{"ALPHA"}, Indicators: []string{"MACD"}})
	tt := ui.NewTester(a.View, 900, 700)
	if err := a.SetTool(ToolHLine); err != nil {
		t.Fatal(err)
	}
	old := a.painted[0].Price
	if old.H < 40 {
		t.Fatalf("price plot %+v", old)
	}
	if err := a.SetIndicator("MACD", false); err != nil {
		t.Fatal(err)
	}
	// Just under the old price plot. MACD is still in the stored split,
	// and this Y is on the price plot only after that pane closes.
	tt.ClickAt(old.X+40, old.Y+old.H+6)
	items := a.panes[0].overlays.Items()
	if len(items) != 1 || items[0].Kind != OverlayHLine {
		t.Fatalf("click in the grown price plot was refused: old %+v now %+v drawings %+v", old, a.painted[0].Price, items)
	}
}

func TestEscapeAndRightClickCancelDraft(t *testing.T) {
	a := NewChart(ChartOptions{Symbols: []string{"ALPHA"}, Indicators: []string{}})
	tt := ui.NewTester(a.View, 900, 640)
	if err := a.SetTool(ToolSegment); err != nil {
		t.Fatal(err)
	}
	price := a.painted[0].Price
	tt.ClickAt(price.X+80, price.Y+40)
	if !a.draftOn {
		t.Fatal("draft did not start")
	}
	tt.Key(0, ui.KeyEscape)
	if a.draftOn {
		t.Fatal("escape left the draft up")
	}
	if len(a.panes[0].overlays.Items()) != 0 {
		t.Fatal("escape created a segment")
	}

	tt.ClickAt(price.X+80, price.Y+40)
	tt.ClickAt(price.X+220, price.Y+140)
	if a.draftOn || len(a.panes[0].overlays.Items()) != 1 {
		t.Fatalf("finished segment: draft %v drawings %d", a.draftOn, len(a.panes[0].overlays.Items()))
	}
	// Away from the finished segment, which a click on the line would select.
	tt.ClickAt(price.X+price.W-40, price.Y+24)
	if !a.draftOn {
		t.Fatal("second draft did not start")
	}
	tt.RightClickAt(price.X+price.W-30, price.Y+price.H*0.8)
	if a.draftOn {
		t.Fatal("right-click left the draft up")
	}
	if n := len(a.panes[0].overlays.Items()); n != 1 {
		t.Fatalf("right-click during a draft left %d drawings", n)
	}
}
