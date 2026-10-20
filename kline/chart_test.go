package kline

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
)

func newApp() *Chart {
	return NewChart(ChartOptions{})
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// press drives a host toolbar control and paints the next frame. Chart.View
// has no toolbar; the demo window binds these same calls.
func press(t *testing.T, tt *ui.Tester, a *Chart, label string) {
	t.Helper()
	switch label {
	case "1h":
		if err := a.SetPeriod(Period{Span: 1, Unit: UnitHour}); err != nil {
			t.Fatal(err)
		}
	case "4h":
		if err := a.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
			t.Fatal(err)
		}
	case "1d":
		if err := a.SetPeriod(Period{Span: 1, Unit: UnitDay}); err != nil {
			t.Fatal(err)
		}
	case "MA", "MACD", "BOLL", "KDJ":
		if err := a.SetIndicator(label, !a.IndicatorOn(label)); err != nil {
			t.Fatal(err)
		}
	case "Overlay":
		a.SetLayout(LayoutOverlay)
	case "Stack":
		a.SetLayout(LayoutStack)
	case "Pan":
		if err := a.SetTool(ToolPan); err != nil {
			t.Fatal(err)
		}
	case "H-Line":
		if err := a.SetTool(ToolHLine); err != nil {
			t.Fatal(err)
		}
	case "Segment":
		if err := a.SetTool(ToolSegment); err != nil {
			t.Fatal(err)
		}
	case "Fib":
		if err := a.SetTool(ToolFib); err != nil {
			t.Fatal(err)
		}
	case "Hollow":
		a.SetHollow(!a.Hollow())
	case "Magnet Off":
		a.SetMagnet(MagnetOff)
	case "Magnet Weak":
		a.SetMagnet(MagnetWeak)
	case "Magnet Strong":
		a.SetMagnet(MagnetStrong)
	case "Reset view":
		a.ResetView()
	default:
		t.Fatalf("unknown control %q", label)
	}
	tt.Frame()
}

func TestViewShowsLatestBar(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 960, 720)
	last := a.panes[0].candles[len(a.panes[0].candles)-1]
	if a.Bars() != 600 || !tt.HasText("ALPHA") || !tt.HasText("BETA") {
		t.Fatalf("bars %d texts %q", a.Bars(), tt.Texts())
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
	tt := ui.NewTester(a.View, 960, 720)
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
	_ = ui.NewTester(fresh.View, 960, 720)
	press(t, tt, a, "Reset view")
	if math.Abs(a.vp.Offset-fresh.vp.Offset) > 0.5 || math.Abs(a.vp.BarWidth-fresh.vp.BarWidth) > 0.5 {
		t.Fatalf("reset offset %v width %v, fresh offset %v width %v", a.vp.Offset, a.vp.BarWidth, fresh.vp.Offset, fresh.vp.BarWidth)
	}
}

func TestPanesShareCrosshair(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 960, 720)
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

