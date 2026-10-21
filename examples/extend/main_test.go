package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

// init registered the host indicator and the host tool, so a chart built
// here must pick both up. A window cannot open in a test, so the view is
// driven in memory.
func TestHostRegistrationsReachTheChart(t *testing.T) {
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"600519"},
		Indicators: []string{"RANGE", "MA"},
	})
	if err := chart.LoadError(); err != nil {
		t.Fatal(err)
	}
	if !chart.IndicatorOn("RANGE") {
		t.Fatal("RANGE did not start on")
	}
	if !containsName(chart.IndicatorNames(), "RANGE") {
		t.Fatalf("RANGE missing from %v", chart.IndicatorNames())
	}
	if err := chart.SetTool("mark"); err != nil {
		t.Fatalf("the host tool is not selectable: %v", err)
	}
	if chart.Tool() != "mark" {
		t.Fatalf("tool %q", chart.Tool())
	}

	tester := ui.NewTester(chart.View, 900, 620)
	tester.Frame()
	box, ok := tester.Find("600519 chart")
	if !ok {
		t.Fatal("the chart did not paint a plot")
	}
	tester.Move(box.X+box.W/2, box.Y+30)
	if readout := chart.Readout("600519"); !containsName([]string{readout}, readout) || !containsText(readout, "RANGE") {
		t.Fatalf("readout %q does not carry the host indicator", readout)
	}
}

// The host line plot must reach the price scale and the header the same
// way a built-in main indicator does.
func TestRangeLineValues(t *testing.T) {
	cs := kline.Generate(30, 3)
	line := &rangeLine{}
	line.Calc(cs)
	if len(line.values) != len(cs) {
		t.Fatalf("values %d, want %d", len(line.values), len(cs))
	}
	for i, c := range cs {
		if got, want := line.values[i], c.High-c.Low; got != want {
			t.Fatalf("bar %d range %v, want %v", i, got, want)
		}
	}
	if len(line.Series()) != 1 || len(line.Series()[0]) != len(cs) {
		t.Fatalf("series shape %v", len(line.Series()))
	}
	if colors := line.LineColors(); len(colors) != 1 || colors[0] == (ui.Color{}) {
		t.Fatalf("colors %+v", colors)
	}
	if line.ValueAt(-1) != "" || line.ValueAt(len(cs)) != "" {
		t.Fatal("ValueAt should be empty outside the series")
	}
	if got := line.ValueAt(0); got == "" || !containsText(got, "RANGE") {
		t.Fatalf("ValueAt %q", got)
	}
}

// The host tool's hit test finds the anchor and nothing else.
func TestHostMarkHit(t *testing.T) {
	cs := kline.Generate(40, 5)
	vp := kline.Viewport{BarWidth: 8, Offset: 0}
	scale := kline.MainScale(cs, nil, vp, 400)
	o := kline.Overlay{Kind: "mark", Points: []kline.Point{{BarIndex: 10, Price: cs[10].Close}}}
	x := kline.AnchorX(o.Points[0], vp, cs)
	y := kline.PriceToLocalY(500, o.Points[0].Price, scale)
	if part, ok := hitMark(o, x, y, 400, 500, vp, scale, cs); !ok || part != kline.HitBody {
		t.Fatalf("hit at the anchor: %d %v", part, ok)
	}
	if _, ok := hitMark(o, x+kline.HitRadius+10, y, 400, 500, vp, scale, cs); ok {
		t.Fatal("hit away from the anchor")
	}
	if _, ok := hitMark(kline.Overlay{}, 0, 0, 400, 500, vp, scale, cs); ok {
		t.Fatal("an empty overlay should not hit")
	}
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func containsText(s, want string) bool {
	for i := 0; i+len(want) <= len(s); i++ {
		if s[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
