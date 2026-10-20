package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
	"time"

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

func TestPeriodSwitchReloadsBothPanes(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1100, 900)
	if !tt.HasText("Period 1h") || !tt.HasText("1h") || !tt.HasText("4h") || !tt.HasText("1d") {
		t.Fatalf("toolbar texts %q", tt.Texts())
	}
	if err := tt.Click("4h"); err != nil {
		t.Fatal(err)
	}
	assertStep(t, a, 4*time.Hour)
	if !tt.HasText("Period 4h") {
		t.Fatalf("texts %q", tt.Texts())
	}
	assertFitRight(t, a)
	if err := tt.Click("1d"); err != nil {
		t.Fatal(err)
	}
	assertStep(t, a, 24*time.Hour)
	assertFitRight(t, a)
	if a.panes[0].candles[0].Time != a.panes[1].candles[0].Time {
		t.Fatal("panes drifted apart after the period switch")
	}

	a.vp.Offset = 40
	tt.Frame()
	if err := tt.Click("Reset view"); err != nil {
		t.Fatal(err)
	}
	if a.vp.BarWidth != kline.DefaultBarWidth {
		t.Fatalf("reset width %v", a.vp.BarWidth)
	}
	assertFitRight(t, a)
}

func TestForwardPrependKeepsScreenIndex(t *testing.T) {
	a := newApp()
	a.fitted = true
	a.vp.Offset = 2
	const watch = 20
	want := float64(watch) - a.vp.Offset
	times := []time.Time{a.panes[0].candles[watch].Time, a.panes[1].candles[watch].Time}
	before := len(a.panes[0].candles)
	a.ensureForward()
	if len(a.panes[0].candles) != before+forwardBatch || len(a.panes[1].candles) != before+forwardBatch {
		t.Fatalf("lengths %d %d, before %d", len(a.panes[0].candles), len(a.panes[1].candles), before)
	}
	for i, s := range a.panes {
		idx := -1
		for j, c := range s.candles {
			if c.Time.Equal(times[i]) {
				idx = j
				break
			}
		}
		if idx < 0 {
			t.Fatalf("pane %d lost %s", i, times[i])
		}
		if math.Abs(float64(idx)-a.vp.Offset-want) > 1e-9 {
			t.Fatalf("pane %d screen %v, want %v", i, float64(idx)-a.vp.Offset, want)
		}
	}
	n := len(a.panes[0].candles)
	a.ensureForward()
	if len(a.panes[0].candles) != n {
		t.Fatal("second call loaded again after the offset left the edge")
	}
}

func TestFrameTriggersForward(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 960, 720)
	a.vp.Offset = 1
	before := len(a.panes[0].candles)
	watched := a.panes[0].candles[9].Time
	screen := float64(9) - 1
	tt.Frame()
	if len(a.panes[0].candles) <= before {
		t.Fatal("frame did not prepend")
	}
	idx := -1
	for i, c := range a.panes[0].candles {
		if c.Time.Equal(watched) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("lost watched bar")
	}
	if math.Abs(float64(idx)-a.vp.Offset-screen) > 1e-9 {
		t.Fatalf("screen %v, want %v", float64(idx)-a.vp.Offset, screen)
	}
	if !tt.HasText(fmt.Sprintf("%d bars", a.bars())) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestForwardSkipsWhileLoading(t *testing.T) {
	a := newApp()
	a.fitted = true
	calls := 0
	inner := a.src
	a.src = sourceFunc(func(req kline.BarRequest) ([]kline.Candle, bool, error) {
		calls++
		return inner.GetBars(req)
	})
	a.loading = true
	a.vp.Offset = 1
	before := len(a.panes[0].candles)
	a.ensureForward()
	if calls != 0 || len(a.panes[0].candles) != before {
		t.Fatalf("in-flight load calls %d len %d", calls, len(a.panes[0].candles))
	}
}