func assertSameBar(t *testing.T, tt *ui.Tester, a *Chart, idx int) {
	t.Helper()
	if a.panes[0].candles[idx].Time != a.panes[1].candles[idx].Time {
		t.Fatalf("bar %d times differ: %s vs %s", idx, a.panes[0].candles[idx].Time, a.panes[1].candles[idx].Time)
	}
	r0, r1 := a.readout(0), a.readout(1)
	when := a.panes[0].candles[idx].Time.UTC().Format("2006-01-02 15:04")
	if !strings.Contains(r0, "Hover") || !strings.Contains(r1, "Hover") || !strings.Contains(r0, when) || !strings.Contains(r1, when) {
		t.Fatalf("linked readouts:\n%s\n%s", r0, r1)
	}
	p0 := FormatPrice(a.panes[0].candles[idx].Close)
	p1 := FormatPrice(a.panes[1].candles[idx].Close)
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
	tt := ui.NewTester(a.View, 1100, 900)
	if a.Period().String() != "1h" {
		t.Fatalf("period %s", a.Period())
	}
	press(t, tt, a, "4h")
	assertStep(t, a, 4*time.Hour)
	if a.Period().String() != "4h" {
		t.Fatalf("period %s", a.Period())
	}
	assertFitRight(t, a)
	press(t, tt, a, "1d")
	assertStep(t, a, 24*time.Hour)
	assertFitRight(t, a)
	if a.panes[0].candles[0].Time != a.panes[1].candles[0].Time {
		t.Fatal("panes drifted apart after the period switch")
	}

	a.vp.Offset = 40
	tt.Frame()
	press(t, tt, a, "Reset view")
	if a.vp.BarWidth != DefaultBarWidth {
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
	tt := ui.NewTester(a.View, 960, 720)
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
	if !tt.HasText("ALPHA") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func TestForwardSkipsWhileLoading(t *testing.T) {
	a := newApp()
	a.fitted = true
	calls := 0
	inner := a.src
	a.src = sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
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
	tt := ui.NewTester(a.View, viewW, viewH)
	if !a.IndicatorOn("MA") || !a.IndicatorOn("MACD") {
		t.Fatal("MA and MACD start on")
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

	press(t, tt, a, "MACD")
	if a.painted[0].MACD.H != 0 || a.painted[1].MACD.H != 0 {
		t.Fatalf("MACD off still has a sub-pane %+v %+v", a.painted[0].MACD, a.painted[1].MACD)
	}
	if strings.Contains(a.readout(0), "DIF") || strings.Contains(a.readout(1), "HIST") {
		t.Fatalf("MACD readout stayed\n%s\n%s", a.readout(0), a.readout(1))
	}
	if !strings.Contains(a.readout(0), "MA5") {
		t.Fatalf("MA should stay on: %q", a.readout(0))
	}
	press(t, tt, a, "MA")
	if strings.Contains(a.readout(0), "MA5") || strings.Contains(a.readout(0), "MA20") {
		t.Fatalf("MA readout stayed %q", a.readout(0))
	}
	press(t, tt, a, "MACD")
	if a.painted[0].MACD.H <= 1 || strings.Contains(a.readout(0), "MA5") || !strings.Contains(a.readout(0), "DIF") {
		t.Fatalf("MACD back on, MA still off: h %v readout %q", a.painted[0].MACD.H, a.readout(0))
	}
}

func TestBOLLAndKDJStack(t *testing.T) {
	const viewW, viewH = 1100, 1000
	a := newApp()
	tt := ui.NewTester(a.View, viewW, viewH)
	if !containsName(a.IndicatorNames(), "BOLL") || !containsName(a.IndicatorNames(), "KDJ") {
		t.Fatalf("indicators %v", a.IndicatorNames())
	}
	if a.IndicatorOn("BOLL") || a.IndicatorOn("KDJ") {
		t.Fatal("BOLL and KDJ start off")
	}
	for _, panes := range a.painted {
		if panes.KDJ.H != 0 {
			t.Fatalf("KDJ starts off: %+v", panes.KDJ)
		}
	}
	if strings.Contains(a.readout(0), "MID ") || strings.Contains(a.readout(0), "K ") {
		t.Fatalf("BOLL and KDJ readouts start off: %q", a.readout(0))
	}
	press(t, tt, a, "BOLL")
	press(t, tt, a, "KDJ")
	if len(a.painted) != 2 {
		t.Fatalf("painted panes %d", len(a.painted))
	}
	for i, panes := range a.painted {
		if panes.Price.H <= 1 || panes.Volume.H <= 1 || panes.MACD.H <= 1 || panes.KDJ.H <= 1 {
			t.Fatalf("pane %d empty band %+v", i, panes)
		}
		if panes.Volume.Y < panes.Price.Y+panes.Price.H || panes.MACD.Y < panes.Volume.Y+panes.Volume.H {
			t.Fatalf("pane %d price/volume/MACD %+v", i, panes)
		}
		if panes.KDJ.Y < panes.MACD.Y+panes.MACD.H {
			t.Fatalf("pane %d KDJ should sit under MACD %+v", i, panes)
		}
	}
	if a.painted[0].Time.H != 0 {
		t.Fatal("the upper symbol should leave the time axis to the bottom one")
	}
	bottom := a.painted[1]
	if bottom.Time.H <= 0 || bottom.Time.Y < bottom.KDJ.Y+bottom.KDJ.H-1 {
		t.Fatalf("time axis %+v kdj %+v", bottom.Time, bottom.KDJ)
	}
	for _, part := range []string{"MID ", "UP ", "LOW ", "K ", "D ", "J "} {
		if !strings.Contains(a.readout(0), part) || !strings.Contains(a.readout(1), part) {
			t.Fatalf("readout missing %s\n%s\n%s", part, a.readout(0), a.readout(1))
		}
	}
	kdj := a.painted[0].KDJ
	tt.Move(kdj.X+90, kdj.Y+kdj.H/2)
	if a.crossPane != 0 || a.crossIndex < 0 {
		t.Fatalf("hover in the KDJ band index %d pane %d", a.crossIndex, a.crossPane)
	}
	if !strings.Contains(a.readout(0), "Hover") || !strings.Contains(a.readout(1), "K ") {
		t.Fatalf("linked KDJ readout\n%s\n%s", a.readout(0), a.readout(1))
	}
	assertCrosshairThroughMACD(t, tt.Image(), viewW, viewH, a, 0)

	press(t, tt, a, "KDJ")
	if a.painted[0].KDJ.H != 0 || strings.Contains(a.readout(0), "K ") {
		t.Fatalf("KDJ off still open: h %v readout %q", a.painted[0].KDJ.H, a.readout(0))
	}
	if !strings.Contains(a.readout(0), "MID ") || a.painted[0].MACD.H <= 1 {
		t.Fatalf("BOLL and MACD should stay: h %v readout %q", a.painted[0].MACD.H, a.readout(0))
	}
	if a.painted[1].Time.Y < a.painted[1].MACD.Y+a.painted[1].MACD.H-1 {
		t.Fatalf("time should return under MACD: %+v", a.painted[1])
	}

	press(t, tt, a, "Overlay")
	press(t, tt, a, "KDJ")
	if len(a.painted) != 1 || a.painted[0].KDJ.H <= 1 || a.painted[0].KDJ.Y < a.painted[0].MACD.Y+a.painted[0].MACD.H {
		t.Fatalf("overlay should keep one KDJ band under MACD: %+v", a.painted[0])
	}
	if !strings.Contains(a.readout(0), "MID ") || !strings.Contains(a.readout(0), "K ") {
		t.Fatalf("primary readout %q", a.readout(0))
	}
	if strings.Contains(a.readout(1), "MID ") || strings.Contains(a.readout(1), "K ") {
		t.Fatalf("compare readout should omit BOLL and KDJ: %q", a.readout(1))
	}
}

func assertCrosshairThroughMACD(t *testing.T, img *image.RGBA, viewW, viewH int, a *Chart, pane int) {
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

func TestHLinePlaceDragAndDelete(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	if a.Tool() != ToolPan {
		t.Fatalf("tool %s", a.Tool())
	}
	press(t, tt, a, "H-Line")
	if a.tool != toolHLine {
		t.Fatalf("tool %d", a.tool)
	}
	price := a.painted[0].Price
	x := price.X + 160
	y := price.Y + price.H*0.35
	startOff := a.vp.Offset
	tt.Press(x, y)
	tt.Move(x+80, y)
	tt.Release(x+80, y)
	if math.Abs(a.vp.Offset-startOff) > 0.01 {
		t.Fatalf("draw mode panned: offset %v, start %v", a.vp.Offset, startOff)
	}
	if n := len(a.panes[0].overlays.Items()); n != 1 {
		t.Fatalf("alpha overlays %d", n)
	}
	if len(a.panes[1].overlays.Items()) != 0 {
		t.Fatal("hline should stay on the pane that was clicked")
	}
	line := a.panes[0].overlays.Items()[0]
	if line.Kind != OverlayHLine || !line.Selected || len(line.Points) != 1 {
		t.Fatalf("line %+v", line)
	}
	placed := line.Points[0].Price

	y2 := price.Y + price.H*0.7
	tt.Press(x, y)
	tt.Move(x, y2)
	tt.Release(x, y2)
	moved := a.panes[0].overlays.Items()[0].Points[0].Price
	if moved >= placed-0.05 {
		t.Fatalf("dragging down should lower the price: %v -> %v", placed, moved)
	}

	tt.Key(0, ui.KeyDelete)
	if len(a.panes[0].overlays.Items()) != 0 {
		t.Fatal("delete should remove the selected hline")
	}

	press(t, tt, a, "Pan")
	tt.Press(x, y)
	tt.Move(x+100, y)
	tt.Release(x+100, y)
	if a.vp.Offset >= startOff-1 {
		t.Fatalf("pan tool should pan, offset %v start %v", a.vp.Offset, startOff)
	}
}

func TestSegmentTwoClickPerPane(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "Segment")
	p0 := a.painted[0].Price
	p1 := a.painted[1].Price
	tt.ClickAt(p0.X+120, p0.Y+p0.H*0.3)
	if !a.draftOn || a.draftPane != 0 {
		t.Fatalf("draft on %v pane %d", a.draftOn, a.draftPane)
	}
	if len(a.panes[0].overlays.Items()) != 0 {
		t.Fatal("first click should leave the segment unfinished")
	}
	x1 := p0.X + 280
	y1 := p0.Y + p0.H*0.7
	tt.Move(x1, y1)
	if a.crossPane != 0 {
		t.Fatalf("rubber-band hover pane %d", a.crossPane)
	}
	tt.ClickAt(x1, y1)
	if a.draftOn {
		t.Fatal("second click should finish the segment")
	}
	items := a.panes[0].overlays.Items()
	if len(items) != 1 || items[0].Kind != OverlaySegment || len(items[0].Points) != 2 || !items[0].Selected {
		t.Fatalf("segment %+v", items)
	}
	if items[0].Points[0].BarIndex == items[0].Points[1].BarIndex {
		t.Fatal("segment endpoints should sit on different bars")
	}
	if len(a.panes[1].overlays.Items()) != 0 {
		t.Fatal("segment landed on the other pane")
	}

	tt.ClickAt(p1.X+100, p1.Y+p1.H*0.4)
	tt.ClickAt(p1.X+220, p1.Y+p1.H*0.6)
	if len(a.panes[0].overlays.Items()) != 1 || len(a.panes[1].overlays.Items()) != 1 {
		t.Fatalf("per-pane counts %d %d", len(a.panes[0].overlays.Items()), len(a.panes[1].overlays.Items()))
	}

	// The segment runs between the two click prices. Its X is the bar
	// center, within half a bar of the click, so the midpoint still hits.
	tt.RightClickAt(p1.X+160, p1.Y+p1.H*0.5)
	if len(a.panes[1].overlays.Items()) != 0 {
		t.Fatal("right-click should delete the segment on that pane")
	}
	if len(a.panes[0].overlays.Items()) != 1 {
		t.Fatal("right-click on beta should leave alpha's segment")
	}
}

func TestFibToolSelects(t *testing.T) {
	a := newApp()
	_ = ui.NewTester(a.View, 1100, 1000)
	if err := a.SetTool(ToolFib); err != nil {
		t.Fatal(err)
	}
	if a.Tool() != ToolFib {
		t.Fatalf("tool %s", a.Tool())
	}
	if err := a.SetTool("nope"); err == nil {
		t.Fatal("unknown tool should error")
	}
	if a.Tool() != ToolFib {
		t.Fatal("unknown tool changed the selection")
	}
}

func TestFibTwoClickPerPane(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "Fib")
	p0 := a.painted[0].Price
	p1 := a.painted[1].Price
	tt.ClickAt(p0.X+120, p0.Y+p0.H*0.25)
	if !a.draftOn || a.draftPane != 0 {
		t.Fatalf("draft on %v pane %d", a.draftOn, a.draftPane)
	}
	if len(a.panes[0].overlays.Items()) != 0 {
		t.Fatal("first click should leave fib unfinished")
	}
	x1 := p0.X + 300
	y1 := p0.Y + p0.H*0.75
	tt.Move(x1, y1)
	tt.ClickAt(x1, y1)
	if a.draftOn {
		t.Fatal("second click should finish fib")
	}
	items := a.panes[0].overlays.Items()
	if len(items) != 1 || items[0].Kind != OverlayFibonacci || len(items[0].Points) != 2 || !items[0].Selected {
		t.Fatalf("fib %+v", items)
	}
	if len(a.panes[1].overlays.Items()) != 0 {
		t.Fatal("fib landed on the other pane")
	}

	tt.ClickAt(p1.X+100, p1.Y+p1.H*0.35)
	tt.ClickAt(p1.X+240, p1.Y+p1.H*0.65)
	if len(a.panes[0].overlays.Items()) != 1 || len(a.panes[1].overlays.Items()) != 1 {
		t.Fatalf("per-pane counts %d %d", len(a.panes[0].overlays.Items()), len(a.panes[1].overlays.Items()))
	}

	midY := p1.Y + p1.H*0.5
	tt.RightClickAt(p1.X+170, midY)
	if len(a.panes[1].overlays.Items()) != 0 {
		t.Fatal("right-click should delete fib on that pane")
	}
	if len(a.panes[0].overlays.Items()) != 1 {
		t.Fatal("right-click on beta should leave alpha fib")
	}

	mid := p0.Y + p0.H*0.5
	tt.ClickAt(p0.X+200, mid)
	tt.Key(0, ui.KeyDelete)
	if len(a.panes[0].overlays.Items()) != 0 {
		t.Fatal("delete should remove selected fib")
	}
}

func TestFibEndpointDrag(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "Fib")
	p0 := a.painted[0].Price
	tt.ClickAt(p0.X+100, p0.Y+p0.H*0.3)
	tt.ClickAt(p0.X+280, p0.Y+p0.H*0.7)
	fib := a.panes[0].overlays.Items()[0]
	other := fib.Points[1]
	end := fib.Points[0]
	scale := MainScale(a.panes[0].candles, a.active(a.panes[0]), a.vp, a.plotW)
	winX := p0.X + float32(AnchorX(end, a.vp, a.panes[0].candles))
	winY := p0.Y + float32(PriceToLocalY(float64(p0.H), end.Price, scale))
	tt.Press(winX, winY)
	tt.Move(winX+40, p0.Y+p0.H*0.85)
	tt.Release(winX+40, p0.Y+p0.H*0.85)
	got := a.panes[0].overlays.Items()[0]
	if math.Abs(got.Points[1].Price-other.Price) > 1e-9 || got.Points[1].BarIndex != other.BarIndex {
		t.Fatalf("other endpoint moved: %+v was %+v", got.Points[1], other)
	}
	if got.Points[0].Price >= end.Price-0.05 {
		t.Fatalf("dragging endpoint down should lower price: %v -> %v", end.Price, got.Points[0].Price)
	}
}

func TestFibMagnetStrongSnapsEndpoints(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "Magnet Strong")
	press(t, tt, a, "Fib")
	p0 := a.painted[0].Price
	x0 := p0.X + 200
	y0 := p0.Y + p0.H*0.42
	plotX := float64(x0 - p0.X)
	idx, ok := a.vp.IndexAt(plotX, len(a.panes[0].candles))
	if !ok {
		t.Fatal("no bar under click")
	}
	bar := a.panes[0].candles[idx]
	scale := MainScale(a.panes[0].candles, a.active(a.panes[0]), a.vp, a.plotW)
	raw := LocalYToPrice(float64(p0.H), float64(y0-p0.Y), scale)
	if onOHLC(raw, bar, 1e-6) {
		y0 = p0.Y + p0.H*0.37
	}
	tt.ClickAt(x0, y0)
	x1 := p0.X + 320
	y1 := p0.Y + p0.H*0.68
	plotX1 := float64(x1 - p0.X)
	idx1, ok := a.vp.IndexAt(plotX1, len(a.panes[0].candles))
	if !ok {
		t.Fatal("no bar under second click")
	}
	bar1 := a.panes[0].candles[idx1]
	raw1 := LocalYToPrice(float64(p0.H), float64(y1-p0.Y), scale)
	if onOHLC(raw1, bar1, 1e-6) {
		y1 = p0.Y + p0.H*0.62
	}
	tt.ClickAt(x1, y1)
	fib := a.panes[0].overlays.Items()[0]
	if !onOHLC(fib.Points[0].Price, bar, 1e-4) {
		t.Fatalf("first endpoint %v not on OHLC %+v", fib.Points[0].Price, bar)
	}
	if !onOHLC(fib.Points[1].Price, bar1, 1e-4) {
		t.Fatalf("second endpoint %v not on OHLC %+v", fib.Points[1].Price, bar1)
	}
}

