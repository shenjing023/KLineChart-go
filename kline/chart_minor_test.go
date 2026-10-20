package kline

import (
	"testing"

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
