package kline

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// hostBars is a plain daily series long enough to warm up MA60.
func hostBars(n int) []Candle {
	out := make([]Candle, n)
	day := time.Date(2024, 1, 2, 9, 30, 0, 0, ashareLocation)
	for i := range out {
		open := 100 + float64(i)
		out[i] = Candle{
			Time: time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, ashareLocation),
			Open: open, High: open + 2, Low: open - 1, Close: open + 1,
			Volume: 1000,
		}
		day = day.AddDate(0, 0, 1)
	}
	return out
}

func hostChart(t *testing.T, opts ChartOptions) *Chart {
	t.Helper()
	c := NewChart(opts)
	if err := c.LoadError(); err != nil {
		t.Fatalf("load: %v", err)
	}
	tt := ui.NewTester(c.View, 900, 620)
	_ = tt.Image()
	return c
}

// TestMAPeriodsConfiguresTheLines is REQ-05: four lines with the host's
// periods and colors, published by name.
func TestMAPeriodsConfiguresTheLines(t *testing.T) {
	colors := []ui.Color{
		ui.RGB(230, 233, 239), ui.RGB(245, 197, 66),
		ui.RGB(179, 136, 255), ui.RGB(38, 198, 218),
	}
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{"MA"},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
		MAPeriods:  []int{5, 10, 20, 60},
		MAColors:   colors,
	})
	values := c.IndicatorValues("A", c.Bars()-1)
	for _, key := range []string{"MA5", "MA10", "MA20", "MA60"} {
		if _, ok := values[key]; !ok {
			t.Fatalf("%s missing from %v", key, values)
		}
	}
	// The default 5/20 must not leak in as a second definition.
	if _, ok := values["MA20"]; !ok {
		t.Fatal("MA20 missing")
	}
	ma := maOf(t, c)
	if len(ma.Periods) != 4 || ma.Periods[3] != 60 {
		t.Fatalf("MA periods %v", ma.Periods)
	}
}

// maOf finds the MA instance of the first pane.
func maOf(t *testing.T, c *Chart) *MA {
	t.Helper()
	for _, ind := range c.panes[0].inds {
		if ma, ok := ind.(*MA); ok {
			return ma
		}
	}
	t.Fatal("no MA instance")
	return nil
}

// TestMAPeriodsSubsetRecordsThatTurningLinesOffIsASubset is the shape a
// host uses for per-line toggles: in this library a line is off when its
// period is not in the list.
func TestMAPeriodsSubsetRecordsThatTurningLinesOffIsASubset(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{"MA"},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
		MAPeriods:  []int{5, 60},
	})
	values := c.IndicatorValues("A", c.Bars()-1)
	if _, ok := values["MA10"]; ok {
		t.Fatalf("MA10 was not in the period list but appears in %v", values)
	}
	if _, ok := values["MA5"]; !ok {
		t.Fatalf("MA5 missing from %v", values)
	}
	if _, ok := values["MA60"]; !ok {
		t.Fatalf("MA60 missing from %v", values)
	}
}

// TestSetMARebuildsWithoutRebinding records that a host may change the
// periods after NewChart and keep the bindable toggle pointer.
func TestSetMARebuildsWithoutRebinding(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{"MA"},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
	})
	toggle := c.IndicatorEnabled("MA")
	if toggle == nil || !*toggle {
		t.Fatal("MA is not on by default")
	}
	c.SetMA([]int{3, 7}, nil)
	ma := maOf(t, c)
	if len(ma.Periods) != 2 || ma.Periods[0] != 3 || ma.Periods[1] != 7 {
		t.Fatalf("MA periods after SetMA %v", ma.Periods)
	}
	if got := c.IndicatorEnabled("MA"); got != toggle {
		t.Fatal("SetMA replaced the bindable toggle pointer")
	}
	if !*toggle {
		t.Fatal("SetMA turned MA off")
	}
	values := c.IndicatorValues("A", c.Bars()-1)
	if _, ok := values["MA3"]; !ok {
		t.Fatalf("MA3 missing from %v", values)
	}
}

