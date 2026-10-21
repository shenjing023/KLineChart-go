package main

import (
	"bytes"
	"image"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

// init registered the host indicator and the host tool, so a chart built
// here must pick both up. A window cannot open in a test, so the view is
// driven in memory.
func TestHostRegistrationsReachTheChart(t *testing.T) {
	a := newExtendExample()
	chart := a.chart
	t.Cleanup(func() { _ = chart.Close() })
	if err := chart.LoadError(); err != nil {
		t.Fatal(err)
	}
	if !chart.IndicatorOn("MID") {
		t.Fatal("MID did not start on")
	}
	if !containsName(chart.IndicatorNames(), "MID") {
		t.Fatalf("MID missing from %v", chart.IndicatorNames())
	}
	if err := chart.SetTool("mark"); err != nil {
		t.Fatalf("the host tool is not selectable: %v", err)
	}
	if chart.Tool() != "mark" {
		t.Fatalf("tool %q", chart.Tool())
	}

	tester := ui.NewTester(a.view, 960, 680)
	tester.Frame()
	box, ok := tester.Find("600519 chart")
	if !ok {
		t.Fatal("the chart did not paint a plot")
	}
	tester.Move(box.X+box.W/2, box.Y+30)
	if readout := chart.Readout("600519"); !containsText(readout, "MID") {
		t.Fatalf("readout %q does not carry the host indicator", readout)
	}
	if got := inkCount(tester.Image(), ui.RGB(255, 140, 0)); got < 40 {
		t.Fatalf("MID line did not paint: %d orange pixels", got)
	}
	if err := tester.Click("Toggle MID"); err != nil {
		t.Fatal(err)
	}
	if chart.IndicatorOn("MID") || strings.Contains(chart.Readout("600519"), "MID") {
		t.Fatal("MID toggle did not remove the indicator/readout")
	}
	if got := inkCount(tester.Image(), ui.RGB(255, 140, 0)); got != 0 {
		t.Fatalf("disabled MID left %d orange pixels", got)
	}
	if err := tester.Click("Toggle MID"); err != nil {
		t.Fatal(err)
	}
	if !chart.IndicatorOn("MID") || inkCount(tester.Image(), ui.RGB(255, 140, 0)) < 40 {
		t.Fatal("MID did not return")
	}
}

// The host line plot must reach the price scale and the header the same
// way a built-in main indicator does.
func TestMidLineValues(t *testing.T) {
	cs := kline.Generate(30, 3)
	line := &midLine{}
	line.Calc(cs)
	if len(line.values) != len(cs) {
		t.Fatalf("values %d, want %d", len(line.values), len(cs))
	}
	for i, c := range cs {
		if got, want := line.values[i], (c.Open+c.Close)/2; got != want {
			t.Fatalf("bar %d mid %v, want %v", i, got, want)
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
	if got := line.ValueAt(0); got == "" || !containsText(got, "MID") {
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
	if part, ok := kline.HitOverlay(o, x, y, 400, 500, vp, scale, cs); !ok || part != kline.HitBody {
		t.Fatalf("hit at the anchor: %d %v", part, ok)
	}
	if _, ok := kline.HitOverlay(o, x+kline.HitRadius+10, y, 400, 500, vp, scale, cs); ok {
		t.Fatal("hit away from the anchor")
	}
	if _, ok := hitMark(kline.Overlay{}, 0, 0, 400, 500, vp, scale, cs); ok {
		t.Fatal("an empty overlay should not hit")
	}
}

func TestNativeViewMarkPlacementDragAndDelete(t *testing.T) {
	a := newExtendExample()
	t.Cleanup(func() { _ = a.chart.Close() })
	tt := ui.NewTester(a.view, 960, 680)
	hot := ui.RGB(255, 80, 190)
	if inkCount(tt.Image(), hot) != 0 {
		t.Fatal("unexpected mark before placement")
	}
	if err := tt.Click("Mark"); err != nil {
		t.Fatal(err)
	}
	if a.chart.Tool() != "mark" {
		t.Fatal("toolbar did not select the custom tool")
	}
	box, ok := tt.Find("600519 chart")
	if !ok {
		t.Fatal("missing chart")
	}
	x, y := box.X+140, box.Y+80
	tt.ClickAt(x, y)
	if n := inkCount(tt.Image(), hot); n < 20 {
		t.Fatalf("custom draw did not produce ink: %d", n)
	}
	x, y = inkCenter(tt.Image(), hot)
	tt.Press(x, y)
	tt.Move(x+48, y+32)
	tt.Release(x+48, y+32)
	nx, ny := inkCenter(tt.Image(), hot)
	if nx-x < 40 || ny-y < 25 || inkCount(tt.Image(), hot) > 70 {
		t.Fatalf("hit/drag failed or placed a duplicate: (%v,%v) -> (%v,%v)", x, y, nx, ny)
	}
	tt.Key(0, ui.KeyDelete)
	if inkCount(tt.Image(), hot) != 0 {
		t.Fatal("Delete left custom ink")
	}
	tt.ClickAt(x, y)
	if inkCount(tt.Image(), hot) < 20 {
		t.Fatal("second placement failed")
	}
	x, y = inkCenter(tt.Image(), hot)
	tt.RightClickAt(x, y)
	if inkCount(tt.Image(), hot) != 0 {
		t.Fatal("right-click hit did not delete the mark")
	}
	if err := tt.Click("Pan"); err != nil {
		t.Fatal(err)
	}
	tt.ClickAt(x, y)
	if a.chart.Tool() != kline.ToolPan || inkCount(tt.Image(), hot) != 0 {
		t.Fatal("Pan placed a mark")
	}
}

func TestHeadlessSmokeUsesNativeView(t *testing.T) {
	a := newExtendExample()
	t.Cleanup(func() { _ = a.chart.Close() })
	var out bytes.Buffer
	if err := a.smoke(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "header shows MID: true") || a.chart.IndicatorOn("MID") {
		t.Fatalf("smoke: %s", out.String())
	}
}

func inkCount(img *image.RGBA, color ui.Color) int {
	n := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			p := img.RGBAAt(x, y)
			if near(p.R, color.R) && near(p.G, color.G) && near(p.B, color.B) {
				n++
			}
		}
	}
	return n
}

func inkCenter(img *image.RGBA, color ui.Color) (float32, float32) {
	xs, ys, n := 0, 0, 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			p := img.RGBAAt(x, y)
			if near(p.R, color.R) && near(p.G, color.G) && near(p.B, color.B) {
				xs += x
				ys += y
				n++
			}
		}
	}
	if n == 0 {
		return -1, -1
	}
	return float32(xs) / float32(n), float32(ys) / float32(n)
}

func near(a, b uint8) bool { return int(a)-int(b) <= 8 && int(b)-int(a) <= 8 }

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