func TestSegmentEndpointDrag(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "Segment")
	p0 := a.painted[0].Price
	tt.ClickAt(p0.X+100, p0.Y+p0.H*0.3)
	tt.ClickAt(p0.X+260, p0.Y+p0.H*0.6)
	seg := a.panes[0].overlays.Items()[0]
	end := seg.Points[0]
	scale := MainScale(a.panes[0].candles, a.active(a.panes[0]), a.vp, a.plotW)
	winX := p0.X + float32(AnchorX(end, a.vp, a.panes[0].candles))
	winY := p0.Y + float32(PriceToLocalY(float64(p0.H), end.Price, scale))
	tt.Press(winX, winY)
	tt.Move(winX+40, p0.Y+p0.H*0.85)
	tt.Release(winX+40, p0.Y+p0.H*0.85)
	got := a.panes[0].overlays.Items()[0].Points[0]
	if got.Price >= end.Price-0.05 {
		t.Fatalf("dragging an endpoint down should lower its price: %v -> %v", end.Price, got.Price)
	}
	if got.BarIndex == end.BarIndex && math.Abs(got.Price-end.Price) < 0.05 {
		t.Fatal("endpoint did not move")
	}
}

func TestPeriodSwitchClearsOverlays(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "H-Line")
	price := a.painted[0].Price
	tt.ClickAt(price.X+120, price.Y+price.H*0.4)
	if len(a.panes[0].overlays.Items()) != 1 {
		t.Fatal("hline was not placed")
	}
	press(t, tt, a, "4h")
	if len(a.panes[0].overlays.Items()) != 0 || len(a.panes[1].overlays.Items()) != 0 {
		t.Fatal("reloading a period should drop drawings on both panes")
	}
}