// TestIndicatorValuesNamesAndGaps pins the key set and the warm-up rule.
func TestIndicatorValuesNamesAndGaps(t *testing.T) {
	// 80 bars warm up the slowest built-in: MACD's DEA needs 26 + 9,
	// and BOLL needs 20.
	bars := hostBars(80)
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{"MA", "MACD", "KDJ", "BOLL"},
		Source:     fixedSource{bars: bars},
		Location:   ashareLocation,
	})
	last := c.Bars() - 1
	values := c.IndicatorValues("A", last)
	want := []string{
		"MA5", "MA20",
		"MACD.DIF", "MACD.DEA", "MACD.HIST",
		"KDJ.K", "KDJ.D", "KDJ.J",
		"BOLL.MID", "BOLL.UP", "BOLL.LOW",
	}
	for _, key := range want {
		if _, ok := values[key]; !ok {
			t.Fatalf("%s missing from %v", key, values)
		}
	}
	// At bar 0 nothing is warmed up: every key is absent rather than NaN.
	first := c.IndicatorValues("A", 0)
	if len(first) != 0 {
		t.Fatalf("bar 0 should have no ready values, got %v", first)
	}
	// Turning an indicator off removes its keys.
	if err := c.SetIndicator("BOLL", false); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	off := c.IndicatorValues("A", last)
	if _, ok := off["BOLL.MID"]; ok {
		t.Fatalf("BOLL keys survived a toggle off: %v", off)
	}
	if _, ok := off["MA5"]; !ok {
		t.Fatal("turning BOLL off removed the MA values")
	}
	if got := c.IndicatorValues("NOPE", last); got != nil {
		t.Fatal("an unknown symbol returned values")
	}
	if got := c.IndicatorValues("A", -1); got != nil {
		t.Fatal("a negative index returned values")
	}
}

// TestReadIndicatorValuesReusesAMap records the no-allocation-per-frame
// path: a host keeps one map and it never holds a stale key.
func TestReadIndicatorValuesReusesAMap(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{"MA"},
		Source:     fixedSource{bars: hostBars(30)},
		Location:   ashareLocation,
	})
	out := make(map[string]float64, 8)
	c.ReadIndicatorValues("A", c.Bars()-1, out)
	if len(out) == 0 {
		t.Fatal("no values written into the host map")
	}
	c.ReadIndicatorValues("A", 0, out)
	if len(out) != 0 {
		t.Fatalf("a warm-up gap left stale keys: %v", out)
	}
	// A nil map is accepted and writes nothing.
	c.ReadIndicatorValues("A", c.Bars()-1, nil)
}

// TestSymbolLabelsHideTheTicker is REQ-09: during a blind session the
// element label must not carry the ticker.
func TestSymbolLabelsHideTheTicker(t *testing.T) {
	c := NewChart(ChartOptions{
		Symbols:      []string{"600519"},
		SymbolLabels: []string{"个股"},
		Period:       Period{Span: 1, Unit: UnitDay},
		Indicators:   []string{},
		Source:       fixedSource{bars: hostBars(30)},
		Location:     ashareLocation,
	})
	tt := ui.NewTester(c.View, 900, 620)
	_ = tt.Image()
	if _, ok := tt.Find("600519 chart"); ok {
		t.Fatalf("the ticker is still the element label; texts %q", tt.Texts())
	}
	if _, ok := tt.Find("个股 chart"); !ok {
		t.Fatalf("the display name is not the element label; texts %q", tt.Texts())
	}
	// The data name is unchanged: Bar and Readout still use the symbol.
	if _, ok := c.Bar("600519", 0); !ok {
		t.Fatal("the symbol stopped addressing the data")
	}
	if c.Readout("600519") == "" {
		t.Fatal("the symbol stopped addressing the readout")
	}
}

// TestSymbolLabelsEmptyFallsBack records the documented fallback.
func TestSymbolLabelsEmptyFallsBack(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:      []string{"600519"},
		SymbolLabels: []string{""},
		Period:       Period{Span: 1, Unit: UnitDay},
		Indicators:   []string{},
		Source:       fixedSource{bars: hostBars(30)},
		Location:     ashareLocation,
	})
	if got := c.displayName(0); got != "600519" {
		t.Fatalf("empty label gave %q, want the symbol", got)
	}
	c.SetSymbolLabels(nil)
	if got := c.displayName(0); got != "600519" {
		t.Fatalf("cleared label gave %q, want the symbol", got)
	}
}

