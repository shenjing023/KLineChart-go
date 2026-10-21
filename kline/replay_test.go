package kline

import (
	"errors"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// memBars is a DataSource whose whole history is the map it is given.
// KindForward returns nothing, so a chart cannot page in bars the test
// did not put in the map.
type memBars struct {
	by   map[string][]Candle
	gets int
}

func (m *memBars) GetBars(req BarRequest) ([]Candle, bool, error) {
	m.gets++
	if req.Kind == KindForward {
		return nil, false, nil
	}
	return cloneCandles(m.by[req.Symbol]), false, nil
}

func replayChart(t *testing.T, symbols []string, bars map[string][]Candle, layout string) (*Chart, *memBars) {
	t.Helper()
	src := &memBars{by: bars}
	c := NewChart(ChartOptions{
		Source:     src,
		Symbols:    symbols,
		Period:     Period{Span: 1, Unit: UnitHour},
		Layout:     layout,
		Indicators: []string{"MA", "MACD", "BOLL", "KDJ"},
	})
	if err := c.LoadError(); err != nil {
		t.Fatal(err)
	}
	return c, src
}

func hourAt(y int, m time.Month, d, h int) time.Time {
	return time.Date(y, m, d, h, 0, 0, 0, time.UTC)
}

func nearVal(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	diff := math.Abs(a - b)
	if diff <= 1e-9 {
		return true
	}
	scale := math.Max(math.Abs(a), math.Abs(b))
	return diff <= 1e-9*scale
}

func indicatorLines(ind Indicator) [][]float64 {
	switch v := ind.(type) {
	case *MA:
		return v.Lines()
	case *MACD:
		return [][]float64{v.DIF, v.DEA, v.Hist}
	case *BOLL:
		return [][]float64{v.Mid, v.Up, v.Low}
	case *KDJ:
		return [][]float64{v.K, v.D, v.J}
	default:
		return nil
	}
}

func sameBars(t *testing.T, got, want []Candle) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("bars %d, want %d", len(got), len(want))
	}
	for i := range want {
		g, w := got[i], want[i]
		if !g.Time.Equal(w.Time) || !nearVal(g.Open, w.Open) || !nearVal(g.High, w.High) || !nearVal(g.Low, w.Low) || !nearVal(g.Close, w.Close) || !nearVal(g.Volume, w.Volume) {
			t.Fatalf("bar %d got %+v want %+v", i, g, w)
		}
	}
}

func sameIndicators(t *testing.T, got, want *Chart) {
	t.Helper()
	if got.Bars() != want.Bars() {
		t.Fatalf("bars %d, prefix chart %d", got.Bars(), want.Bars())
	}
	for _, sym := range got.Symbols() {
		ig, iw := got.paneOf(sym), want.paneOf(sym)
		if ig < 0 || iw < 0 {
			t.Fatalf("symbol %s", sym)
		}
		sameBars(t, got.panes[ig].candles, want.panes[iw].candles)
		got.active(&got.panes[ig])
		want.active(&want.panes[iw])
		for s := range got.slots {
			if !got.slots[s].on {
				continue
			}
			name := got.slots[s].spec.Name
			gl := indicatorLines(got.panes[ig].inds[s])
			wl := indicatorLines(want.panes[iw].inds[s])
			if len(gl) != len(wl) {
				t.Fatalf("%s %s lines %d vs %d", sym, name, len(gl), len(wl))
			}
			for li := range wl {
				if len(gl[li]) != len(wl[li]) {
					t.Fatalf("%s %s line %d len %d vs %d", sym, name, li, len(gl[li]), len(wl[li]))
				}
				for i := range wl[li] {
					if !nearVal(gl[li][i], wl[li][i]) {
						t.Fatalf("%s %s line %d bar %d got %v want %v", sym, name, li, i, gl[li][i], wl[li][i])
					}
				}
			}
		}
		gLo, gHi := PriceDomain(got.panes[ig].candles, 0, len(got.panes[ig].candles)-1, lineExtras(got, ig)...)
		wLo, wHi := PriceDomain(want.panes[iw].candles, 0, len(want.panes[iw].candles)-1, lineExtras(want, iw)...)
		if !nearVal(gLo, wLo) || !nearVal(gHi, wHi) {
			t.Fatalf("%s price domain %v..%v, prefix %v..%v", sym, gLo, gHi, wLo, wHi)
		}
		gv := VolumeMax(got.panes[ig].candles, 0, len(got.panes[ig].candles)-1)
		wv := VolumeMax(want.panes[iw].candles, 0, len(want.panes[iw].candles)-1)
		if !nearVal(gv, wv) {
			t.Fatalf("%s volume max %v, prefix %v", sym, gv, wv)
		}
	}
}