func TestForwardKeepsOverlayOnItsBar(t *testing.T) {
	a := newApp()
	_ = ui.NewTester(a.View, 960, 720)
	cs := a.panes[0].candles
	when := cs[10].Time
	a.panes[0].overlays.Add(OverlayHLine, Point{BarIndex: 10, Time: when, Price: cs[10].Close})
	a.fitted = true
	a.vp.Offset = 2
	a.ensureForward()
	pt := a.panes[0].overlays.Items()[0].Points[0]
	idx, ok := pt.Index(a.panes[0].candles)
	if !ok || !a.panes[0].candles[idx].Time.Equal(when) {
		t.Fatalf("overlay left its bar: idx %d ok %v", idx, ok)
	}
}

func TestOverlayPixels(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "H-Line")
	price := a.painted[0].Price
	tt.ClickAt(price.X+140, price.Y+price.H*0.45)
	press(t, tt, a, "Segment")
	tt.ClickAt(price.X+90, price.Y+price.H*0.25)
	tt.ClickAt(price.X+300, price.Y+price.H*0.72)
	img := tt.Image()
	// Magenta is the unselected hline. It is not the MA accent, so the
	// count is the drawing and not the indicator.
	line := countNear(img, price, ui.RGB(217, 70, 239))
	hot := countNear(img, price, ui.RGB(255, 214, 0))
	if line < 30 {
		t.Fatalf("hline pixels %d in %+v", line, price)
	}
	if hot < 15 {
		t.Fatalf("selected segment pixels %d in %+v", hot, price)
	}
}

func countNear(img *image.RGBA, r ui.Rect, target ui.Color) int {
	if img == nil {
		return 0
	}
	b := img.Bounds()
	x0 := int(r.X)
	y0 := int(r.Y)
	x1 := int(r.X + r.W)
	y1 := int(r.Y + r.H)
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	n := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if nearColor(img.RGBAAt(x, y), []ui.Color{target}) {
				n++
			}
		}
	}
	return n
}

func TestOverlayLayoutShowsBothSymbolsOnOnePlot(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	if _, ok := tt.Find("ALPHA chart"); !ok {
		t.Fatal("stack mode should keep the ALPHA pane")
	}
	if _, ok := tt.Find("BETA chart"); !ok {
		t.Fatal("stack mode should keep the BETA pane")
	}
	press(t, tt, a, "Overlay")
	if a.layout != layoutOverlay {
		t.Fatalf("layout %d", a.layout)
	}
	if _, ok := tt.Find("ALPHA chart"); ok {
		t.Fatal("overlay mode should replace the two panes with one plot")
	}
	if _, ok := tt.Find("BETA chart"); ok {
		t.Fatal("overlay mode should not keep a second pane")
	}
	box := mustBox(t, tt, "Compare chart")
	if len(a.painted) != 1 {
		t.Fatalf("painted panes %d", len(a.painted))
	}
	panes := a.painted[0]
	if panes.Price.H < 80 || panes.Volume.H < 16 {
		t.Fatalf("overlay needs one price plot and one volume strip: %+v", panes)
	}
	if panes.MACD.H < 16 {
		t.Fatalf("overlay keeps the primary MACD strip: %+v", panes.MACD)
	}
	if panes.Volume.Y < panes.Price.Y+panes.Price.H || panes.MACD.Y < panes.Volume.Y+panes.Volume.H {
		t.Fatalf("bands out of order: %+v", panes)
	}
	if box.H < 400 {
		t.Fatalf("the compare plot should use the window, box %+v", box)
	}
	if a.Layout() != LayoutOverlay {
		t.Fatalf("layout %s", a.Layout())
	}

	// One volume strip and one MACD band: turning MACD off removes that
	// band and leaves the primary volume strip.
	press(t, tt, a, "MACD")
	if a.painted[0].MACD.H != 0 || a.painted[0].Volume.H < 16 {
		t.Fatalf("MACD off should leave primary volume only: %+v", a.painted[0])
	}
	press(t, tt, a, "MACD")

	img := tt.Image()
	line := countNear(img, a.painted[0].Price, StyleFrom(nil).Compare)
	if line < 40 {
		t.Fatalf("BETA close line pixels %d in %+v", line, a.painted[0].Price)
	}
	candles := countNear(img, a.painted[0].Price, StyleFrom(nil).Up) +
		countNear(img, a.painted[0].Price, StyleFrom(nil).Down)
	if candles < 40 {
		t.Fatalf("primary candles pixels %d", candles)
	}

	press(t, tt, a, "Stack")
	if _, ok := tt.Find("ALPHA chart"); !ok {
		t.Fatal("stack should restore ALPHA")
	}
	if _, ok := tt.Find("BETA chart"); !ok {
		t.Fatal("stack should restore BETA")
	}
}