// TestPaneHeightsSetsTheBands is REQ-07: the host asks for DIPs and the
// price plot takes the rest.
func TestPaneHeightsSetsTheBands(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:     []string{"A"},
		Period:      Period{Span: 1, Unit: UnitDay},
		Indicators:  []string{"MA", "MACD", "KDJ"},
		Source:      fixedSource{bars: hostBars(90)},
		Location:    ashareLocation,
		PaneHeights: PaneHeights{Sub: 70, Volume: 55},
	})
	panes := c.painted[0]
	if near(panes.MACD.H, 70, 1) == false {
		t.Fatalf("MACD height %v, want 70", panes.MACD.H)
	}
	if near(panes.KDJ.H, 70, 1) == false {
		t.Fatalf("KDJ height %v, want 70", panes.KDJ.H)
	}
	if near(panes.Volume.H, 55, 1) == false {
		t.Fatalf("volume height %v, want 55", panes.Volume.H)
	}
	// The bands must not overlap and must stay inside the box.
	if panes.Price.Y+panes.Price.H > panes.Volume.Y {
		t.Fatal("the price plot overlaps the volume strip")
	}
	if panes.Volume.Y+panes.Volume.H > panes.MACD.Y {
		t.Fatal("the volume strip overlaps the MACD band")
	}
	if panes.MACD.Y+panes.MACD.H > panes.KDJ.Y {
		t.Fatal("the MACD band overlaps the KDJ band")
	}
	if panes.KDJ.Y+panes.KDJ.H > panes.Time.Y+panes.Time.H+1 {
		t.Fatal("the bands run past the chart box")
	}
	if panes.Price.H <= 0 {
		t.Fatal("the price plot has no height")
	}
}

// TestPaneHeightsShrinksRatherThanBreaking records that asking for more
// than fits leaves a usable price plot instead of a negative one.
func TestPaneHeightsShrinksRatherThanBreaking(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:     []string{"A"},
		Period:      Period{Span: 1, Unit: UnitDay},
		Indicators:  []string{"MA", "MACD", "KDJ"},
		Source:      fixedSource{bars: hostBars(90)},
		Location:    ashareLocation,
		PaneHeights: PaneHeights{Sub: 4000, Volume: 4000},
	})
	panes := c.painted[0]
	if panes.Price.H <= 0 {
		t.Fatalf("price height %v under an impossible request", panes.Price.H)
	}
	if panes.KDJ.Y+panes.KDJ.H > panes.Time.Y+panes.Time.H+1 {
		t.Fatal("the bands run past the chart box")
	}
	// The compression is proportional: every band asked for the same
	// number, so every band must end up the same height. A fix that
	// clamped each band separately would pass "nothing crashed" and fail
	// here.
	if !near(panes.MACD.H, panes.KDJ.H, 1) || !near(panes.Volume.H, panes.KDJ.H, 1) {
		t.Fatalf("compression was not proportional: vol=%v macd=%v kdj=%v",
			panes.Volume.H, panes.MACD.H, panes.KDJ.H)
	}
	// The price plot keeps the documented quarter of the body, and the
	// whole layout still fills the box with only the gaps spare.
	body := panes.Time.Y - panes.Price.Y
	if panes.Price.H < body*0.2 {
		t.Fatalf("price plot %v is under a fifth of the body %v", panes.Price.H, body)
	}
	if gap := panes.Time.Y - (panes.KDJ.Y + panes.KDJ.H); !near(gap, float32(paneGap), 0.5) {
		t.Fatalf("gap before the time strip is %v, want %v", gap, paneGap)
	}
}

// TestPaneHeightsZeroKeepsTheBuiltInSplit records that the option is
// opt-in and the default layout is byte-identical.
func TestPaneHeightsZeroKeepsTheBuiltInSplit(t *testing.T) {
	opts := ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{"MA", "MACD", "KDJ"},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
	}
	a := hostChart(t, opts)
	withZero := opts
	withZero.PaneHeights = PaneHeights{}
	b := hostChart(t, withZero)
	if !samePanes(a.painted[0], b.painted[0]) {
		t.Fatalf("a zero PaneHeights changed the layout: %+v vs %+v", a.painted[0], b.painted[0])
	}
}

