package kline

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func TestMarkerPlacementStacksAndClears(t *testing.T) {
	a := newApp()
	bar := a.panes[0].candles[40]
	inside := bar.Time.Add(15 * time.Minute)
	if err := a.SetMarkers("600519", []Marker{
		{Time: bar.Time, Side: SideBuy, Qty: 2},
		{Time: inside, Side: SideBuy, Label: "add"},
		{Time: bar.Time, Side: SideSell, Price: bar.High},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.AppendMarker("600519", Marker{Time: bar.Time, Side: SideSell, Label: "S"}); err != nil {
		t.Fatal(err)
	}
	got, err := a.Markers("600519")
	if err != nil || len(got) != 4 {
		t.Fatalf("markers %d err %v", len(got), err)
	}
	marks := a.panes[0].marks
	if marks[0].bar != 40 || marks[1].bar != 40 || marks[0].stack != 0 || marks[1].stack != 1 {
		t.Fatalf("buys %+v %+v", marks[0], marks[1])
	}
	if marks[2].stack != 0 || marks[3].stack != 1 || marks[2].m.Price != bar.High {
		t.Fatalf("sells %+v %+v", marks[2], marks[3])
	}
	if marks[0].text != "B 2" || marks[1].text != "add" || marks[3].text != "S" {
		t.Fatalf("labels %q %q %q", marks[0].text, marks[1].text, marks[3].text)
	}
	beta, err := a.Markers("000001")
	if err != nil || len(beta) != 0 {
		t.Fatalf("beta markers leaked: %d %v", len(beta), err)
	}
	if err := a.ClearMarkers("600519"); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.Markers("600519"); len(n) != 0 {
		t.Fatalf("clear left %d", len(n))
	}
}

func TestMarkerTipsSitOutsideTheBar(t *testing.T) {
	const yLow, yHigh = float32(180), float32(40)
	buy0 := markerTipY(SideBuy, yLow, yHigh, 0)
	buy1 := markerTipY(SideBuy, yLow, yHigh, 1)
	sell0 := markerTipY(SideSell, yLow, yHigh, 0)
	sell1 := markerTipY(SideSell, yLow, yHigh, 1)
	if buy0 <= yLow || buy1 != buy0+markerStride {
		t.Fatalf("buy tips %v %v, low %v stride %v", buy0, buy1, yLow, markerStride)
	}
	if sell0 >= yHigh || sell1 != sell0-markerStride {
		t.Fatalf("sell tips %v %v, high %v", sell0, sell1, yHigh)
	}
	if buy1-buy0 < markerH || yHigh-sell1 < markerH {
		t.Fatal("stacked markers overlap")
	}
}

func TestMarkerPriceTickUsesGivenPrice(t *testing.T) {
	with := tradeMark{m: Marker{Side: SideBuy, Price: 11}}
	without := tradeMark{m: Marker{Side: SideBuy}}
	if !markerTick(with) || markerTick(without) {
		t.Fatal("a zero price uses the bar and draws no tick")
	}
	plot := ui.Rect{X: 0, Y: 0, W: 100, H: 200}
	scale := chart.NewLinear(10, 12)
	y := markerTickY(plot, scale, with, 1)
	want := chart.Y(plot, scale.At(11))
	if math.Abs(float64(y-want)) > 0.1 {
		t.Fatalf("tick y %v want %v", y, want)
	}
}

func TestSetMarkersRejectsUnknownSymbolAndSide(t *testing.T) {
	a := newApp()
	bar := a.panes[0].candles[3].Time
	keep := []Marker{{Time: bar, Side: SideBuy}}
	if err := a.SetMarkers("600519", keep); err != nil {
		t.Fatal(err)
	}
	err := a.SetMarkers("NOPE", []Marker{{Time: bar, Side: SideBuy}})
	if !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("symbol err %v", err)
	}
	err = a.SetMarkers("600519", []Marker{{Time: bar, Side: SideBuy}, {Time: bar, Side: 0}})
	if !errors.Is(err, ErrUnknownMarker) {
		t.Fatalf("side err %v", err)
	}
	got, _ := a.Markers("600519")
	if len(got) != 1 || got[0].Side != SideBuy {
		t.Fatalf("rejected set changed markers: %+v", got)
	}
	host := []Marker{{Time: bar, Side: SideSell}}
	if err := a.SetMarkers("600519", host); err != nil {
		t.Fatal(err)
	}
	host[0].Side = SideBuy
	got, _ = a.Markers("600519")
	if got[0].Side != SideSell {
		t.Fatal("SetMarkers kept the caller's slice")
	}
}

func TestMarkerPeriodRemapAndOutOfRange(t *testing.T) {
	start := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	var hour, four []Candle
	for i := 0; i < 8; i++ {
		hour = append(hour, Candle{Time: start.Add(time.Duration(i) * time.Hour), Open: 10, High: 12, Low: 9, Close: 11})
	}
	for i := 0; i < 2; i++ {
		four = append(four, Candle{Time: start.Add(time.Duration(i) * 4 * time.Hour), Open: 10, High: 12, Low: 9, Close: 11})
	}
	src := &remapSource{hour: hour, four: four}
	a := NewChart(ChartOptions{
		Source:     src,
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Indicators: []string{},
	})
	// 01:30 sits in the 01:00 hour bar and in the 00:00 four-hour bar.
	at := start.Add(90 * time.Minute)
	outside := start.Add(-time.Hour)
	past := start.Add(8 * time.Hour)
	if err := a.SetMarkers("600519", []Marker{
		{Time: at, Side: SideBuy},
		{Time: outside, Side: SideSell},
		{Time: past, Side: SideBuy},
	}); err != nil {
		t.Fatal(err)
	}
	if a.panes[0].marks[0].bar != 1 || a.panes[0].marks[1].bar != -1 || a.panes[0].marks[2].bar != -1 {
		t.Fatalf("hour placement %+v", a.panes[0].marks)
	}
	tt := ui.NewTester(a.View, 800, 600)
	tt.Frame()
	if err := a.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.panes[0].marks) != 3 {
		t.Fatal("period switch dropped markers")
	}
	if a.panes[0].marks[0].bar != 0 || a.panes[0].marks[1].bar != -1 || a.panes[0].marks[2].bar != -1 {
		t.Fatalf("4h placement %+v bars %d", a.panes[0].marks, len(a.panes[0].candles))
	}
	a.crossIndex = 0
	if !strings.Contains(a.Readout("600519"), "B @") {
		t.Fatalf("readout %q", a.Readout("600519"))
	}
	a.crossIndex = -1
	if strings.Contains(a.Readout("600519"), "B @") {
		t.Fatalf("last bar should not show the 00:00 marker: %q", a.Readout("600519"))
	}
}