func TestOverlayReadoutAndSharedViewport(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "Overlay")
	tt.Frame()
	first, last := a.vp.Visible(a.bars(), a.plotW)
	base := IndexBase(first, last, a.panes[0].candles, a.panes[1].candles)
	if base != first {
		t.Fatalf("synthetic series should index the first visible bar, base %d first %d", base, first)
	}
	if math.Abs(a.indexValue(0, base)-100) > 1e-6 || math.Abs(a.indexValue(1, base)-100) > 1e-6 {
		t.Fatalf("index at base: %v %v", a.indexValue(0, base), a.indexValue(1, base))
	}
	later := base + 8
	if later > last {
		t.Fatalf("visible window %d..%d is too short", first, last)
	}
	want0 := a.panes[0].candles[later].Close / a.panes[0].candles[base].Close * 100
	want1 := a.panes[1].candles[later].Close / a.panes[1].candles[base].Close * 100
	if math.Abs(a.indexValue(0, later)-want0) > 1e-6 || math.Abs(a.indexValue(1, later)-want1) > 1e-6 {
		t.Fatalf("later index %v %v, want %v %v", a.indexValue(0, later), a.indexValue(1, later), want0, want1)
	}

	price := a.painted[0].Price
	x := price.X + float32(a.vp.BarLeft(later)+a.vp.BarWidth/2)
	tt.Move(x, price.Y+price.H*0.4)
	if a.crossIndex != later {
		t.Fatalf("cross index %d, want %d", a.crossIndex, later)
	}
	r0, r1 := a.readout(0), a.readout(1)
	when := a.panes[0].candles[later].Time.UTC().Format("2006-01-02 15:04")
	if !strings.Contains(r0, "Hover") || !strings.Contains(r1, "Hover") || !strings.Contains(r0, when) || !strings.Contains(r1, when) {
		t.Fatalf("both readouts should follow the same bar:\n%s\n%s", r0, r1)
	}
	p0 := FormatPrice(a.panes[0].candles[later].Close)
	p1 := FormatPrice(a.panes[1].candles[later].Close)
	if !strings.Contains(r0, "C "+p0) || !strings.Contains(r1, "C "+p1) {
		t.Fatalf("each readout should show that symbol's close:\n%s\n%s", r0, r1)
	}
	if !strings.Contains(r0, "idx "+FormatPrice(want0)) || !strings.Contains(r1, "idx "+FormatPrice(want1)) {
		t.Fatalf("readouts should show the index:\n%s\n%s", r0, r1)
	}
	if !strings.Contains(r0, "DIF") || strings.Contains(r1, "DIF") {
		t.Fatalf("MACD stays on the primary readout:\n%s\n%s", r0, r1)
	}
	if !tt.HasText(r0) || !tt.HasText(r1) {
		t.Fatalf("headers missing both readouts, texts %q", tt.Texts())
	}

	offset := a.vp.Offset
	tt.Press(price.X+40, price.Y+30)
	tt.Move(price.X+160, price.Y+30)
	tt.Release(price.X+160, price.Y+30)
	if a.vp.Offset >= offset-1 {
		t.Fatalf("drag on the compare plot should pan the shared viewport: %v, was %v", a.vp.Offset, offset)
	}
	moved := a.vp.Offset
	press(t, tt, a, "Stack")
	if math.Abs(a.vp.Offset-moved) > 1e-9 {
		t.Fatalf("leaving overlay changed the viewport: %v, was %v", a.vp.Offset, moved)
	}

	// Panning to a later window rebases both series to 100 at the new
	// first visible bar.
	press(t, tt, a, "Overlay")
	tt.Frame()
	anchored, _ := a.vp.Visible(a.bars(), a.plotW)
	a.vp.Offset = float64(anchored + 40)
	a.vp.Clamp(a.bars(), a.plotW)
	tt.Frame()
	next, _ := a.vp.Visible(a.bars(), a.plotW)
	if next <= anchored {
		t.Fatalf("window did not move: %d then %d", anchored, next)
	}
	if math.Abs(a.indexValue(0, next)-100) > 1e-6 || math.Abs(a.indexValue(1, next)-100) > 1e-6 {
		t.Fatalf("rebased index %v %v at %d", a.indexValue(0, next), a.indexValue(1, next), next)
	}
	if math.Abs(a.indexValue(0, anchored)-100) < 0.05 {
		t.Fatalf("old base stayed at 100: %v", a.indexValue(0, anchored))
	}
}

func TestOverlayHLineStoresPrimaryPrice(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "Overlay")
	press(t, tt, a, "H-Line")
	price := a.painted[0].Price
	x := price.X + 180
	y := price.Y + price.H*0.4
	tt.ClickAt(x, y)
	items := a.panes[0].overlays.Items()
	if len(items) != 1 || len(a.panes[1].overlays.Items()) != 0 {
		t.Fatalf("overlay drawings %d %d", len(items), len(a.panes[1].overlays.Items()))
	}
	got := items[0].Points[0].Price
	want := LocalYToPrice(float64(price.H), float64(y-price.Y), a.priceScale(0))
	if math.Abs(got-want) > 0.05 {
		t.Fatalf("stored %v, pointer price %v", got, want)
	}
	// The stored value is a raw price. An index near 100 would mean the
	// click was written in normalized units.
	if math.Abs(got-100) < 1 && math.Abs(want-a.panes[0].candles[len(a.panes[0].candles)-1].Close) > 5 {
		t.Fatalf("stored price %v looks like an index, last close %v", got, a.panes[0].candles[len(a.panes[0].candles)-1].Close)
	}
	press(t, tt, a, "Stack")
	if math.Abs(a.panes[0].overlays.Items()[0].Points[0].Price-got) > 1e-9 {
		t.Fatal("returning to stack changed the stored price")
	}
}