// samePanes compares two layouts including the Extra slices.
func samePanes(a, b Panes) bool {
	if a.Price != b.Price || a.Volume != b.Volume || a.MACD != b.MACD || a.KDJ != b.KDJ || a.Time != b.Time {
		return false
	}
	if a.PriceAxis != b.PriceAxis || a.VolumeAxis != b.VolumeAxis || a.MACDAxis != b.MACDAxis || a.KDJAxis != b.KDJAxis {
		return false
	}
	if len(a.Extra) != len(b.Extra) || len(a.ExtraAxis) != len(b.ExtraAxis) {
		return false
	}
	for i := range a.Extra {
		if a.Extra[i] != b.Extra[i] || a.ExtraAxis[i] != b.ExtraAxis[i] {
			return false
		}
	}
	return true
}

func near(got, want, tol float32) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d <= tol
}

// TestPaneHeightsVolumeOnlyWithNoSubPanes is the 0AMV panel shape: a MA
// line, a volume strip, and no sub-pane. The host sets Volume and leaves
// Sub at zero, which must not turn the volume override off or leave a
// gap where a sub-pane would have been.
func TestPaneHeightsVolumeOnlyWithNoSubPanes(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:     []string{"A"},
		Period:      Period{Span: 1, Unit: UnitDay},
		Indicators:  []string{"MA"},
		Source:      fixedSource{bars: hostBars(90)},
		Location:    ashareLocation,
		PaneHeights: PaneHeights{Volume: 34},
	})
	panes := c.painted[0]
	if near(panes.Volume.H, 34, 1) == false {
		t.Fatalf("volume height %v, want 34", panes.Volume.H)
	}
	if panes.MACD.H != 0 || panes.KDJ.H != 0 || len(panes.Extra) != 0 {
		t.Fatalf("a chart with no sub-pane allocated one: macd=%v kdj=%v extra=%v",
			panes.MACD.H, panes.KDJ.H, panes.Extra)
	}
	// The bands must fill the box exactly, with only the pane gaps spare.
	used := panes.Price.H + panes.Volume.H + panes.Time.H
	if gap := panes.Volume.Y - (panes.Price.Y + panes.Price.H); !near(gap, float32(paneGap), 0.5) {
		t.Fatalf("gap between price and volume is %v, want %v", gap, paneGap)
	}
	if gap := panes.Time.Y - (panes.Volume.Y + panes.Volume.H); !near(gap, float32(paneGap), 0.5) {
		t.Fatalf("gap between volume and time is %v, want %v", gap, paneGap)
	}
	if got := used + 2*float32(paneGap); !near(got, panes.Time.Y+panes.Time.H-panes.Price.Y, 0.5) {
		t.Fatalf("bands plus gaps cover %v, want %v", got, panes.Time.Y+panes.Time.H-panes.Price.Y)
	}
}

// TestPaneHeightsDrawsEverySubPaneCombination is the bug a host hit: a
// single open sub-pane borrows the MACD geometry before assignSubRects
// moves it onto KDJ, so applyPaneHeights saw an empty KDJ rect and put
// the band in Extra. The sub-pane then had a height but no indicator in
// it. Every combination must place its bands on the right fields.
func TestPaneHeightsDrawsEverySubPaneCombination(t *testing.T) {
	for _, tc := range []struct {
		name string
		inds []string
		macd bool
		kdj  bool
	}{
		{"KDJ alone", []string{"KDJ"}, false, true},
		{"MACD alone", []string{"MACD"}, true, false},
		{"MACD and KDJ", []string{"MACD", "KDJ"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := hostChart(t, ChartOptions{
				Symbols:     []string{"A"},
				Period:      Period{Span: 1, Unit: UnitDay},
				Indicators:  tc.inds,
				Source:      fixedSource{bars: hostBars(90)},
				Location:    ashareLocation,
				PaneHeights: PaneHeights{Sub: 65, Volume: 40},
			})
			panes := c.painted[0]
			if tc.macd && !near(panes.MACD.H, 65, 1) {
				t.Fatalf("MACD height %v, want 65", panes.MACD.H)
			}
			if tc.kdj && !near(panes.KDJ.H, 65, 1) {
				t.Fatalf("KDJ height %v, want 65 (was dropped into Extra %v)", panes.KDJ.H, panes.Extra)
			}
			if !tc.macd && panes.MACD.H != 0 {
				t.Fatalf("a closed MACD band has height %v", panes.MACD.H)
			}
			if !tc.kdj && panes.KDJ.H != 0 {
				t.Fatalf("a closed KDJ band has height %v", panes.KDJ.H)
			}
			if len(panes.Extra) != 0 {
				t.Fatalf("a built-in band landed in Extra: %v", panes.Extra)
			}
			// The band must actually paint: subLayers skips a rect at or
			// below 1 DIP, which is how the blank sub-pane presented.
			subs := c.enabledSubs()
			layers := subLayers(panes, subs, 0, c.Bars()-1)
			if len(layers) != len(subs) {
				t.Fatalf("%d of %d sub-panes would paint", len(layers), len(subs))
			}
			for _, layer := range layers {
				if layer.plot.H <= 1 {
					t.Fatalf("sub-pane %q would be skipped with height %v", layer.ind.Name(), layer.plot.H)
				}
			}
		})
	}
}