func lineExtras(c *Chart, pane int) [][]float64 {
	var extra [][]float64
	for _, ind := range c.active(&c.panes[pane]) {
		if plot, ok := ind.(LinePlot); ok {
			extra = append(extra, plot.Series()...)
		}
	}
	return extra
}

func TestTailAppendMatchesFullCalc(t *testing.T) {
	cs := Generate(90, 5)
	for _, ind := range []Indicator{NewMA(5, 20), NewMACD(12, 26, 9), NewBOLL(20, 2), NewKDJ(9, 3, 3)} {
		ind.Calc(cs[:30])
		tail := ind.(BarTail)
		for i := 30; i < len(cs); i++ {
			tail.Append(cs[:i+1])
			if i%7 == 0 {
				prev := cs[i]
				cs[i].Close += 1.25
				if cs[i].High < cs[i].Close {
					cs[i].High = cs[i].Close
				}
				tail.Revise(cs[:i+1], prev)
			}
		}
		full := specNew(ind)
		full.Calc(cs)
		gl, wl := indicatorLines(ind), indicatorLines(full)
		for li := range wl {
			for i := range wl[li] {
				if !nearVal(gl[li][i], wl[li][i]) {
					t.Fatalf("%s line %d bar %d got %v want %v", ind.Name(), li, i, gl[li][i], wl[li][i])
				}
			}
		}
	}
}

func specNew(ind Indicator) Indicator {
	switch ind.(type) {
	case *MA:
		return NewMA(5, 20)
	case *MACD:
		return NewMACD(12, 26, 9)
	case *BOLL:
		return NewBOLL(20, 2)
	case *KDJ:
		return NewKDJ(9, 3, 3)
	default:
		return nil
	}
}

func TestAppendBarFollowsOnlyTheRightEdge(t *testing.T) {
	cs := Generate(180, 3)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": cs}, LayoutStack)
	tt := ui.NewTester(c.View, 960, 640)
	tt.Frame()
	if !c.pinnedRight() {
		t.Fatal("a fresh frame should sit on the right edge")
	}
	next := cs[len(cs)-1]
	next.Time = next.Time.Add(time.Hour)
	next.Close += 3
	if next.High < next.Close {
		next.High = next.Close
	}
	if err := c.AppendBar("600519", next); err != nil {
		t.Fatal(err)
	}
	if c.Bars() != len(cs)+1 {
		t.Fatalf("bars %d", c.Bars())
	}
	want := c.vp
	want.FitRight(c.Bars(), c.plotW)
	if math.Abs(c.vp.Offset-want.Offset) > 1e-9 {
		t.Fatalf("offset %v, fitted right edge %v", c.vp.Offset, want.Offset)
	}
	c.vp.Pan(280, c.plotW, c.Bars())
	stayed := c.vp.Offset
	if c.pinnedRight() {
		t.Fatal("pan should leave the right edge")
	}
	next.Time = next.Time.Add(time.Hour)
	next.Close += 1
	if err := c.AppendBar("600519", next); err != nil {
		t.Fatal(err)
	}
	if math.Abs(c.vp.Offset-stayed) > 1e-9 {
		t.Fatalf("panned offset moved from %v to %v", stayed, c.vp.Offset)
	}
	prev := c.panes[0].candles[len(c.panes[0].candles)-1]
	revised := prev
	revised.Close = prev.Close - 2
	if revised.Low > revised.Close {
		revised.Low = revised.Close
	}
	off := c.vp.Offset
	if err := c.UpdateLast("600519", revised); err != nil {
		t.Fatal(err)
	}
	if c.Bars() != len(cs)+2 || math.Abs(c.vp.Offset-off) > 1e-9 {
		t.Fatalf("update last bars %d offset %v, want %d and %v", c.Bars(), c.vp.Offset, len(cs)+2, off)
	}
	if err := c.AppendBar("600519", revised); !errors.Is(err, ErrBarTime) {
		t.Fatalf("same open time: %v", err)
	}
	if err := c.AppendBar("NOPE", next); !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("unknown symbol: %v", err)
	}
}