func TestHollowToggleThinsRisingBodies(t *testing.T) {
	a := newApp()
	// Wide bars make the hollow interior larger than the outline, which
	// is the contrast the toggle is for. The default width is only 8 DIPs.
	a.vp.BarWidth = MaxBarWidth
	tt := ui.NewTester(a.View, 1100, 1000)
	if a.Hollow() {
		t.Fatal("hollow rising candles start off")
	}
	up := ui.RGB(38, 166, 154)
	down := ui.RGB(239, 83, 80)
	price := a.painted[0].Price
	solidUp := countNear(tt.Image(), price, up)
	solidDown := countNear(tt.Image(), price, down)
	press(t, tt, a, "Hollow")
	if !a.hollowUp {
		t.Fatal("Hollow toggle did not stick")
	}
	hollowUp := countNear(tt.Image(), price, up)
	hollowDown := countNear(tt.Image(), price, down)
	if solidUp < 80 || solidDown < 40 {
		t.Fatalf("solid candle pixels up %d down %d", solidUp, solidDown)
	}
	if hollowUp*2 >= solidUp {
		t.Fatalf("hollow rising pixels %d, solid %d", hollowUp, solidUp)
	}
	// Falling bodies stay filled, so the red count stays in the same range.
	if hollowDown*2 < solidDown || solidDown*2 < hollowDown {
		t.Fatalf("falling pixels changed hollow %d solid %d", hollowDown, solidDown)
	}
	press(t, tt, a, "Hollow")
	if a.hollowUp {
		t.Fatal("second click should fill rising bodies again")
	}
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
	tt := ui.NewTester(a.View, 1100, 1000)
	writePNG(t, dir+"/p1-macd.png", tt.Image())
	if len(a.painted) < 2 || a.painted[0].MACD.H <= 1 {
		t.Fatal("frame has no MACD band")
	}
	macd := a.painted[0].MACD
	tt.Move(macd.X+120, macd.Y+macd.H/2)
	writePNG(t, dir+"/p1-crosshair.png", tt.Image())
	press(t, tt, a, "1d")
	writePNG(t, dir+"/p1-1d.png", tt.Image())

	press(t, tt, a, "1h")
	press(t, tt, a, "H-Line")
	alpha := a.painted[0].Price
	beta := a.painted[1].Price
	tt.ClickAt(alpha.X+180, alpha.Y+alpha.H*0.42)
	press(t, tt, a, "Segment")
	tt.ClickAt(alpha.X+80, alpha.Y+alpha.H*0.22)
	tt.Move(alpha.X+320, alpha.Y+alpha.H*0.78)
	writePNG(t, dir+"/p2-rubber-band.png", tt.Image())
	tt.ClickAt(alpha.X+320, alpha.Y+alpha.H*0.78)
	tt.ClickAt(beta.X+140, beta.Y+beta.H*0.35)
	tt.ClickAt(beta.X+300, beta.Y+beta.H*0.62)
	tt.Move(10, 10)
	writePNG(t, dir+"/p2-hline-segment.png", tt.Image())
	if len(a.panes[0].overlays.Items()) != 2 || len(a.panes[1].overlays.Items()) != 1 {
		t.Fatalf("frame overlays %d %d", len(a.panes[0].overlays.Items()), len(a.panes[1].overlays.Items()))
	}

	ov := newApp()
	ovt := ui.NewTester(ov.View, 1100, 1000)
	press(t, ovt, ov, "Overlay")
	ovt.Move(10, 10)
	writePNG(t, dir+"/p3-overlay.png", ovt.Image())
	if len(ov.painted) != 1 || ov.painted[0].Price.H < 80 || ov.painted[0].Volume.H < 16 {
		t.Fatal("overlay frame has no single price plot")
	}
	price := ov.painted[0].Price
	_, last := ov.vp.Visible(ov.bars(), ov.plotW)
	x := price.X + float32(ov.vp.BarLeft(last)+ov.vp.BarWidth/2)
	if x < price.X || x > price.X+price.W {
		x = price.X + price.W*0.7
	}
	ovt.Move(x, price.Y+price.H*0.45)
	writePNG(t, dir+"/p3-overlay-crosshair.png", ovt.Image())
	if ov.crossIndex < 0 || !strings.Contains(ov.readout(0), "Hover") || !strings.Contains(ov.readout(1), "Hover") {
		t.Fatalf("overlay crosshair readouts %q %q", ov.readout(0), ov.readout(1))
	}

	// Wide bars make a hollow body readable next to a filled one.
	p4 := newApp()
	p4.vp.BarWidth = 16
	p4t := ui.NewTester(p4.View, 1100, 1000)
	p4t.SetDark(false)
	p4t.Move(10, 10)
	writePNG(t, dir+"/p4-light.png", p4t.Image())
	p4t.SetDark(true)
	writePNG(t, dir+"/p4-dark.png", p4t.Image())
	press(t, p4t, p4, "Hollow")
	writePNG(t, dir+"/p4-dark-hollow.png", p4t.Image())
	p4t.SetDark(false)
	writePNG(t, dir+"/p4-light-hollow.png", p4t.Image())
	if !p4.hollowUp {
		t.Fatal("hollow frame was painted solid")
	}

	p6 := newApp()
	p6t := ui.NewTester(p6.View, 1100, 1000)
	press(t, p6t, p6, "BOLL")
	press(t, p6t, p6, "KDJ")
	writePNG(t, dir+"/p6-boll-kdj-stack.png", p6t.Image())
	if len(p6.painted) != 2 || p6.painted[0].KDJ.H <= 1 || p6.painted[0].KDJ.Y < p6.painted[0].MACD.Y+p6.painted[0].MACD.H {
		t.Fatalf("stack frame should show KDJ under MACD: %+v", p6.painted)
	}
	if !strings.Contains(p6.readout(0), "MID ") || !strings.Contains(p6.readout(0), "K ") {
		t.Fatalf("stack frame readout %q", p6.readout(0))
	}

	press(t, p6t, p6, "Fib")
	fibPlot := p6.painted[0].Price
	p6t.ClickAt(fibPlot.X+90, fibPlot.Y+fibPlot.H*0.2)
	p6t.ClickAt(fibPlot.X+340, fibPlot.Y+fibPlot.H*0.78)
	p6t.Move(10, 10)
	writePNG(t, dir+"/p7-fibonacci.png", p6t.Image())
	if len(p6.panes[0].overlays.Items()) == 0 || p6.panes[0].overlays.Items()[0].Kind != OverlayFibonacci {
		t.Fatal("fib frame should show a completed fib on alpha")
	}

	p8 := NewChart(ChartOptions{})
	p8t := ui.NewTester(p8.View, 1100, 1000)
	st := p8.Style(nil)
	st.Up = ui.RGB(255, 214, 0)
	st.Down = ui.RGB(56, 189, 248)
	st.LimitUp = ui.RGB(255, 255, 255)
	st.LimitDown = ui.RGB(168, 85, 247)
	p8.SetStyle(st)
	p8t.Frame()
	writePNG(t, dir+"/p8-custom-colors.png", p8t.Image())
	if countNear(p8t.Image(), p8.painted[0].Price, st.Up) < 20 {
		t.Fatal("custom up color did not paint")
	}
}