// TestPaneHeightsVolumeOnlyStillPlacesKDJ is the second report: setting
// only Volume also runs applyPaneHeights, so the KDJ band must survive a
// host that just wanted a shorter volume strip.
func TestPaneHeightsVolumeOnlyStillPlacesKDJ(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:     []string{"A"},
		Period:      Period{Span: 1, Unit: UnitDay},
		Indicators:  []string{"KDJ"},
		Source:      fixedSource{bars: hostBars(90)},
		Location:    ashareLocation,
		PaneHeights: PaneHeights{Volume: 40},
	})
	panes := c.painted[0]
	if !near(panes.Volume.H, 40, 1) {
		t.Fatalf("volume height %v, want 40", panes.Volume.H)
	}
	if panes.KDJ.H <= 1 {
		t.Fatalf("KDJ height %v with only Volume set; the band was dropped", panes.KDJ.H)
	}
	if len(panes.Extra) != 0 {
		t.Fatalf("the KDJ band landed in Extra: %v", panes.Extra)
	}
}

// TestIndicatorNilMeansDefaultsAndEmptyMeansNone pins the trap a host hit:
// a panel that computed its indicator list at runtime produced nil when MA
// was off, and ChartOptions read that as "use the defaults", so MA and
// MACD came back on. An empty non-nil slice is the way to say "none".
// SetIndicators is the opposite, and this test pins both directions so a
// future change cannot quietly make them agree.
func TestIndicatorNilMeansDefaultsAndEmptyMeansNone(t *testing.T) {
	base := ChartOptions{
		Symbols:  []string{"A"},
		Period:   Period{Span: 1, Unit: UnitDay},
		Source:   fixedSource{bars: hostBars(40)},
		Location: ashareLocation,
	}
	withNil := base
	withNil.Indicators = nil
	a := hostChart(t, withNil)
	if !a.IndicatorOn("MA") || !a.IndicatorOn("MACD") {
		t.Fatalf("nil Indicators should keep the defaults, got MA=%v MACD=%v",
			a.IndicatorOn("MA"), a.IndicatorOn("MACD"))
	}

	withEmpty := base
	withEmpty.Indicators = []string{}
	b := hostChart(t, withEmpty)
	if b.IndicatorOn("MA") || b.IndicatorOn("MACD") || b.IndicatorOn("KDJ") {
		t.Fatalf("an empty Indicators slice should turn everything off, got MA=%v MACD=%v KDJ=%v",
			b.IndicatorOn("MA"), b.IndicatorOn("MACD"), b.IndicatorOn("KDJ"))
	}

	// SetIndicators reads nil as "none" on both charts: the opposite of
	// the option above, and the same for a chart built either way.
	if err := a.SetIndicators(nil); err != nil {
		t.Fatalf("SetIndicators(nil): %v", err)
	}
	if a.IndicatorOn("MA") || a.IndicatorOn("MACD") {
		t.Fatalf("SetIndicators(nil) should turn everything off, got MA=%v MACD=%v",
			a.IndicatorOn("MA"), a.IndicatorOn("MACD"))
	}
}