func TestAppendBarHoldsUntilEverySymbol(t *testing.T) {
	a := Generate(40, 1)
	b := Generate(40, 2)
	c, _ := replayChart(t, []string{"600519", "000001"}, map[string][]Candle{"600519": a, "000001": b}, LayoutStack)
	n := c.Bars()
	nextA := a[len(a)-1]
	nextA.Time = nextA.Time.Add(time.Hour)
	nextB := nextA
	nextB.Close = b[len(b)-1].Close
	if err := c.AppendBar("600519", nextA); err != nil {
		t.Fatal(err)
	}
	if c.Bars() != n {
		t.Fatalf("one symbol appended, bars %d want %d", c.Bars(), n)
	}
	if err := c.AppendBar("000001", nextB); err != nil {
		t.Fatal(err)
	}
	if c.Bars() != n+1 {
		t.Fatalf("bars %d", c.Bars())
	}
	if !c.panes[0].candles[n].Time.Equal(c.panes[1].candles[n].Time) {
		t.Fatal("promoted bars should share an open time")
	}
}

func TestAppendBarAmortizedAlloc(t *testing.T) {
	cs := Generate(8000, 9)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": cs}, LayoutStack)
	tt := ui.NewTester(c.View, 960, 640)
	tt.Frame()
	if c.Bars() < 1000 {
		t.Fatalf("chart loaded %d bars", c.Bars())
	}
	result := testing.Benchmark(func(b *testing.B) {
		// Each measured run continues from the bar the previous run
		// appended. Starting from a stale copy repeats an open time.
		bar := c.panes[0].candles[len(c.panes[0].candles)-1]
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bar.Time = bar.Time.Add(time.Hour)
			bar.Close += 0.01
			if err := c.AppendBar("600519", bar); err != nil {
				b.Fatal(err)
			}
		}
	})
	// A full Calc of MA, MACD, BOLL, and KDJ on 8000 bars is hundreds of
	// kilobytes. An amortized append stays in a few kilobytes.
	if result.N < 100 {
		t.Fatalf("benchmark ran %d iterations, bytes/op %d", result.N, result.AllocedBytesPerOp())
	}
	if result.AllocedBytesPerOp() > 8*1024 {
		t.Fatalf("append allocated %d bytes/op over %d iterations", result.AllocedBytesPerOp(), result.N)
	}
}

func TestReplayMatchesLoadedPrefix(t *testing.T) {
	fullA := Generate(120, 11)
	fullB := Generate(120, 12)
	for _, cs := range [][]Candle{fullA, fullB} {
		cs[100].High = 1e9
		cs[100].Low = -1e6
		cs[100].Close = 1e8
		cs[100].Volume = 1e12
	}
	const n = 80
	full := map[string][]Candle{"600519": fullA, "000001": fullB}
	prefix := map[string][]Candle{"600519": cloneCandles(fullA[:n]), "000001": cloneCandles(fullB[:n])}
	blind, _ := replayChart(t, []string{"600519", "000001"}, full, LayoutStack)
	replay, err := StartReplay(blind, full)
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(n); err != nil {
		t.Fatal(err)
	}
	plain, _ := replayChart(t, []string{"600519", "000001"}, prefix, LayoutStack)
	sameIndicators(t, blind, plain)
	if hi := mustDomainHi(blind); hi > 1e6 {
		t.Fatalf("price domain saw a hidden high: %v", hi)
	}
	_ = ui.NewTester(blind.View, 960, 720)
	_ = ui.NewTester(plain.View, 960, 720)
	for _, sym := range []string{"600519", "000001"} {
		gLo, gHi, gVol := visibleSpan(blind, sym)
		wLo, wHi, wVol := visibleSpan(plain, sym)
		if !nearVal(gLo, wLo) || !nearVal(gHi, wHi) || !nearVal(gVol, wVol) {
			t.Fatalf("%s visible domain %v..%v vol %v, prefix %v..%v vol %v", sym, gLo, gHi, gVol, wLo, wHi, wVol)
		}
	}
	blind.crossIndex = 40
	plain.crossIndex = 40
	if blind.Readout("600519") != plain.Readout("600519") {
		t.Fatalf("readout %q\nprefix %q", blind.Readout("600519"), plain.Readout("600519"))
	}
	if _, ok := blind.Bar("600519", n); ok {
		t.Fatal("bar at the cursor is still hidden")
	}
	for _, cs := range [][]Candle{blind.panes[0].candles, blind.panes[1].candles} {
		for _, bar := range cs {
			if !bar.Time.Before(fullA[n].Time) {
				t.Fatalf("revealed %s is not before the hidden bar", bar.Time)
			}
		}
	}
}