// install must map the retained markers with the target period. The
// previous implementation mounted first, so the last bar's end still
// came from the old period.
func TestMarkerReindexUsesTargetPeriodOnLoad(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	four := Period{Span: 4, Unit: UnitHour}
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	bar := Candle{Time: start, Open: 100, High: 101, Low: 99, Close: 100, Volume: 10}
	for _, tc := range []struct {
		name          string
		before, after Period
		wantBar       int
		wantReadout   bool
	}{
		{name: "coarsen", before: hour, after: four, wantBar: 0, wantReadout: true},
		{name: "refine", before: four, after: hour, wantBar: -1, wantReadout: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			feed := &StubLiveSource{}
			for _, p := range []Period{hour, four} {
				if err := feed.SetBars("A", p, []Candle{bar}); err != nil {
					t.Fatal(err)
				}
			}
			c := NewChart(ChartOptions{Source: feed, Symbols: []string{"A"}, Period: tc.before, Indicators: []string{}})
			defer c.Close()
			if err := c.SetMarkers("A", []Marker{{
				Time: start.Add(3 * time.Hour), Side: SideBuy, Label: "late_trade",
			}}); err != nil {
				t.Fatal(err)
			}
			if err := c.SetPeriod(tc.after); err != nil {
				t.Fatal(err)
			}
			if got := c.panes[0].marks[0].bar; got != tc.wantBar {
				t.Fatalf("marker bar %d, want %d after %s -> %s", got, tc.wantBar, tc.before, tc.after)
			}
			if got := strings.Contains(c.Readout("A"), "late_trade"); got != tc.wantReadout {
				t.Fatalf("readout visibility %v, want %v: %q", got, tc.wantReadout, c.Readout("A"))
			}
		})
	}
}

