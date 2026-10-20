package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func TestDemoToolbarBindsChart(t *testing.T) {
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     kline.Period{Span: 1, Unit: kline.UnitHour},
		Layout:     kline.LayoutStack,
		Indicators: []string{"MA", "MACD"},
	})
	tt := ui.NewTester(func(c *ui.Context) { demoView(c, chart) }, 1100, 1000)
	for _, text := range []string{"K-line", "600 bars", "Period 1h", "MA", "MACD", "BOLL", "KDJ", "Hollow", "Classic", "US", "Contrast", "Limit 10%", "1h", "4h", "1d", "Stack", "Overlay", "Pan", "H-Line", "Segment", "Fib", "Magnet Off", "Magnet Weak", "Magnet Strong", "Reset view"} {
		if !tt.HasText(text) {
			t.Fatalf("toolbar missing %q, texts %q", text, tt.Texts())
		}
	}
	if _, ok := tt.Find("ALPHA chart"); !ok {
		t.Fatal("demo should show the stack chart under the toolbar")
	}
	if err := tt.Click("4h"); err != nil {
		t.Fatal(err)
	}
	if chart.Period().String() != "4h" {
		t.Fatalf("period %s", chart.Period())
	}
	if err := tt.Click("MACD"); err != nil {
		t.Fatal(err)
	}
	if chart.IndicatorOn("MACD") {
		t.Fatal("MACD toggle should turn the indicator off")
	}
	if err := tt.Click("Overlay"); err != nil {
		t.Fatal(err)
	}
	if chart.Layout() != kline.LayoutOverlay {
		t.Fatalf("layout %s", chart.Layout())
	}
	if _, ok := tt.Find("Compare chart"); !ok {
		t.Fatal("overlay click should show one compare plot")
	}
	if err := tt.Click("Hollow"); err != nil {
		t.Fatal(err)
	}
	if !chart.Hollow() {
		t.Fatal("Hollow toggle did not stick")
	}
	if err := tt.Click("H-Line"); err != nil {
		t.Fatal(err)
	}
	if chart.Tool() != kline.ToolHLine {
		t.Fatalf("tool %s", chart.Tool())
	}
	if err := tt.Click("Magnet Strong"); err != nil {
		t.Fatal(err)
	}
	if chart.Magnet() != kline.MagnetStrong {
		t.Fatalf("magnet %v", chart.Magnet())
	}
	chart.SetHollow(false)
	tt.Frame()
	if chart.Hollow() {
		t.Fatal("frame rebuilt a hollow flag the host cleared")
	}
	if err := tt.Click("US"); err != nil {
		t.Fatal(err)
	}
	us := chart.Style(nil)
	if us.Up != ui.RGB(239, 83, 80) || us.Down != ui.RGB(38, 166, 154) || us.Buy != us.Up || us.Sell != us.Down {
		t.Fatalf("US preset up %+v down %+v buy %+v sell %+v", us.Up, us.Down, us.Buy, us.Sell)
	}
	if err := tt.Click("Limit 10%"); err != nil {
		t.Fatal(err)
	}
	if chart.LimitPct() != 0.10 {
		t.Fatalf("limit %v", chart.LimitPct())
	}
	if chart.Hollow() {
		t.Fatal("color preset and limit percent should leave hollow off")
	}
	if err := tt.Click("Limit 10%"); err != nil {
		t.Fatal(err)
	}
	if chart.LimitPct() != 0 {
		t.Fatalf("second click should clear the limit, got %v", chart.LimitPct())
	}
	if err := tt.Click("Classic"); err != nil {
		t.Fatal(err)
	}
	classic := chart.Style(nil)
	if classic.Up != ui.RGB(38, 166, 154) || classic.Buy != classic.Up || classic.Sell != classic.Down {
		t.Fatalf("classic up %+v buy %+v sell %+v", classic.Up, classic.Buy, classic.Sell)
	}
	if err := tt.Click("Reset view"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Stack"); err != nil {
		t.Fatal(err)
	}
	if chart.Layout() != kline.LayoutStack {
		t.Fatalf("layout %s", chart.Layout())
	}
}

func TestRenderDemoFrame(t *testing.T) {
	dir := os.Getenv("KLINE_FRAMES")
	if dir == "" {
		t.Skip("set KLINE_FRAMES to write png frames")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	chart := kline.NewChart(kline.ChartOptions{})
	tt := ui.NewTester(func(c *ui.Context) { demoView(c, chart) }, 1100, 1000)
	writeDemoPNG(t, filepath.Join(dir, "p8-demo-toolbar.png"), tt.Image())
	if err := tt.Click("US"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Contrast"); err != nil {
		t.Fatal(err)
	}
	writeDemoPNG(t, filepath.Join(dir, "p8-demo-contrast.png"), tt.Image())
	contrast := chart.Style(nil)
	if contrast.Up != ui.RGB(255, 214, 0) || contrast.Buy != contrast.Up || contrast.Sell != contrast.Down {
		t.Fatalf("contrast up %+v buy %+v sell %+v", contrast.Up, contrast.Buy, contrast.Sell)
	}
}

func writeDemoPNG(t *testing.T, path string, img *image.RGBA) {
	t.Helper()
	if img == nil {
		t.Fatal("nil frame")
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