func TestForwardDoesNotReenter(t *testing.T) {
	a := newApp()
	a.fitted = true
	a.vp.Offset = 1
	src := &reenterSource{inner: a.src, app: a}
	a.src = src
	a.ensureForward()
	if src.forwards != len(a.panes) {
		t.Fatalf("forward calls %d, want %d", src.forwards, len(a.panes))
	}
}

func TestIndicatorsAndMACDPane(t *testing.T) {
	const viewW, viewH = 1100, 980
	a := newApp()
	tt := ui.NewTester(a.view, viewW, viewH)
	if !tt.HasText("MA") || !tt.HasText("MACD") {
		t.Fatalf("toolbar texts %q", tt.Texts())
	}
	if len(a.painted) != 2 {
		t.Fatalf("painted panes %d", len(a.painted))
	}
	if a.painted[0].Time.H != 0 {
		t.Fatal("the upper symbol should leave the time axis to the bottom one")
	}
	if a.painted[1].Time.H <= 0 || a.painted[1].Time.Y < a.painted[1].MACD.Y+a.painted[1].MACD.H-1 {
		t.Fatalf("time axis %+v macd %+v", a.painted[1].Time, a.painted[1].MACD)
	}
	for i, panes := range a.painted {
		if panes.Price.H <= 1 || panes.Volume.H <= 1 || panes.MACD.H <= 1 {
			t.Fatalf("pane %d empty band %+v", i, panes)
		}
		if panes.Volume.Y < panes.Price.Y+panes.Price.H || panes.MACD.Y < panes.Volume.Y+panes.Volume.H {
			t.Fatalf("pane %d order price/volume/MACD %+v", i, panes)
		}
	}
	last := a.readout(0)
	for _, part := range []string{"MA5", "MA20", "DIF", "DEA", "HIST"} {
		if !strings.Contains(last, part) {
			t.Fatalf("readout missing %s: %q", part, last)
		}
	}

	macd := a.painted[0].MACD
	tt.Move(macd.X+90, macd.Y+macd.H/2)
	if a.crossPane != 0 || a.crossIndex < 0 {
		t.Fatalf("hover in the MACD band index %d pane %d", a.crossIndex, a.crossPane)
	}
	if !strings.Contains(a.readout(0), "Hover") || !strings.Contains(a.readout(0), "DIF") || !strings.Contains(a.readout(1), "DIF") {
		t.Fatalf("linked MACD readout\n%s\n%s", a.readout(0), a.readout(1))
	}
	assertCrosshairThroughMACD(t, tt.Image(), viewW, viewH, a, 0)
	assertCrosshairThroughMACD(t, tt.Image(), viewW, viewH, a, 1)

	if err := tt.Click("MACD"); err != nil {
		t.Fatal(err)
	}
	if a.painted[0].MACD.H != 0 || a.painted[1].MACD.H != 0 {
		t.Fatalf("MACD off still has a sub-pane %+v %+v", a.painted[0].MACD, a.painted[1].MACD)
	}
	if strings.Contains(a.readout(0), "DIF") || strings.Contains(a.readout(1), "HIST") {
		t.Fatalf("MACD readout stayed\n%s\n%s", a.readout(0), a.readout(1))
	}
	if !strings.Contains(a.readout(0), "MA5") {
		t.Fatalf("MA should stay on: %q", a.readout(0))
	}
	if err := tt.Click("MA"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(a.readout(0), "MA5") || strings.Contains(a.readout(0), "MA20") {
		t.Fatalf("MA readout stayed %q", a.readout(0))
	}
	if err := tt.Click("MACD"); err != nil {
		t.Fatal(err)
	}
	if a.painted[0].MACD.H <= 1 || strings.Contains(a.readout(0), "MA5") || !strings.Contains(a.readout(0), "DIF") {
		t.Fatalf("MACD back on, MA still off: h %v readout %q", a.painted[0].MACD.H, a.readout(0))
	}
}

func assertCrosshairThroughMACD(t *testing.T, img *image.RGBA, viewW, viewH int, a *app, pane int) {
	t.Helper()
	if img == nil {
		t.Fatal("nil frame")
	}
	band := a.painted[pane].MACD
	xDIP := a.painted[pane].Price.X + float32(a.vp.BarLeft(a.crossIndex)+a.vp.BarWidth/2)
	b := img.Bounds()
	sx := float32(b.Dx()) / float32(viewW)
	sy := float32(b.Dy()) / float32(viewH)
	x := int(xDIP*sx + 0.5)
	y0 := int(band.Y * sy)
	y1 := int((band.Y + band.H) * sy)
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	// Match the crosshair color, not the plot fill. Light and dark themes
	// use different muted grays; either one counts.
	muted := []ui.Color{ui.LightTheme().TextMuted, ui.DarkTheme().TextMuted}
	hits := 0
	rows := y1 - y0
	for y := y0; y < y1; y++ {
		for dx := -1; dx <= 1; dx++ {
			px := x + dx
			if px < b.Min.X || px >= b.Max.X {
				continue
			}
			c := img.RGBAAt(px, y)
			if nearColor(c, muted) {
				hits++
				break
			}
		}
	}
	if rows < 8 || hits*2 < rows {
		t.Fatalf("pane %d vertical crosshair should cross the MACD band: hits %d of %d rows at x %d, band %+v", pane, hits, rows, x, band)
	}
}

func nearColor(c color.RGBA, targets []ui.Color) bool {
	for _, m := range targets {
		dr := int(c.R) - int(m.R)
		dg := int(c.G) - int(m.G)
		db := int(c.B) - int(m.B)
		if dr*dr+dg*dg+db*db <= 24*24 {
			return true
		}
	}
	return false
}

func TestRenderFrames(t *testing.T) {
	dir := os.Getenv("KLINE_FRAMES")
	if dir == "" {
		t.Skip("set KLINE_FRAMES to write png frames")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	tt := ui.NewTester(a.view, 1100, 1000)
	writePNG(t, dir+"/p1-macd.png", tt.Image())
	if len(a.painted) < 2 || a.painted[0].MACD.H <= 1 {
		t.Fatal("frame has no MACD band")
	}
	macd := a.painted[0].MACD
	tt.Move(macd.X+120, macd.Y+macd.H/2)
	writePNG(t, dir+"/p1-crosshair.png", tt.Image())
	if err := tt.Click("1d"); err != nil {
		t.Fatal(err)
	}
	writePNG(t, dir+"/p1-1d.png", tt.Image())
}

func assertStep(t *testing.T, a *app, step time.Duration) {
	t.Helper()
	if len(a.panes) != 2 {
		t.Fatalf("panes %d", len(a.panes))
	}
	for _, s := range a.panes {
		if len(s.candles) < 2 {
			t.Fatalf("%s len %d", s.name, len(s.candles))
		}
		if got := s.candles[1].Time.Sub(s.candles[0].Time); got != step {
			t.Fatalf("%s step %s, want %s", s.name, got, step)
		}
	}
}

func assertFitRight(t *testing.T, a *app) {
	t.Helper()
	if !a.fitted || a.plotW <= 0 {
		t.Fatalf("fitted %v plotW %v", a.fitted, a.plotW)
	}
	v := kline.Viewport{BarWidth: a.vp.BarWidth}
	v.FitRight(a.bars(), a.plotW)
	if math.Abs(a.vp.Offset-v.Offset) > 1e-6 {
		t.Fatalf("offset %v, fit %v", a.vp.Offset, v.Offset)
	}
}

type sourceFunc func(kline.BarRequest) ([]kline.Candle, bool, error)

func (f sourceFunc) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	return f(req)
}

type reenterSource struct {
	inner    kline.DataSource
	app      *app
	forwards int
}

func (r *reenterSource) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	if req.Kind == kline.KindForward {
		r.forwards++
		if r.forwards > 8 {
			return nil, false, fmt.Errorf("forward reentered")
		}
		r.app.ensureForward()
	}
	return r.inner.GetBars(req)
}

func writePNG(t *testing.T, path string, img *image.RGBA) {
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

func mustBox(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing %q, texts %q", label, tt.Texts())
	}
	return r
}