func TestThemeChangesPlotBackground(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 800)
	tt.SetDark(false)
	price := a.painted[0].Price
	light := countNear(tt.Image(), price, LightDefault().Plot)
	tt.SetDark(true)
	dark := countNear(tt.Image(), price, DarkDefault().Plot)
	if light < 200 || dark < 200 {
		t.Fatalf("plot pixels light %d dark %d in %+v", light, dark, price)
	}
}

func assertStep(t *testing.T, a *Chart, step time.Duration) {
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

func assertFitRight(t *testing.T, a *Chart) {
	t.Helper()
	if !a.fitted || a.plotW <= 0 {
		t.Fatalf("fitted %v plotW %v", a.fitted, a.plotW)
	}
	v := Viewport{BarWidth: a.vp.BarWidth}
	v.FitRight(a.bars(), a.plotW)
	if math.Abs(a.vp.Offset-v.Offset) > 1e-6 {
		t.Fatalf("offset %v, fit %v", a.vp.Offset, v.Offset)
	}
}

func TestLiveBarAppendsRevisesAndRecalculates(t *testing.T) {
	a := newApp()
	if _, ok := a.src.(LiveSource); ok {
		t.Fatal("the demo source must not implement LiveSource")
	}
	stub := stubWithPanes(t, a, a.period)
	if err := a.SetSource(stub); err != nil {
		t.Fatal(err)
	}
	n := len(a.panes[0].candles)
	if maLineLen(&a.panes[0]) != n {
		t.Fatalf("ma %d bars %d", maLineLen(&a.panes[0]), n)
	}
	last := a.panes[0].candles[n-1]
	next := last
	next.Time = last.Time.Add(time.Hour)
	next.Close = last.Close + 5
	stub.Push("ALPHA", a.period, next)
	// BETA has no bar at that open time yet, so ALPHA's bar waits.
	if len(a.panes[0].candles) != n || len(a.panes[0].held) != 1 || a.Bars() != n || maLineLen(&a.panes[0]) != n {
		t.Fatalf("one-sided push: candles %d held %d bars %d", len(a.panes[0].candles), len(a.panes[0].held), a.Bars())
	}
	revised := next
	revised.Close = next.Close + 1
	stub.Push("ALPHA", a.period, revised)
	if len(a.panes[0].held) != 1 || a.panes[0].held[0].Close != revised.Close {
		t.Fatal("revise did not replace the held bar")
	}
	beta := a.panes[1].candles[n-1]
	beta.Time = next.Time
	beta.Close += 2
	stub.Push("BETA", a.period, beta)
	if a.Bars() != n+1 || a.panes[0].candles[n] != revised || a.panes[1].candles[n] != beta {
		t.Fatalf("matched open time: bars %d", a.Bars())
	}
	if len(a.panes[0].held) != 0 || len(a.panes[1].held) != 0 {
		t.Fatal("matched bars stayed held")
	}
	if maLineLen(&a.panes[0]) != n+1 || maLineLen(&a.panes[1]) != n+1 {
		t.Fatal("MA was not recalculated")
	}
	shown := revised
	shown.Close = revised.Close + 1
	stub.Push("ALPHA", a.period, shown)
	if a.Bars() != n+1 || a.panes[0].candles[n].Close != shown.Close {
		t.Fatal("revise did not replace the last bar")
	}
	stale := last
	stale.Close = 0
	stub.Push("ALPHA", a.period, stale)
	if a.Bars() != n+1 || a.panes[0].candles[n].Close != shown.Close || len(a.panes[0].held) != 0 {
		t.Fatal("a bar older than the last open time was applied")
	}
}

func TestLiveSubscriptionFollowsPeriodAndSource(t *testing.T) {
	a := newApp()
	hour := a.period
	four := Period{Span: 4, Unit: UnitHour}
	syn := &SyntheticSource{}
	stub := &StubLiveSource{}
	for _, s := range a.panes {
		if err := stub.SetBars(s.name, hour, s.candles); err != nil {
			t.Fatal(err)
		}
		bars, _, err := syn.GetBars(BarRequest{
			Symbol: s.name, Period: four, Kind: KindInit, Limit: sampleBars,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := stub.SetBars(s.name, four, bars); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.SetSource(stub); err != nil {
		t.Fatal(err)
	}
	if err := a.SetPeriod(four); err != nil {
		t.Fatal(err)
	}
	before := len(a.panes[0].candles)
	last := a.panes[0].candles[before-1]
	stub.Push("ALPHA", hour, Candle{Time: last.Time.Add(time.Hour), Close: 1})
	if len(a.panes[0].candles) != before {
		t.Fatalf("1h push after a 4h switch len %d", len(a.panes[0].candles))
	}
	stub.Push("ALPHA", four, Candle{Time: last.Time.Add(4 * time.Hour), Close: 2})
	stub.Push("BETA", four, Candle{Time: last.Time.Add(4 * time.Hour), Close: 2})
	if len(a.panes[0].candles) != before+1 || a.panes[0].candles[before].Close != 2 {
		t.Fatalf("4h push len %d", len(a.panes[0].candles))
	}

	a.src = syn
	if err := a.SetPeriod(hour); err != nil {
		t.Fatal(err)
	}
	before = len(a.panes[0].candles)
	last = a.panes[0].candles[before-1]
	stub.Push("ALPHA", hour, Candle{Time: last.Time.Add(time.Hour), Close: 3})
	if len(a.panes[0].candles) != before {
		t.Fatal("the stub still delivered after the chart left it")
	}
}

func TestLiveBarFollowsRightEdgeOnly(t *testing.T) {
	a := newApp()
	stub := stubWithPanes(t, a, a.period)
	if err := a.SetSource(stub); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 960, 720)
	tt.Frame()
	if !a.fitted || a.plotW <= 1 {
		t.Fatalf("fitted %v plotW %v", a.fitted, a.plotW)
	}
	before := a.bars()
	pushBoth(stub, a, 1)
	if a.bars() != before+1 {
		t.Fatalf("bars %d, want %d", a.bars(), before+1)
	}
	fitted := Viewport{BarWidth: a.vp.BarWidth}
	fitted.FitRight(a.bars(), a.plotW)
	if math.Abs(a.vp.Offset-fitted.Offset) > 1e-6 {
		t.Fatalf("offset %v, fit %v", a.vp.Offset, fitted.Offset)
	}
	a.vp.Offset = 0
	pushBoth(stub, a, 2)
	if a.vp.Offset != 0 {
		t.Fatalf("panned offset moved to %v", a.vp.Offset)
	}
	tt.Frame()
	if !tt.HasText("ALPHA") || !tt.HasText("BETA") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

func stubWithPanes(t *testing.T, a *Chart, period Period) *StubLiveSource {
	t.Helper()
	stub := &StubLiveSource{}
	for _, s := range a.panes {
		if err := stub.SetBars(s.name, period, s.candles); err != nil {
			t.Fatal(err)
		}
	}
	return stub
}

func pushBoth(src *StubLiveSource, a *Chart, closeDelta float64) {
	for _, s := range a.panes {
		last := s.candles[len(s.candles)-1]
		next := last
		next.Time = last.Time.Add(time.Hour)
		next.Close = last.Close + closeDelta
		src.Push(s.name, a.period, next)
	}
}

func maLineLen(s *series) int {
	for _, ind := range s.inds {
		ma, ok := ind.(*MA)
		if !ok || len(ma.Lines()) == 0 {
			continue
		}
		return len(ma.Lines()[0])
	}
	return -1
}

type sourceFunc func(BarRequest) ([]Candle, bool, error)

func (f sourceFunc) GetBars(req BarRequest) ([]Candle, bool, error) {
	return f(req)
}

type reenterSource struct {
	inner    DataSource
	app      *Chart
	forwards int
}

func (r *reenterSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	if req.Kind == KindForward {
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

func onOHLC(price float64, c Candle, eps float64) bool {
	for _, p := range []float64{c.Open, c.High, c.Low, c.Close} {
		if math.Abs(price-p) <= eps {
			return true
		}
	}
	return false
}

func TestMagnetToolbarAndBehavior(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	if a.Magnet() != MagnetOff {
		t.Fatalf("default magnet %v", a.magnet)
	}
	press(t, tt, a, "Magnet Strong")
	if a.magnet != MagnetStrong {
		t.Fatalf("magnet mode %v", a.magnet)
	}
	tt.Frame()
	press(t, tt, a, "Magnet Strong")
	if a.magnet != MagnetStrong {
		t.Fatal("strong should stay selected")
	}

	press(t, tt, a, "H-Line")
	p0 := a.painted[0].Price
	x := p0.X + 200
	y := p0.Y + p0.H*0.42
	plotX := float64(x - p0.X)
	idx, ok := a.vp.IndexAt(plotX, len(a.panes[0].candles))
	if !ok {
		t.Fatal("no bar under click")
	}
	bar := a.panes[0].candles[idx]
	scale := MainScale(a.panes[0].candles, a.active(a.panes[0]), a.vp, a.plotW)
	raw := LocalYToPrice(float64(p0.H), float64(y-p0.Y), scale)
	if onOHLC(raw, bar, 1e-6) {
		y = p0.Y + p0.H*0.37
		raw = LocalYToPrice(float64(p0.H), float64(y-p0.Y), scale)
	}
	tt.ClickAt(x, y)
	line := a.panes[0].overlays.Items()[0]
	stored := line.Points[0].Price
	if !onOHLC(stored, bar, 1e-4) {
		t.Fatalf("strong magnet: stored %v not on bar OHLC (raw %v) %+v", stored, raw, bar)
	}

	placed := stored
	press(t, tt, a, "Magnet Off")
	press(t, tt, a, "Magnet Strong")
	after := a.panes[0].overlays.Items()[0].Points[0].Price
	if math.Abs(after-placed) > 1e-9 {
		t.Fatalf("mode change rewrote price: %v -> %v", placed, after)
	}

	y2 := p0.Y + p0.H*0.72
	lineY := p0.Y + float32(PriceToLocalY(float64(p0.H), stored, scale))
	tt.Press(x, lineY)
	tt.Move(x, y2)
	tt.Release(x, y2)
	dragged := a.panes[0].overlays.Items()[0].Points[0].Price
	wantRaw := LocalYToPrice(float64(p0.H), float64(y2-p0.Y), scale)
	if math.Abs(dragged-wantRaw) > 0.05 {
		t.Fatalf("body drag should use raw price %v, got %v", wantRaw, dragged)
	}
	barAt := a.panes[0].candles[line.Points[0].BarIndex]
	if onOHLC(wantRaw, barAt, 1e-4) {
		t.Fatal("pick a drag target between OHLC levels for this test")
	}
	if onOHLC(dragged, barAt, 1e-4) {
		t.Fatalf("existing hline body drag snapped onto OHLC %v (raw %v)", dragged, wantRaw)
	}
}

// A real press jitters. The price stored after Press, a 1 DIP move, and
// Release must stay on the bar's OHLC when magnet is strong.
func TestNewHLinePressMoveKeepsStrongMagnet(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	press(t, tt, a, "Magnet Strong")
	press(t, tt, a, "H-Line")
	p0 := a.painted[0].Price
	x := p0.X + 200
	y := p0.Y + p0.H*0.42
	y1 := y + 1 // 1 DIP
	plotX := float64(x - p0.X)
	idx, ok := a.vp.IndexAt(plotX, len(a.panes[0].candles))
	if !ok {
		t.Fatal("no bar under the pointer")
	}
	scale := a.priceScale(0)
	raw := LocalYToPrice(float64(p0.H), float64(y1-p0.Y), scale)
	bar := a.panes[0].candles[idx]
	if onOHLC(raw, bar, 1e-4) {
		y = p0.Y + p0.H*0.37
		y1 = y + 1
		raw = LocalYToPrice(float64(p0.H), float64(y1-p0.Y), scale)
		bar = a.panes[0].candles[idx]
	}
	if onOHLC(raw, bar, 1e-4) {
		t.Fatalf("1 DIP move landed on OHLC %v of %+v; pick another Y", raw, bar)
	}
	tt.Press(x, y)
	tt.Move(x, y1)
	tt.Release(x, y1)
	items := a.panes[0].overlays.Items()
	if len(items) != 1 || len(items[0].Points) != 1 {
		t.Fatal("hline was not placed")
	}
	stored := items[0].Points[0].Price
	if !onOHLC(stored, bar, 1e-4) {
		t.Fatalf("new hline after 1 DIP move stored %v, want OHLC of bar %+v (raw %v)", stored, bar, raw)
	}
}

func TestStrongMagnetEmptySeriesKeepsPointerPrice(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 1100, 1000)
	for i := range a.panes {
		a.panes[i].candles = nil
	}
	press(t, tt, a, "Magnet Strong")
	press(t, tt, a, "H-Line")
	p0 := a.painted[0].Price
	x := p0.X + 180
	y := p0.Y + p0.H*0.45
	raw := LocalYToPrice(float64(p0.H), float64(y-p0.Y), a.priceScale(0))
	if math.Abs(raw) < 1e-4 {
		t.Fatal("pointer price is 0; pick a Y off the scale floor")
	}
	tt.ClickAt(x, y)
	items := a.panes[0].overlays.Items()
	if len(items) != 1 || len(items[0].Points) != 1 {
		t.Fatal("hline was not placed on an empty series")
	}
	stored := items[0].Points[0].Price
	if math.Abs(stored-raw) > 1e-4 {
		t.Fatalf("empty series strong magnet stored %v, pointer price %v", stored, raw)
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