func TestReplayAggregateMarkerStaysHiddenUntilBaseReveal(t *testing.T) {
	start := time.Date(2024, 3, 1, 8, 0, 0, 0, time.UTC)
	base := sampleWalk(8, 1, start, time.Hour)
	all := map[string][]Candle{"600519": base}
	c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
	r, err := StartReplay(c, all)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMarkers("600519", []Marker{{
		Time: base[2].Time, Side: SideBuy, Label: "ten",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := r.SeekIndex(2); err != nil {
		t.Fatal(err)
	}
	if err := c.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	if got := c.panes[0].marks[0].bar; got != -1 {
		t.Fatalf("hidden 10:00 marker mapped to aggregate bar %d", got)
	}
	c.crossIndex = 0
	if strings.Contains(c.Readout("600519"), "ten") {
		t.Fatalf("readout leaked a hidden aggregate marker: %q", c.Readout("600519"))
	}
	if err := r.Step(1); err != nil {
		t.Fatal(err)
	}
	if got := c.panes[0].marks[0].bar; got != 0 {
		t.Fatalf("revealed 10:00 marker mapped to aggregate bar %d, want 0", got)
	}
	if !strings.Contains(c.Readout("600519"), "ten") {
		t.Fatalf("readout missed the revealed marker: %q", c.Readout("600519"))
	}
	if err := r.StepBack(1); err != nil {
		t.Fatal(err)
	}
	if got := c.panes[0].marks[0].bar; got != -1 {
		t.Fatalf("StepBack left the marker at aggregate bar %d", got)
	}
}

func TestReplayAggregateMarkerStackUsesVisibleOnly(t *testing.T) {
	start := time.Date(2024, 3, 1, 8, 0, 0, 0, time.UTC)
	base := sampleWalk(8, 2, start, time.Hour)
	all := map[string][]Candle{"600519": base}
	c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
	r, err := StartReplay(c, all)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMarkers("600519", []Marker{
		{Time: base[2].Time, Side: SideBuy, Label: "ten"},
		{Time: base[3].Time, Side: SideBuy, Label: "eleven"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.SeekIndex(2); err != nil {
		t.Fatal(err)
	}
	if err := c.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	for i, mark := range c.panes[0].marks {
		if mark.bar != -1 || mark.stack != 0 {
			t.Fatalf("hidden mark %d bar %d stack %d", i, mark.bar, mark.stack)
		}
	}
	if err := r.Step(1); err != nil {
		t.Fatal(err)
	}
	if mark := c.panes[0].marks[0]; mark.bar != 0 || mark.stack != 0 {
		t.Fatalf("first visible mark bar %d stack %d", mark.bar, mark.stack)
	}
	if mark := c.panes[0].marks[1]; mark.bar != -1 || mark.stack != 0 {
		t.Fatalf("still hidden mark bar %d stack %d", mark.bar, mark.stack)
	}
	if err := r.Step(1); err != nil {
		t.Fatal(err)
	}
	if mark := c.panes[0].marks[1]; mark.bar != 0 || mark.stack != 1 {
		t.Fatalf("second visible mark bar %d stack %d", mark.bar, mark.stack)
	}
}

func TestMarkerReplayBoundaryUsesHalfOpenInterval(t *testing.T) {
	start := time.Date(2024, 3, 1, 8, 0, 0, 0, time.UTC)
	base := sampleWalk(4, 3, start, time.Hour)
	all := map[string][]Candle{"600519": base}
	c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
	r, err := StartReplay(c, all)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMarkers("600519", []Marker{
		{Time: base[1].Time, Side: SideBuy, Label: "at-open"},
		{Time: base[1].Time.Add(-time.Nanosecond), Side: SideSell, Label: "before-open"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.SeekIndex(1); err != nil {
		t.Fatal(err)
	}
	if c.panes[0].marks[0].bar != -1 || c.panes[0].marks[1].bar != 0 {
		t.Fatalf("cursor at 09:00 marks %+v", c.panes[0].marks)
	}
	if err := r.Step(1); err != nil {
		t.Fatal(err)
	}
	if c.panes[0].marks[0].bar != 1 {
		t.Fatalf("marker at the 09:00 open mapped to bar %d, want 1", c.panes[0].marks[0].bar)
	}
}

func TestMarkerPrependShiftsIndex(t *testing.T) {
	a := newApp()
	a.fitted = true
	a.plotW = 800
	const watch = 20
	when := a.panes[0].candles[watch].Time
	if err := a.SetMarkers("600519", []Marker{{Time: when, Side: SideBuy, Label: "B"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetMarkers("000001", []Marker{{Time: a.panes[1].candles[watch].Time, Side: SideSell}}); err != nil {
		t.Fatal(err)
	}
	a.vp.Offset = 2
	before := a.panes[0].marks[0].bar
	a.ensureForward()
	got := a.panes[0].marks[0].bar
	if got != before+forwardBatch {
		t.Fatalf("index %d before %d", got, before)
	}
	if !a.panes[0].candles[got].Time.Equal(when) {
		t.Fatalf("bar %d time %s want %s", got, a.panes[0].candles[got].Time, when)
	}
	if a.panes[1].marks[0].bar != before+forwardBatch {
		t.Fatalf("beta index %d", a.panes[1].marks[0].bar)
	}
}

func TestMarkerLiveAppendRemaps(t *testing.T) {
	a := newApp()
	hour := a.period
	stub := &StubLiveSource{}
	for _, s := range a.panes {
		if err := stub.SetBars(s.name, hour, s.candles); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.SetSource(stub); err != nil {
		t.Fatal(err)
	}
	last := a.panes[0].candles[len(a.panes[0].candles)-1]
	next := last
	next.Time = last.Time.Add(time.Hour)
	next.Close = last.Close + 1
	if err := a.SetMarkers("600519", []Marker{{Time: next.Time, Side: SideBuy, Label: "open"}}); err != nil {
		t.Fatal(err)
	}
	if a.panes[0].marks[0].bar != -1 {
		t.Fatalf("future marker placed at %d", a.panes[0].marks[0].bar)
	}
	stub.Push("600519", hour, next)
	beta := a.panes[1].candles[len(a.panes[1].candles)-1]
	beta.Time = next.Time
	stub.Push("000001", hour, beta)
	want := len(a.panes[0].candles) - 1
	if a.panes[0].marks[0].bar != want {
		t.Fatalf("after append bar %d want %d", a.panes[0].marks[0].bar, want)
	}
	a.crossIndex = want
	if !strings.Contains(a.Readout("600519"), "open") {
		t.Fatalf("readout %q", a.Readout("600519"))
	}
	if strings.Contains(a.Readout("000001"), "open") {
		t.Fatal("beta readout picked up alpha's marker")
	}
}

func TestMarkerColorsFollowUpDown(t *testing.T) {
	base := DarkDefault()
	if base.Buy != base.Up || base.Sell != base.Down {
		t.Fatalf("defaults buy %+v up %+v sell %+v down %+v", base.Buy, base.Up, base.Sell, base.Down)
	}
	partial := completeStyle(Style{Up: ui.RGB(1, 2, 3), Down: ui.RGB(4, 5, 6)})
	if partial.Buy != partial.Up || partial.Sell != partial.Down {
		t.Fatalf("partial palette did not default markers: %+v %+v", partial.Buy, partial.Sell)
	}
	custom := completeStyle(Style{Up: ui.RGB(1, 2, 3), Buy: ui.RGB(9, 9, 9)})
	if custom.Buy != ui.RGB(9, 9, 9) {
		t.Fatalf("explicit buy %+v", custom.Buy)
	}
	cleared := completeStyle(Style{Up: ui.RGB(1, 2, 3), Clear: ClearBuy})
	if cleared.Buy != (ui.Color{}) {
		t.Fatalf("cleared buy %+v", cleared.Buy)
	}
	us := DarkDefault()
	us.Up = ui.RGB(38, 166, 154)
	us.Down = ui.RGB(239, 83, 80)
	us.Buy = us.Up
	us.Sell = us.Down
	if us.Buy == base.Buy || us.Sell == base.Sell {
		t.Fatal("US pair should swap the marker colors with the candles")
	}
}

func TestOverlayMarkersStayOnTheirSymbol(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.View, 900, 700)
	tt.Frame()
	if err := a.SetLayout(LayoutOverlay); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	when := a.panes[1].candles[len(a.panes[1].candles)-5].Time
	if err := a.SetMarkers("000001", []Marker{{Time: when, Side: SideSell, Price: 50, Label: "be"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetMarkers("600519", []Marker{{Time: a.panes[0].candles[len(a.panes[0].candles)-5].Time, Side: SideBuy}}); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.panes[0].marks[0].bar < 0 || a.panes[1].marks[0].bar < 0 {
		t.Fatalf("overlay hid markers alpha %d beta %d", a.panes[0].marks[0].bar, a.panes[1].marks[0].bar)
	}
	if a.panes[0].marks[0].bar == a.panes[1].marks[0].bar && a.panes[0].candles[a.panes[0].marks[0].bar].Time.Equal(when) && a.panes[0].marks[0].m.Label == "be" {
		t.Fatal("alpha took beta's marker")
	}
}

func TestMarkersStayInsidePricePlot(t *testing.T) {
	style := DarkDefault()
	style.Buy = ui.RGB(255, 0, 255)
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Indicators: []string{},
		HideHeader: true,
		Style:      style,
	})
	tt := ui.NewTester(c.View, 800, 600)
	tt.Frame()
	first, last := c.Visible()
	bar := c.panes[0].candles[first]
	for i := first; i <= last; i++ {
		if c.panes[0].candles[i].Low < bar.Low {
			bar = c.panes[0].candles[i]
		}
	}
	marks := make([]Marker, 12)
	for i := range marks {
		marks[i] = Marker{Time: bar.Time, Side: SideBuy, Label: "B"}
	}
	if err := c.SetMarkers("600519", marks); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	price := c.painted[0].Price
	volume := c.painted[0].Volume
	if got := countNear(tt.Image(), price, style.Buy); got == 0 {
		t.Fatal("marker test painted no price pixels")
	}
	if got := countNear(tt.Image(), volume, style.Buy); got != 0 {
		t.Fatalf("markers painted %d pixels in the volume strip", got)
	}
}

type remapSource struct {
	hour []Candle
	four []Candle
}

func (s *remapSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	if req.Kind == KindForward {
		return []Candle{}, false, nil
	}
	if req.Period.Span == 4 {
		return append([]Candle(nil), s.four...), false, nil
	}
	return append([]Candle(nil), s.hour...), false, nil
}

// TestMarkerDrawReplacesOnlyTheShape records the split: the library keeps
// resolving times, stacking, clipping, and replay, and the hook answers
// only what one marker looks like. Without the hook the arrow still draws.
func TestMarkerDrawReplacesOnlyTheShape(t *testing.T) {
	bars := hostBars(40)
	hot := ui.RGB(255, 0, 255)
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
	})
	marks := []Marker{
		{Time: bars[10].Time, Side: SideBuy, Qty: 1},
		{Time: bars[10].Time, Side: SideBuy, Qty: 2}, // same bar: must stack
		{Time: bars[25].Time, Side: SideSell, Qty: 1},
	}
	if err := c.SetMarkers("A", marks); err != nil {
		t.Fatalf("SetMarkers: %v", err)
	}
	// Default: no hook, the arrow draws and nothing calls out.
	tt := ui.NewTester(c.View, 900, 500)
	if img := tt.Image(); img == nil {
		t.Fatal("no default frame")
	}

	var seen []MarkerSpot
	c.MarkerDraw(func(p *ui.Painter, spot MarkerSpot, m Marker, style Style) {
		seen = append(seen, spot)
		p.Fill(ui.Rect{X: spot.X - 3, Y: spot.TipY - 3, W: 6, H: 6}, hot, 0)
	})
	tt = ui.NewTester(c.View, 900, 500)
	img := tt.Image()
	if len(seen) != len(marks) {
		t.Fatalf("hook ran %d times for %d markers", len(seen), len(marks))
	}
	// The two buys on one bar must report different stack slots, which is
	// the part the library still owns.
	if seen[0].Stack == seen[1].Stack {
		t.Fatalf("two markers on one bar share stack slot %d", seen[0].Stack)
	}
	if seen[0].TipY == seen[1].TipY {
		t.Fatal("two markers on one bar have the same tip; stacking did not apply")
	}
	// The hook's paint reached the frame.
	if countColor(img, hot) == 0 {
		t.Fatal("the host shape never reached the frame")
	}
	// Every spot carries the bar and candle it resolved to.
	for _, spot := range seen {
		if spot.Bar < 0 || spot.Bar >= len(bars) {
			t.Fatalf("spot bar %d outside the series", spot.Bar)
		}
		if spot.Candle.Time != bars[spot.Bar].Time {
			t.Fatalf("spot bar %d carries the wrong candle", spot.Bar)
		}
	}
}

// TestMarkerDrawNilRestoresTheArrow records that the hook is optional.
func TestMarkerDrawNilRestoresTheArrow(t *testing.T) {
	bars := hostBars(30)
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
	})
	if err := c.SetMarkers("A", []Marker{{Time: bars[10].Time, Side: SideBuy}}); err != nil {
		t.Fatalf("SetMarkers: %v", err)
	}
	calls := 0
	c.MarkerDraw(func(*ui.Painter, MarkerSpot, Marker, Style) { calls++ })
	tt := ui.NewTester(c.View, 900, 500)
	_ = tt.Image()
	if calls == 0 {
		t.Fatal("the hook never ran")
	}
	calls = 0
	c.MarkerDraw(nil)
	tt = ui.NewTester(c.View, 900, 500)
	if img := tt.Image(); img == nil {
		t.Fatal("nil hook produced no frame")
	}
	if calls != 0 {
		t.Fatalf("a cleared hook still ran %d times", calls)
	}
}