func mustDomainHi(c *Chart) float64 {
	_, hi := PriceDomain(c.panes[0].candles, 0, len(c.panes[0].candles)-1, lineExtras(c, 0)...)
	return hi
}

func visibleSpan(c *Chart, sym string) (lo, hi, vol float64) {
	i := c.paneOf(sym)
	cs := c.panes[i].candles
	first, last := c.vp.Visible(len(cs), c.plotW)
	lo, hi = PriceDomain(cs, first, last, lineExtras(c, i)...)
	vol = VolumeMax(cs, first, last)
	return lo, hi, vol
}

func TestReplayStepSeekAndBack(t *testing.T) {
	fullA := Generate(70, 4)
	fullB := Generate(70, 6)
	full := map[string][]Candle{"600519": fullA, "000001": fullB}
	c, _ := replayChart(t, []string{"600519", "000001"}, full, LayoutStack)
	replay, err := StartReplay(c, full)
	if err != nil {
		t.Fatal(err)
	}
	var revealed []time.Time
	replay.OnBar(func(ev ReplayBar) {
		revealed = append(revealed, ev.Time)
		if len(ev.Bars) != 2 || !ev.Bars[0].Time.Equal(ev.Time) || !ev.Bars[1].Time.Equal(ev.Time) {
			t.Fatalf("event %+v", ev)
		}
	})
	if err := replay.Step(3); err != nil {
		t.Fatal(err)
	}
	if replay.Cursor() != 3 || c.Bars() != 3 {
		t.Fatalf("cursor %d bars %d", replay.Cursor(), c.Bars())
	}
	if err := replay.SeekTo(fullA[10].Time); err != nil {
		t.Fatal(err)
	}
	if replay.Cursor() != 11 {
		t.Fatalf("seek time cursor %d", replay.Cursor())
	}
	if err := replay.SeekTo(fullA[0].Time.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if replay.Cursor() != 0 || c.Bars() != 0 {
		t.Fatalf("before first: cursor %d bars %d", replay.Cursor(), c.Bars())
	}
	if err := replay.SeekIndex(40); err != nil {
		t.Fatal(err)
	}
	if err := replay.StepBack(7); err != nil {
		t.Fatal(err)
	}
	if replay.Cursor() != 33 || c.Bars() != 33 {
		t.Fatalf("back cursor %d bars %d", replay.Cursor(), c.Bars())
	}
	stepped, _ := replayChart(t, []string{"600519", "000001"}, full, LayoutStack)
	other, err := StartReplay(stepped, full)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Step(33); err != nil {
		t.Fatal(err)
	}
	sameIndicators(t, c, stepped)
	prefix := map[string][]Candle{"600519": cloneCandles(fullA[:33]), "000001": cloneCandles(fullB[:33])}
	plain, _ := replayChart(t, []string{"600519", "000001"}, prefix, LayoutStack)
	sameIndicators(t, c, plain)
	if len(revealed) == 0 || !revealed[0].Equal(fullA[0].Time) {
		t.Fatalf("first event %v", revealed)
	}
	// StepBack does not report a bar. The forward seek to bar 10's open
	// reports the bars that were still hidden.
	n := len(revealed)
	if err := replay.StepBack(1); err != nil {
		t.Fatal(err)
	}
	if len(revealed) != n {
		t.Fatalf("step back fired %d events", len(revealed)-n)
	}
}

func TestReplayPartialHigherPeriod(t *testing.T) {
	// 08:00 through 12:00. The 10:00 low is hidden until that bar is
	// revealed, so a 4h bar built from the first two hours must not use it.
	base := []Candle{
		{Time: hourAt(2024, 3, 1, 8), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 10},
		{Time: hourAt(2024, 3, 1, 9), Open: 1.5, High: 5, Low: 1.2, Close: 4, Volume: 20},
		{Time: hourAt(2024, 3, 1, 10), Open: 4, High: 4.2, Low: 0.2, Close: 3, Volume: 30},
		{Time: hourAt(2024, 3, 1, 11), Open: 3, High: 3.5, Low: 2.5, Close: 3.2, Volume: 40},
		{Time: hourAt(2024, 3, 1, 12), Open: 3.2, High: 8, Low: 3, Close: 7, Volume: 50},
	}
	bars := map[string][]Candle{"600519": base, "000001": cloneCandles(base)}
	c, src := replayChart(t, []string{"600519", "000001"}, bars, LayoutStack)
	replay, err := StartReplay(c, bars)
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(2); err != nil {
		t.Fatal(err)
	}
	gets := src.gets
	if err := c.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	if src.gets != gets {
		t.Fatalf("SetPeriod during replay called GetBars %d extra times", src.gets-gets)
	}
	want := Candle{Time: hourAt(2024, 3, 1, 8), Open: 1, High: 5, Low: 0.5, Close: 4, Volume: 30}
	sameBars(t, c.panes[0].candles, []Candle{want})
	if err := replay.Step(2); err != nil {
		t.Fatal(err)
	}
	want.Low = 0.2
	want.Close = 3.2
	want.Volume = 100
	sameBars(t, c.panes[0].candles, []Candle{want})
	if err := replay.Step(1); err != nil {
		t.Fatal(err)
	}
	next := Candle{Time: hourAt(2024, 3, 1, 12), Open: 3.2, High: 8, Low: 3, Close: 7, Volume: 50}
	sameBars(t, c.panes[0].candles, []Candle{want, next})
	if err := replay.StepBack(1); err != nil {
		t.Fatal(err)
	}
	sameBars(t, c.panes[0].candles, []Candle{want})
	if err := c.SetPeriod(Period{Span: 15, Unit: UnitMinute}); !errors.Is(err, ErrReplayPeriod) {
		t.Fatalf("finer period: %v", err)
	}
	if c.Bars() != 1 {
		t.Fatalf("finer period changed bars to %d", c.Bars())
	}
	// 09:30 sits in the 08:00 4h bucket.
	late := []Candle{{Time: time.Date(2024, 3, 1, 9, 30, 0, 0, time.UTC), Open: 2, High: 3, Low: 1, Close: 2.5, Volume: 4}}
	lateBars := map[string][]Candle{"600519": late, "000001": cloneCandles(late)}
	lateChart, _ := replayChart(t, []string{"600519", "000001"}, lateBars, LayoutStack)
	lateReplay, err := StartReplay(lateChart, lateBars)
	if err != nil {
		t.Fatal(err)
	}
	if err := lateReplay.Step(1); err != nil {
		t.Fatal(err)
	}
	if err := lateChart.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	if got := lateChart.panes[0].candles[0].Time; !got.Equal(hourAt(2024, 3, 1, 8)) {
		t.Fatalf("09:30 bucket %s", got)
	}
}

func TestReplayPlayPauseUsesClock(t *testing.T) {
	cs := Generate(30, 8)
	bars := map[string][]Candle{"600519": cs}
	c, _ := replayChart(t, []string{"600519"}, bars, LayoutStack)
	replay, err := StartReplay(c, bars)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	replay.SetClock(func() time.Time { return now })
	var n int
	replay.OnBar(func(ReplayBar) { n++ })
	replay.Play(4)
	now = now.Add(250 * time.Millisecond)
	replay.Tick()
	if replay.Cursor() != 1 || !replay.Playing() {
		t.Fatalf("cursor %d playing %v", replay.Cursor(), replay.Playing())
	}
	now = now.Add(500 * time.Millisecond)
	replay.Tick()
	if replay.Cursor() != 3 || n != 3 {
		t.Fatalf("cursor %d events %d", replay.Cursor(), n)
	}
	replay.Pause()
	now = now.Add(2 * time.Second)
	replay.Tick()
	if replay.Cursor() != 3 || replay.Playing() {
		t.Fatalf("paused cursor %d playing %v", replay.Cursor(), replay.Playing())
	}
	replay.Play(2)
	now = now.Add(time.Second)
	replay.Tick()
	if replay.Cursor() != 5 {
		t.Fatalf("resume cursor %d", replay.Cursor())
	}
}

func TestReplayPanesMarkersDrawingsAndLiveFence(t *testing.T) {
	fullA := Generate(50, 2)
	fullB := Generate(50, 3)
	fullB[40].Close = 5000
	period := Period{Span: 1, Unit: UnitHour}
	src := &StubLiveSource{}
	if err := src.SetBars("600519", period, fullA); err != nil {
		t.Fatal(err)
	}
	if err := src.SetBars("000001", period, fullB); err != nil {
		t.Fatal(err)
	}
	c := NewChart(ChartOptions{
		Source:     src,
		Symbols:    []string{"600519", "000001"},
		Period:     period,
		Layout:     LayoutOverlay,
		Indicators: []string{"MA", "MACD", "BOLL"},
	})
	if err := c.LoadError(); err != nil {
		t.Fatal(err)
	}
	replay, err := StartReplay(c, map[string][]Candle{"600519": fullA, "000001": fullB})
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(20); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMarkers("600519", []Marker{{Time: fullA[30].Time, Side: SideBuy, Qty: 1}}); err != nil {
		t.Fatal(err)
	}
	if c.panes[0].marks[0].bar != -1 {
		t.Fatalf("future marker bar %d", c.panes[0].marks[0].bar)
	}
	c.crossIndex = c.Bars() - 1
	if strings.Contains(c.Readout("600519"), "B") {
		t.Fatalf("readout leaked a future marker: %s", c.Readout("600519"))
	}
	if err := replay.SeekIndex(31); err != nil {
		t.Fatal(err)
	}
	if c.panes[0].marks[0].bar != 30 {
		t.Fatalf("marker bar %d", c.panes[0].marks[0].bar)
	}
	c.crossIndex = 30
	if !strings.Contains(c.Readout("600519"), "B") {
		t.Fatalf("readout %s", c.Readout("600519"))
	}
	c.panes[0].overlays.Add(OverlaySegment, Point{BarIndex: 4, Price: 10}, Point{BarIndex: 28, Price: 12})
	if err := replay.SeekIndex(6); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(c.View, 960, 720)
	tt.Frame()
	if len(c.panes[0].overlays.Items()) != 1 {
		t.Fatal("step back dropped the drawing")
	}
	if c.Layout() != LayoutOverlay {
		t.Fatal("layout")
	}
	if len(c.panes[1].candles) != c.Bars() {
		t.Fatalf("compare series %d, primary %d", len(c.panes[1].candles), c.Bars())
	}
	for _, bar := range c.panes[1].candles {
		if bar.Close > 1000 {
			t.Fatalf("compare close %v is a hidden bar", bar.Close)
		}
	}
	pushed := fullA[len(fullA)-1]
	pushed.Time = pushed.Time.Add(time.Hour)
	pushed.Close = 99999
	src.Push("600519", period, pushed)
	src.Push("000001", period, pushed)
	if c.Bars() != replay.Cursor() {
		t.Fatalf("live push changed replay bars to %d, cursor %d", c.Bars(), replay.Cursor())
	}
	for _, cs := range [][]Candle{c.panes[0].candles, c.panes[1].candles} {
		for _, bar := range cs {
			if bar.Close > 1000 {
				t.Fatal("live push leaked into the revealed series")
			}
		}
	}
	before := runtime.NumGoroutine()
	replay.Play(8)
	replay.Pause()
	replay.Close()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := replay.Step(1); !errors.Is(err, ErrReplayClosed) {
		t.Fatalf("step after close: %v", err)
	}
	if got := runtime.NumGoroutine(); got != before {
		t.Fatalf("goroutines %d, before close %d", got, before)
	}
}

func TestAppendBarRejectedDuringReplay(t *testing.T) {
	base := Generate(60, 7)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": base}, LayoutStack)
	replay, err := StartReplay(c, map[string][]Candle{"600519": base})
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(5); err != nil {
		t.Fatal(err)
	}
	for _, bar := range []Candle{
		base[40],                             // a revealed-suffix bar
		{Time: base[59].Time.Add(time.Hour)}, // past the end of the series
	} {
		if err := c.AppendBar("600519", bar); !errors.Is(err, ErrReplayActive) {
			t.Fatalf("AppendBar(%s) = %v, want ErrReplayActive", bar.Time, err)
		}
	}
	if c.Bars() != 5 || replay.Cursor() != 5 {
		t.Fatalf("bars %d cursor %d, want 5 and 5", c.Bars(), replay.Cursor())
	}
	if err := replay.Step(1); err != nil {
		t.Fatalf("the replay is wedged: %v", err)
	}
	if c.Bars() != 6 || replay.Cursor() != 6 {
		t.Fatalf("step after a refused append: bars %d cursor %d", c.Bars(), replay.Cursor())
	}
	if !c.panes[0].candles[5].Time.Equal(base[5].Time) {
		t.Fatalf("bar 5 is %s, want the revealed %s", c.panes[0].candles[5].Time, base[5].Time)
	}
}

func TestUpdateLastRejectedDuringReplay(t *testing.T) {
	base := Generate(60, 7)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": base}, LayoutStack)
	replay, err := StartReplay(c, map[string][]Candle{"600519": base})
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(5); err != nil {
		t.Fatal(err)
	}
	revealed := c.panes[0].candles[4].Close
	hidden := base[40]
	hidden.Time = c.panes[0].candles[4].Time
	if err := c.UpdateLast("600519", hidden); !errors.Is(err, ErrReplayActive) {
		t.Fatalf("UpdateLast = %v, want ErrReplayActive", err)
	}
	if got := c.panes[0].candles[4].Close; got != revealed {
		t.Fatalf("a hidden value replaced the revealed close: %v -> %v", revealed, got)
	}
}

// A replay appends through the private path, so the public fence must not
// stop it revealing bars.
func TestReplayRevealsThroughPrivateAppend(t *testing.T) {
	base := Generate(40, 7)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": base}, LayoutStack)
	replay, err := StartReplay(c, map[string][]Candle{"600519": base})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10; i++ {
		if err := replay.Step(1); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if c.Bars() != i {
			t.Fatalf("step %d: bars %d", i, c.Bars())
		}
	}
	sameBars(t, c.panes[0].candles, base[:10])
	if err := replay.StepBack(3); err != nil {
		t.Fatal(err)
	}
	sameBars(t, c.panes[0].candles, base[:7])
}

// Playback must stop when a reveal cannot happen, or View asks for frames
// forever on a replay that can never advance.
func TestReplayTickStopsWhenRevealFails(t *testing.T) {
	base := Generate(60, 7)
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": base}, LayoutStack)
	replay, err := StartReplay(c, map[string][]Candle{"600519": base})
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(5); err != nil {
		t.Fatal(err)
	}
	// Corrupt the edge behind the chart's back, the way a stray writer
	// would: the next reveal is refused by AppendBar's edge check.
	c.panes[0].candles = append(c.panes[0].candles, base[40])
	now := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	replay.SetClock(func() time.Time { return now })
	replay.Play(1000)
	now = now.Add(time.Second) // one second at 1000 bars/s is due
	before := replay.Cursor()
	replay.Tick()
	if replay.Playing() {
		t.Fatal("Playing stayed true although no bar can be revealed")
	}
	if replay.Cursor() != before {
		t.Fatalf("cursor moved from %d to %d", before, replay.Cursor())
	}
}

// Starting a replacement replay from OnBar transfers ownership at the
// callback boundary. The old batch must not reveal its remaining bars
// into the replacement's empty prefix.
func TestReplayCallbackReplacementStopsOldAdvance(t *testing.T) {
	base := Generate(20, 7)
	all := map[string][]Candle{"600519": base}
	c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
	old, err := StartReplay(c, all)
	if err != nil {
		t.Fatal(err)
	}
	var replacement *Replay
	callbacks := 0
	old.OnBar(func(ReplayBar) {
		callbacks++
		old.OnBar(nil)
		replacement, err = StartReplay(c, all)
		if err != nil {
			t.Fatal(err)
		}
	})
	if err := old.Step(4); !errors.Is(err, ErrReplayClosed) {
		t.Fatalf("Step after replacement = %v, want ErrReplayClosed", err)
	}
	if replacement == nil || c.replay != replacement {
		t.Fatal("the replacement replay is not attached")
	}
	if callbacks != 1 || old.Cursor() != 1 {
		t.Fatalf("callbacks %d old cursor %d, want 1 and 1", callbacks, old.Cursor())
	}
	if replacement.Cursor() != 0 || c.Bars() != 0 {
		t.Fatalf("replacement cursor %d bars %d, want an empty prefix", replacement.Cursor(), c.Bars())
	}
}

// Close from OnBar is terminal for the current batch. The already
// revealed bar stays visible, but no later bar is written.
func TestReplayCallbackCloseStopsBatch(t *testing.T) {
	base := Generate(20, 9)
	all := map[string][]Candle{"600519": base}
	c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
	r, err := StartReplay(c, all)
	if err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	r.OnBar(func(ReplayBar) {
		callbacks++
		r.Close()
	})
	if err := r.Step(5); !errors.Is(err, ErrReplayClosed) {
		t.Fatalf("Step after callback Close = %v, want ErrReplayClosed", err)
	}
	if callbacks != 1 || r.Cursor() != 1 || c.Bars() != 1 {
		t.Fatalf("callbacks %d cursor %d bars %d, want 1", callbacks, r.Cursor(), c.Bars())
	}
	sameBars(t, c.panes[0].candles, base[:1])
}

// Tick may have several due bars in one frame. Pause from the callback
// stops the batch at the bar that was just revealed.
func TestReplayCallbackPauseStopsTick(t *testing.T) {
	base := Generate(30, 11)
	all := map[string][]Candle{"600519": base}
	c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
	r, err := StartReplay(c, all)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	r.SetClock(func() time.Time { return now })
	r.OnBar(func(ReplayBar) { r.Pause() })
	r.Play(10)
	now = now.Add(500 * time.Millisecond)
	r.Tick()
	if r.Playing() || r.Cursor() != 1 || c.Bars() != 1 {
		t.Fatalf("playing %v cursor %d bars %d, want paused at 1", r.Playing(), r.Cursor(), c.Bars())
	}
	now = now.Add(time.Second)
	r.Tick()
	if r.Cursor() != 1 || c.Bars() != 1 {
		t.Fatalf("paused Tick advanced to cursor %d bars %d", r.Cursor(), c.Bars())
	}
}

func TestReplayStepFollowsEdgeUnlessPanned(t *testing.T) {
	cs := Generate(160, 15)
	bars := map[string][]Candle{"600519": cs}
	c, _ := replayChart(t, []string{"600519"}, bars, LayoutStack)
	replay, err := StartReplay(c, bars)
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.SeekIndex(120); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(c.View, 960, 640)
	tt.Frame()
	if err := replay.Step(1); err != nil {
		t.Fatal(err)
	}
	fitted := c.vp
	fitted.FitRight(c.Bars(), c.plotW)
	if math.Abs(c.vp.Offset-fitted.Offset) > 1e-6 {
		t.Fatalf("step offset %v, right edge %v", c.vp.Offset, fitted.Offset)
	}
	c.vp.Pan(240, c.plotW, c.Bars())
	stayed := c.vp.Offset
	if err := replay.Step(1); err != nil {
		t.Fatal(err)
	}
	if math.Abs(c.vp.Offset-stayed) > 1e-9 {
		t.Fatalf("panned step moved offset from %v to %v", stayed, c.vp.Offset)
	}
}
