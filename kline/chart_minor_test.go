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
