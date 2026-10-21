// Package kline_test holds runnable examples for the chart library. Each
// Example is compiled and run by `go test`, so an example that drifts
// from the exported API fails the build instead of rotting in a doc.
package kline_test

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

// ExampleNewChart is the smallest embed: a chart with the demo defaults,
// painted by ui.View. A window would wrap this in mygo.WindowOptions:
//
//	mygo.NewWindow(mygo.WindowOptions{
//		Title:   "Chart",
//		Width:   960,
//		Height:  720,
//		Content: ui.View(chart.View),
//	})
//
// The example itself only builds the view, because a test must not open
// a window.
func ExampleNewChart() {
	chart := kline.NewChart(kline.ChartOptions{})
	if err := chart.LoadError(); err != nil {
		fmt.Println("load:", err)
		return
	}
	fmt.Println("bars:", chart.Bars())
	fmt.Println("symbols:", chart.Symbols())
	fmt.Println("period:", chart.Period())
	fmt.Println("layout:", chart.Layout())
	fmt.Printf("view is %T\n", ui.View(chart.View))
	// Output:
	// bars: 600
	// symbols: [600519 000001]
	// period: 1h
	// layout: stack
	// view is *ui.Content
}

// ExampleChartOptions shows the options a host usually sets: its own
// source, two symbols, a period, one indicator, and a fixed precision and
// time zone for the header text.
func ExampleChartOptions() {
	digits := 3
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		// A machine without the tz database falls back to UTC.
		shanghai = time.UTC
	}
	chart := kline.NewChart(kline.ChartOptions{
		Source:        &kline.SyntheticSource{},
		Symbols:       []string{"600519", "000001"},
		Period:        kline.Period{Span: 4, Unit: kline.UnitHour},
		Layout:        kline.LayoutStack,
		Indicators:    []string{"MA"},
		Magnet:        kline.MagnetWeak,
		HideHeader:    true, // the host draws its own bar
		PriceDecimals: &digits,
		Location:      shanghai,
	})
	fmt.Println("period:", chart.Period())
	fmt.Println("layout:", chart.Layout())
	fmt.Println("indicator MA on:", chart.IndicatorOn("MA"))
	fmt.Println("indicator MACD on:", chart.IndicatorOn("MACD"))
	fmt.Println("magnet weak:", chart.Magnet() == kline.MagnetWeak)
	fmt.Println("readout is not empty:", chart.Readout("600519") != "")
	// Output:
	// period: 4h
	// layout: stack
	// indicator MA on: true
	// indicator MACD on: false
	// magnet weak: true
	// readout is not empty: true
}

// ExampleDataSource is the minimal source a host implements for its own
// history. Init returns the newest window; forward returns older bars
// strictly before Anchor; both report whether more history remains.
func ExampleDataSource() {
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	start := time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)
	all := make([]kline.Candle, 5)
	for i := range all {
		all[i] = kline.Candle{Time: start.Add(time.Duration(i) * time.Hour), Close: float64(10 + i)}
	}
	src := &memorySource{all: all}
	init, more, err := src.GetBars(kline.BarRequest{Symbol: "A", Period: hour, Kind: kline.KindInit, Limit: 2})
	fmt.Println("init:", len(init), more, err)
	older, more, err := src.GetBars(kline.BarRequest{Symbol: "A", Period: hour, Kind: kline.KindForward, Anchor: init[0].Time, Limit: 2})
	fmt.Println("forward:", len(older), more, err)
	oldest, more, err := src.GetBars(kline.BarRequest{Symbol: "A", Period: hour, Kind: kline.KindForward, Anchor: older[0].Time, Limit: 2})
	fmt.Println("oldest:", len(oldest), more, err)
	fmt.Println("strict before:", older[len(older)-1].Time.Before(init[0].Time))
	// Output:
	// init: 2 true <nil>
	// forward: 2 true <nil>
	// oldest: 1 false <nil>
	// strict before: true
}

// ExampleRegister installs one host indicator and shows the kind check.
// Register is process-wide; the example removes its name afterwards.
func ExampleRegister() {
	const name = "EXAMPLE_MEAN"
	kline.Unregister(name)
	defer kline.Unregister(name)
	spec := kline.Spec{Name: name, Kind: kline.PaneMain, New: func() kline.Indicator { return &exampleMean{} }}
	fmt.Println("register:", kline.Register(spec) == nil)
	fmt.Println("duplicate:", errors.Is(kline.Register(spec), kline.ErrDuplicateIndicator))
	mismatch := kline.Spec{Name: name + "_BAD", Kind: kline.PaneSub,
		New: func() kline.Indicator { return &exampleMean{} }}
	fmt.Println("kind mismatch:", errors.Is(kline.Register(mismatch), kline.ErrIndicatorKind))
	// Output:
	// register: true
	// duplicate: true
	// kind mismatch: true
}

// ExampleChart_SyncIndicators installs a late registration and removes it
// again. Existing toggle pointers stay valid across both syncs.
func ExampleChart_SyncIndicators() {
	const name = "EXAMPLE_SYNC"
	kline.Unregister(name)
	defer kline.Unregister(name)
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{"MA"}})
	ma := chart.IndicatorEnabled("MA")
	fmt.Println("before:", chart.IndicatorEnabled(name) == nil)
	err := kline.Register(kline.Spec{Name: name, Kind: kline.PaneMain,
		New: func() kline.Indicator { return &exampleMean{} }})
	if err != nil {
		panic(err)
	}
	chart.SyncIndicators()
	fmt.Println("added off:", chart.IndicatorEnabled(name) != nil && !chart.IndicatorOn(name))
	fmt.Println("old pointer:", chart.IndicatorEnabled("MA") == ma && chart.IndicatorOn("MA"))
	kline.Unregister(name)
	chart.SyncIndicators()
	fmt.Println("removed:", chart.IndicatorEnabled(name) == nil)
	// Output:
	// before: true
	// added off: true
	// old pointer: true
	// removed: true
}

// ExampleChart_View draws the chart inside a host view. The host keeps
// its own chrome and calls View where the plot belongs.
func ExampleChart_View() {
	chart := kline.NewChart(kline.ChartOptions{
		Indicators: []string{"MA", "MACD"},
	})
	host := func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Text(c, "my toolbar")
			ui.Column(c).Grow(1).Children(func() {
				chart.View(c)
			})
		})
	}
	// A test drives the view without a window; a window passes host
	// straight to ui.View.
	tester := ui.NewTester(host, 960, 720)
	tester.Frame()
	fmt.Println("toolbar painted:", tester.HasText("my toolbar"))
	fmt.Println("header painted:", tester.HasText("600519"))
	// Output:
	// toolbar painted: true
	// header painted: true
}

// ExampleChart_SetPeriod switches the bar size. A successful change
// reloads from the source and drops drawings; asking for the period
// already loaded is a no-op.
func ExampleChart_SetPeriod() {
	chart := kline.NewChart(kline.ChartOptions{Indicators: []string{}})
	hour := chart.Period()
	if err := chart.SetPeriod(kline.Period{Span: 1, Unit: kline.UnitDay}); err != nil {
		fmt.Println("set period:", err)
		return
	}
	fmt.Println("period:", chart.Period())
	fmt.Println("bars:", chart.Bars())
	if err := chart.SetPeriod(hour); err != nil {
		fmt.Println("back to 1h:", err)
	}
	fmt.Println("period:", chart.Period())
	err := chart.SetPeriod(kline.Period{Span: 0, Unit: kline.UnitHour})
	fmt.Println("invalid period:", err)
	fmt.Println("period kept:", chart.Period())
	// Output:
	// period: 1d
	// bars: 600
	// period: 1h
	// invalid period: kline: invalid period
	// period kept: 1h
}

// ExampleChart_SetLayout puts both symbols on one price plot, indexed to
// 100 at the first visible bar where both closes are positive.
func ExampleChart_SetLayout() {
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"600519", "000001"},
		Indicators: []string{"MA"},
	})
	if err := chart.SetLayout(kline.LayoutOverlay); err != nil {
		fmt.Println("set layout:", err)
		return
	}
	fmt.Println("layout:", chart.Layout())
	// Overlay is a two-series mode: the header carries the index.
	fmt.Println("readout has idx:", containsIdx(chart.Readout("600519")))
	err := chart.SetLayout("sideways")
	fmt.Println("unknown layout:", err)
	fmt.Println("layout kept:", chart.Layout())
	// Output:
	// layout: overlay
	// readout has idx: true
	// unknown layout: kline: unknown layout: sideways
	// layout kept: overlay
}

func containsIdx(s string) bool {
	for i := 0; i+2 < len(s); i++ {
		if s[i] == 'i' && s[i+1] == 'd' && s[i+2] == 'x' {
			return true
		}
	}
	return false
}

// ExampleChart_SetIndicators turns a fixed set of indicators on and off.
// Nil and an empty slice both mean "none", unlike ChartOptions.Indicators
// where nil keeps the defaults.
func ExampleChart_SetIndicators() {
	chart := kline.NewChart(kline.ChartOptions{})
	fmt.Println("default:", chart.IndicatorOn("MA"), chart.IndicatorOn("MACD"), chart.IndicatorOn("BOLL"))

	if err := chart.SetIndicators([]string{"MA", "BOLL"}); err != nil {
		fmt.Println("set indicators:", err)
		return
	}
	fmt.Println("explicit:", chart.IndicatorOn("MA"), chart.IndicatorOn("MACD"), chart.IndicatorOn("BOLL"))

	if err := chart.SetIndicators(nil); err != nil {
		fmt.Println("clear indicators:", err)
	}
	fmt.Println("cleared:", chart.IndicatorOn("MA"), chart.IndicatorOn("BOLL"))

	// A ui.Toggle binds the returned *bool directly.
	flag := chart.IndicatorEnabled("KDJ")
	*flag = true
	fmt.Println("toggle bound:", chart.IndicatorOn("KDJ"))

	err := chart.SetIndicators([]string{"NOPE"})
	fmt.Println("unknown:", err)
	fmt.Println("unchanged:", chart.IndicatorOn("KDJ"))
	// Output:
	// default: true true false
	// explicit: true false true
	// cleared: false false
	// toggle bound: true
	// unknown: kline: unknown indicator: NOPE
	// unchanged: true
}

// ExampleChart_Bar reads the series back: a host uses it to draw its own
// table, or to hand a time to SetMarkers.
func ExampleChart_Bar() {
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"600519"},
		Indicators: []string{},
	})
	last, ok := chart.Bar("600519", chart.Bars()-1)
	fmt.Println("found:", ok)
	fmt.Println("high above low:", last.High > last.Low)
	fmt.Println("time is Shanghai:", last.Time.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("15:04") == "14:00")

	_, ok = chart.Bar("600519", chart.Bars())
	fmt.Println("out of range:", ok)
	_, ok = chart.Bar("NOPE", 0)
	fmt.Println("unknown symbol:", ok)
	// Output:
	// found: true
	// high above low: true
	// time is Shanghai: true
	// out of range: false
	// unknown symbol: false
}

// ExampleChart_SetMarkers places buy and sell arrows. Time picks the bar
// whose open interval holds it, so the marker survives a period change
// and a forward prepend.
func ExampleChart_SetMarkers() {
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"600519"},
		Indicators: []string{},
	})
	buy, ok := chart.Bar("600519", chart.Bars()-3)
	if !ok {
		fmt.Println("no bar")
		return
	}
	sell, _ := chart.Bar("600519", chart.Bars()-1)

	err := chart.SetMarkers("600519", []kline.Marker{
		{Time: buy.Time, Side: kline.SideBuy, Qty: 100},
		{Time: sell.Time, Side: kline.SideSell, Price: sell.Close, Label: "exit"},
	})
	if err != nil {
		fmt.Println("set markers:", err)
		return
	}
	marks, err := chart.Markers("600519")
	fmt.Println("markers:", len(marks), err)
	fmt.Println("second is a sell:", marks[1].Side == kline.SideSell)
	fmt.Println("label kept:", marks[1].Label)

	err = chart.AppendMarker("600519", kline.Marker{Time: buy.Time, Side: kline.SideBuy, Qty: 50})
	fmt.Println("append:", err == nil)
	marks, _ = chart.Markers("600519")
	fmt.Println("after append:", len(marks))

	err = chart.SetMarkers("600519", []kline.Marker{{Time: buy.Time, Side: kline.MarkerSide(99)}})
	fmt.Println("bad side:", err)
	marks, _ = chart.Markers("600519")
	fmt.Println("list kept:", len(marks))
	// Output:
	// markers: 2 <nil>
	// second is a sell: true
	// label kept: exit
	// append: true
	// after append: 3
	// bad side: kline: unknown marker side: 99
	// list kept: 3
}

// Example_periods validates a bar size and renders its duration.
func ExamplePeriod() {
	for _, p := range []kline.Period{
		{Span: 15, Unit: kline.UnitMinute},
		{Span: 4, Unit: kline.UnitHour},
		{Span: 1, Unit: kline.UnitDay},
	} {
		d, err := p.Duration()
		fmt.Printf("%s valid=%v duration=%v err=%v\n", p, p.Valid(), d, err)
	}
	bad := kline.Period{Span: 0, Unit: "week"}
	fmt.Printf("%s valid=%v\n", bad, bad.Valid())
	// Output:
	// 15m valid=true duration=15m0s err=<nil>
	// 4h valid=true duration=4h0m0s err=<nil>
	// 1d valid=true duration=24h0m0s err=<nil>
	// 0 week valid=false
}

// ExampleFormatPrice shows the price formatting the chart uses, and the
// digits variant a host passes through ChartOptions.PriceDecimals.
func ExampleFormatPrice() {
	fmt.Println(kline.FormatPrice(1234.5))
	// Zero digits rounds to the nearest integer (ties to even).
	fmt.Println(kline.FormatPriceDigits(1234.5, 0))
	fmt.Println(kline.FormatPriceDigits(1234.5678, 3))
	// A non-finite price prints an em dash.
	fmt.Println(kline.FormatPrice(math.NaN()))

	bar := kline.Candle{Time: time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC), Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 5000}
	// A NaN moving average is omitted from the line.
	fmt.Println(kline.FormatBar(bar, 100.25, math.NaN()))
	// Output:
	// 1234.50
	// 1234
	// 1234.568
	// —
	// 2024-01-02 09:30  O 100.00  H 101.00  L 99.00  C 100.50  V 5000  MA5 100.25
}

// ExampleViewport shows the shared window math a host needs when it
// paints or hit-tests its own chart chrome.
func ExampleViewport() {
	var vp kline.Viewport
	vp.FitRight(100, 800)
	first, last := vp.Visible(100, 800)
	fmt.Printf("fit bars: %d %d, right gap: %.0f\n", first, last, 800-98*vp.BarWidth)
	index, ok := vp.IndexAt(vp.BarLeft(20)+vp.BarWidth/2, 100)
	fmt.Printf("bar 20 left: %.0f, index at center: %d %v\n", vp.BarLeft(20), index, ok)
	vp.Pan(160, 800, 100)
	first, last = vp.Visible(100, 800)
	fmt.Printf("after pan right: %d %d\n", first, last)
	anchor := vp.BarLeft(40) + vp.BarWidth/2
	vp.Zoom(-120, anchor, 800, 100)
	fmt.Printf("zoom keeps anchor %.0f at %.0f\n", anchor, vp.BarLeft(40)+vp.BarWidth/2)
	vp.Clamp(100, 800)
	first, last = vp.Visible(100, 800)
	fmt.Printf("after zoom, visible: %d %d\n", first, last)
	scale := chart.NewLinear(90, 110)
	y := kline.PriceToLocalY(200, 100, scale)
	fmt.Printf("price 100 at y %.0f, back to %.0f\n", y, kline.LocalYToPrice(200, y, scale))
	// Output:
	// fit bars: 2 99, right gap: 16
	// bar 20 left: 144, index at center: 20 true
	// after pan right: 0 97
	// zoom keeps anchor 340 at 340
	// after zoom, visible: 13 76
	// price 100 at y 100, back to 100
}

// ExamplePriceDomain shows finite-value filtering and indicator
// expansion. A NaN in the window is skipped, not propagated into the
// scale.
func ExamplePriceDomain() {
	bars := []kline.Candle{
		{Open: 100, High: 102, Low: 99, Close: 101},
		{Open: 101, High: math.Inf(1), Low: 100, Close: 102},
	}
	lo, hi := kline.PriceDomain(bars, 0, 1, []float64{98})
	fmt.Printf("domain: %.2f %.2f\n", lo, hi)
	lo, hi = kline.PriceDomain(nil, 0, 0)
	fmt.Printf("empty: %.0f %.0f\n", lo, hi)
	// Output:
	// domain: 97.68 102.32
	// empty: 0 1
}

// ExampleVolumeMax filters non-finite and non-positive volumes. The
// result leaves 8% headroom above the maximum.
func ExampleVolumeMax() {
	bars := []kline.Candle{{Volume: 100}, {Volume: math.NaN()}, {Volume: 200}, {Volume: math.Inf(1)}}
	fmt.Printf("%.0f\n", kline.VolumeMax(bars, 0, len(bars)-1))
	fmt.Println(kline.VolumeMax(nil, 0, 0))
	// Output:
	// 216
	// 1
}

// ExampleChart_SetTool selects a built-in tool and leaves the current
// tool unchanged when the name is unknown.
func ExampleChart_SetTool() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	fmt.Println("initial:", chart.Tool())
	fmt.Println("segment:", chart.SetTool(kline.ToolSegment), chart.Tool())
	fmt.Println("unknown:", errors.Is(chart.SetTool("not-a-tool"), kline.ErrUnknownTool), chart.Tool())
	fmt.Println("pan:", chart.SetTool(kline.ToolPan), chart.Tool())
	// Output:
	// initial: pan
	// segment: <nil> segment
	// unknown: true segment
	// pan: <nil> pan
}

// ExampleChart_SetMagnet selects snapping and leaves the current mode
// unchanged when the value is unknown.
func ExampleChart_SetMagnet() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	fmt.Println("initial off:", chart.Magnet() == kline.MagnetOff)
	fmt.Println("strong:", chart.SetMagnet(kline.MagnetStrong), chart.Magnet() == kline.MagnetStrong)
	fmt.Println("unknown:", errors.Is(chart.SetMagnet(kline.MagnetMode(99)), kline.ErrUnknownMagnet), chart.Magnet() == kline.MagnetStrong)
	// Output:
	// initial off: true
	// strong: <nil> true
	// unknown: true true
}

// ExampleChart_SetHollow shows the hollow-body switch and the bindable
// flag a host toolbar can use.
func ExampleChart_SetHollow() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	fmt.Println("initial:", chart.Hollow())
	flag := chart.HollowEnabled()
	chart.SetHollow(true)
	fmt.Println("after set:", chart.Hollow(), *flag)
	*flag = false
	fmt.Println("after binding:", chart.Hollow())
	// Output:
	// initial: false
	// after set: true true
	// after binding: false
}

// ExampleChart_SetLimitPct shows limit coloring's threshold. A negative
// value turns limit colors off.
func ExampleChart_SetLimitPct() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	chart.SetLimitPct(0.10)
	fmt.Printf("on: %.2f\n", chart.LimitPct())
	chart.SetLimitPct(-1)
	fmt.Printf("off: %.0f\n", chart.LimitPct())
	// Output:
	// on: 0.10
	// off: 0
}

// ExampleChart_ResetView restores the default bar width. The next paint
// fits the latest bars again.
func ExampleChart_ResetView() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	tester := ui.NewTester(chart.View, 900, 500)
	tester.Frame()
	first, last := chart.Visible()
	fmt.Printf("visible: %d..%d\n", first, last)
	chart.ResetView()
	tester.Frame()
	first, last = chart.Visible()
	fmt.Printf("refitted: %d..%d\n", first, last)
	// Output:
	// visible: 498..599
	// refitted: 498..599
}

// ExampleChart_Symbols shows the host read-only series views. Bars is
// the shared count; Bar returns false outside a symbol's window.
func ExampleChart_Symbols() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	fmt.Println("symbols:", chart.Symbols())
	fmt.Println("bars:", chart.Bars())
	fmt.Println("unknown bar:", func() bool { _, ok := chart.Bar("missing", 0); return ok }())
	fmt.Println("unknown readout:", chart.Readout("missing") == "")
	// Output:
	// symbols: [600519]
	// bars: 600
	// unknown bar: false
	// unknown readout: true
}

// ExampleChart_AlignError shows a host checking feed alignment after a
// two-symbol load and a push.
func ExampleChart_AlignError() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"A", "B"}, Indicators: []string{}})
	defer chart.Close()
	last, _ := chart.Bar("A", chart.Bars()-1)
	fmt.Println("aligned:", chart.AlignError() == nil)
	fmt.Println("A append:", chart.AppendBar("A", kline.Candle{Time: last.Time.Add(2 * time.Hour), Close: 12}))
	fmt.Println("B append:", chart.AppendBar("B", kline.Candle{Time: last.Time.Add(time.Hour), Close: 11}))
	fmt.Println("mismatch reported:", errors.Is(chart.AlignError(), kline.ErrMisaligned))
	// Output:
	// aligned: true
	// A append: <nil>
	// B append: <nil>
	// mismatch reported: true
}

// ExampleChart_SetIndicator toggles one indicator without rebuilding the
// chart. IndicatorEnabled returns the same flag a ui.Toggle can bind.
func ExampleChart_SetIndicator() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{"MA"}})
	fmt.Println("names:", chart.IndicatorNames())
	fmt.Println("MA:", chart.IndicatorOn("MA"), "vol:", chart.IndicatorOn("VOL"))
	flag := chart.IndicatorEnabled("BOLL")
	fmt.Println("BOLL pointer:", flag != nil)
	if err := chart.SetIndicator("BOLL", true); err != nil {
		panic(err)
	}
	fmt.Println("BOLL:", chart.IndicatorOn("BOLL"), *flag)
	fmt.Println("unknown:", errors.Is(chart.SetIndicator("NOPE", true), kline.ErrUnknownIndicator))
	// Output:
	// names: [MA MACD BOLL KDJ]
	// MA: true vol: false
	// BOLL pointer: true
	// BOLL: true true
	// unknown: true
}

// ExampleNormalizeClose indexes each close to its own base. Two symbols
// meet at 100 even when their raw prices differ.
func ExampleNormalizeClose() {
	a := []kline.Candle{{Close: 10}, {Close: 11}, {Close: 12}}
	b := []kline.Candle{{Close: 100}, {Close: 90}, {Close: 110}}
	na := kline.NormalizeClose(a, 0)
	nb := kline.NormalizeClose(b, 0)
	fmt.Printf("both start at %.0f\n", na[0])
	fmt.Printf("a: %.0f %.0f %.0f\n", na[0], na[1], na[2])
	fmt.Printf("b: %.0f %.0f %.0f\n", nb[0], nb[1], nb[2])
	fmt.Println("invalid base:", kline.NormalizeClose(a, -1) == nil)
	// Output:
	// both start at 100
	// a: 100 110 120
	// b: 100 90 110
	// invalid base: true
}

// ExampleIndexBase picks the first visible bar where every overlay series
// has a positive close.
func ExampleIndexBase() {
	a := []kline.Candle{{Close: 0}, {Close: 10}, {Close: 11}}
	b := []kline.Candle{{Close: 100}, {Close: 0}, {Close: 120}}
	fmt.Println("visible base:", kline.IndexBase(1, 2, a, b))
	fmt.Println("fallback base:", kline.IndexBase(1, 1, a, b))
	fmt.Println("empty:", kline.IndexBase(0, 0))
	// Output:
	// visible base: 2
	// fallback base: 2
	// empty: 0
}

// ExampleNormalizeToIndex scales OHLC by a base close and keeps time and
// volume untouched.
func ExampleNormalizeToIndex() {
	bars := []kline.Candle{{Open: 9, High: 11, Low: 8, Close: 10, Volume: 500}}
	scaled := kline.NormalizeToIndex(bars, 0)
	fmt.Printf("OHLC: %.0f %.0f %.0f %.0f\n", scaled[0].Open, scaled[0].High, scaled[0].Low, scaled[0].Close)
	fmt.Printf("volume: %.0f\n", scaled[0].Volume)
	// Output:
	// OHLC: 90 110 80 100
	// volume: 500
}

// ExampleStubLiveSource wires history and pushes without a network.
func ExampleStubLiveSource() {
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	start := time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)
	src := &kline.StubLiveSource{}
	if err := src.SetBars("A", hour, []kline.Candle{{Time: start, Close: 10}}); err != nil {
		panic(err)
	}
	var got []kline.Candle
	if err := src.SubscribeBar("A", hour, func(c kline.Candle) { got = append(got, c) }); err != nil {
		panic(err)
	}
	src.Push("A", hour, kline.Candle{Time: start.Add(time.Hour), Close: 11})
	fmt.Println("pushed:", len(got), got[0].Close)
	if err := src.UnsubscribeBar("A", hour); err != nil {
		panic(err)
	}
	src.Push("A", hour, kline.Candle{Time: start.Add(2 * time.Hour), Close: 12})
	fmt.Println("after unsubscribe:", len(got))
	// Output:
	// pushed: 1 11
	// after unsubscribe: 1
}

// ExampleSnapPrice shows weak and strong snapping to the same bar.
func ExampleSnapPrice() {
	bar := kline.Candle{Open: 100, High: 110, Low: 90, Close: 105}
	scale := chart.NewLinear(80, 120)
	const plotH = 400
	fmt.Printf("off: %.0f\n", kline.SnapPrice(kline.MagnetOff, bar, 104, plotH, scale))
	fmt.Printf("strong: %.0f\n", kline.SnapPrice(kline.MagnetStrong, bar, 104, plotH, scale))
	near := kline.LocalYToPrice(plotH, kline.PriceToLocalY(plotH, 105, scale)+2, scale)
	fmt.Printf("weak near: %.0f\n", kline.SnapPrice(kline.MagnetWeak, bar, near, plotH, scale))
	far := kline.LocalYToPrice(plotH, kline.PriceToLocalY(plotH, 105, scale)+40, scale)
	fmt.Printf("weak far unchanged: %v\n", kline.SnapPrice(kline.MagnetWeak, bar, far, plotH, scale) == far)
	// Output:
	// off: 104
	// strong: 105
	// weak near: 105
	// weak far unchanged: true
}

// ExampleStartReplay shows the host contract for blind playback: the
// chart starts empty, every revealed base bar calls OnBar, and a public
// append is refused until the replay is closed.
func ExampleStartReplay() {
	const symbol = "600519"
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	full, _, err := (&kline.SyntheticSource{Extra: -1}).GetBars(kline.BarRequest{
		Symbol: symbol, Period: hour, Kind: kline.KindInit, Limit: 20,
	})
	if err != nil {
		fmt.Println("load:", err)
		return
	}
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{symbol},
		Period:     hour,
		Indicators: []string{},
	})
	replay, err := kline.StartReplay(chart, map[string][]kline.Candle{symbol: full})
	if err != nil {
		fmt.Println("replay:", err)
		return
	}
	defer replay.Close()
	fmt.Println("after start:", chart.Bars())
	events := 0
	replay.OnBar(func(kline.ReplayBar) { events++ })
	if err := replay.SeekIndex(6); err != nil {
		fmt.Println("seek:", err)
		return
	}
	fmt.Println("after seek:", chart.Bars(), "events:", events)
	fmt.Println("append:", chart.AppendBar(symbol, full[10]) == kline.ErrReplayActive)
	replay.Close()
	fmt.Println("after close:", replay.Step(1))
	// Output:
	// after start: 0
	// after seek: 6 events: 6
	// append: true
	// after close: kline: replay is closed
}

// ExampleReplay_SeekTo shows a host seeking by time. It reveals every
// base bar whose open is at or before t.
func ExampleReplay_SeekTo() {
	const symbol = "600519"
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	full, _, err := (&kline.SyntheticSource{Extra: -1}).GetBars(kline.BarRequest{
		Symbol: symbol, Period: hour, Kind: kline.KindInit, Limit: 20,
	})
	if err != nil {
		fmt.Println("load:", err)
		return
	}
	chart := kline.NewChart(kline.ChartOptions{
		Symbols: []string{symbol}, Period: hour, Indicators: []string{},
	})
	replay, err := kline.StartReplay(chart, map[string][]kline.Candle{symbol: full})
	if err != nil {
		fmt.Println("replay:", err)
		return
	}
	if err := replay.SeekTo(full[7].Time); err != nil {
		fmt.Println("seek:", err)
		return
	}
	fmt.Println("cursor:", replay.Cursor())
	fmt.Println("bars:", chart.Bars())
	// Output:
	// cursor: 8
	// bars: 8
}

// ExampleReplay_Play drives frame-based playback with a fake clock. Play
// does not start a goroutine; the host calls Tick.
func ExampleReplay_Play() {
	bars := kline.Generate(5, 3)
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	replay, err := kline.StartReplay(chart, map[string][]kline.Candle{"600519": bars})
	if err != nil {
		panic(err)
	}
	defer replay.Close()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	replay.SetClock(func() time.Time { return now })
	replay.Play(2)
	now = now.Add(time.Second)
	replay.Tick()
	fmt.Println("after one second:", replay.Cursor())
	replay.Pause()
	now = now.Add(time.Hour)
	replay.Tick()
	fmt.Println("paused:", replay.Cursor(), replay.Playing())
	replay.Play(20)
	now = now.Add(time.Second)
	replay.Tick()
	fmt.Println("finished:", replay.Cursor(), replay.Playing())
	// Output:
	// after one second: 2
	// paused: 2 false
	// finished: 5 false
}

// ExampleReplay_StepBack walks backward without replaying OnBar, then
// reveals the next bar again.
func ExampleReplay_StepBack() {
	bars := kline.Generate(6, 4)
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	replay, err := kline.StartReplay(chart, map[string][]kline.Candle{"600519": bars})
	if err != nil {
		panic(err)
	}
	defer replay.Close()
	events := 0
	replay.OnBar(func(kline.ReplayBar) { events++ })
	fmt.Println("step:", replay.Step(4), replay.Cursor(), events)
	fmt.Println("back:", replay.StepBack(2), replay.Cursor(), events)
	fmt.Println("step again:", replay.Step(1), replay.Cursor(), events)
	fmt.Println("back past start:", replay.StepBack(99), replay.Cursor(), events)
	// Output:
	// step: <nil> 4 4
	// back: <nil> 2 4
	// step again: <nil> 3 5
	// back past start: <nil> 0 5
}

// ExampleChart_Crosshair shows the pointer read used by a host detail
// panel. Hovering the price axis does not report a bar.
func ExampleChart_Crosshair() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	tt := ui.NewTester(chart.View, 900, 500)
	tt.Frame()
	box, ok := tt.Find("600519 chart")
	if !ok {
		fmt.Println("chart missing")
		return
	}
	tt.Move(box.X+box.W*0.6, box.Y+box.H*0.2)
	tt.Frame()
	symbol, index, hit := chart.Crosshair()
	fmt.Println("hover:", symbol, index >= 0, hit)
	if bar, ok := chart.Bar(symbol, index); ok {
		fmt.Println("bar found:", !bar.Time.IsZero())
	}
	tt.Move(box.X+box.W-1, box.Y+box.H*0.2)
	tt.Frame()
	_, _, hit = chart.Crosshair()
	fmt.Println("over axis:", hit)
	// Output:
	// hover: 600519 true true
	// bar found: true
	// over axis: false
}

// ExampleChart_SetSource replaces a plain DataSource and reloads the
// current period. A nil source is refused before the chart changes.
func ExampleChart_SetSource() {
	chart := kline.NewChart(kline.ChartOptions{
		Source:     &kline.SyntheticSource{Extra: 40},
		Symbols:    []string{"600519"},
		Indicators: []string{},
	})
	fmt.Println("before:", chart.Bars(), chart.LoadError())
	fmt.Println("nil:", chart.SetSource(nil))
	if err := chart.SetSource(&kline.SyntheticSource{Extra: 80}); err != nil {
		fmt.Println("reload:", err)
		return
	}
	fmt.Println("after:", chart.Bars(), chart.LoadError())
	// Output:
	// before: 600 <nil>
	// nil: kline: nil data source
	// after: 600 <nil>
}

// ExampleChart_AppendBar feeds one new bar. With two symbols the bar is
// held until both have the same open time.
func ExampleChart_AppendBar() {
	symbols := []string{"600519", "000001"}
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    symbols,
		Indicators: []string{},
	})
	last, _ := chart.Bar(symbols[0], chart.Bars()-1)
	next := last
	next.Time = last.Time.Add(time.Hour)
	next.Close = last.Close + 1
	next.High = next.Close
	fmt.Println("first:", chart.AppendBar(symbols[0], next), chart.Bars())
	other, _ := chart.Bar(symbols[1], chart.Bars()-1)
	other.Time = next.Time
	fmt.Println("both:", chart.AppendBar(symbols[1], other), chart.Bars())
	fmt.Println("revision:", chart.UpdateLast(symbols[0], next) == nil)
	wrong := next
	wrong.Time = next.Time.Add(-time.Hour)
	fmt.Println("edge:", errors.Is(chart.AppendBar(symbols[0], wrong), kline.ErrBarTime))
	// Output:
	// first: <nil> 600
	// both: <nil> 601
	// revision: true
	// edge: true
}

// ExampleSplit shows the geometry a host needs when it owns paint. Paint
// itself takes these rects and never mutates the caller's viewport.
func ExampleSplit() {
	panes := kline.Split(ui.Rect{W: 800, H: 500})
	fmt.Println("price above volume:", panes.Price.Y+panes.Price.H < panes.Volume.Y)
	fmt.Println("axis to the right:", panes.PriceAxis.X >= panes.Price.X+panes.Price.W)
	fmt.Println("time at the bottom:", panes.Time.Y >= panes.Volume.Y)
	fmt.Println("sub-panes empty:", panes.MACD.H == 0 && panes.KDJ.H == 0)
	// Output:
	// price above volume: true
	// axis to the right: true
	// time at the bottom: true
	// sub-panes empty: true
}

// ExampleCompareScales shows the two scales an Overlay host needs:
// index for the plotted candles and raw for drawings in primary prices.
func ExampleCompareScales() {
	primary := []kline.Candle{
		{Close: 10, Open: 10, High: 11, Low: 9},
		{Close: 11, Open: 10, High: 12, Low: 9},
	}
	compare := []kline.Candle{{Close: 20}, {Close: 30}}
	index, raw, base := kline.CompareScales(primary, compare, nil,
		kline.Viewport{BarWidth: 8}, 160)
	fmt.Println("base:", base)
	fmt.Println("index has 100:", index.Lo <= 100 && index.Hi >= 100)
	fmt.Println("raw maps primary close:", raw.Lo <= 10 && raw.Hi >= 10)
	primary[0].Close = math.Inf(1)
	_, raw, base = kline.CompareScales(primary, compare, nil,
		kline.Viewport{BarWidth: 8}, 160)
	fmt.Println("invalid base:", base)
	fmt.Println("raw finite:", !math.IsInf(raw.Lo, 0) && !math.IsInf(raw.Hi, 0))
	// Output:
	// base: 0
	// index has 100: true
	// raw maps primary close: true
	// invalid base: 1
	// raw finite: true
}

// ExamplePaint draws a host-owned chart into a ui.Painter, checks that the
// candle color produced pixels, and leaves the caller's Viewport untouched.
func ExamplePaint() {
	bars := kline.Generate(30, 8)
	panes := kline.Split(ui.Rect{W: 640, H: 360})
	vp := kline.Viewport{}
	vp.FitRight(len(bars), float64(panes.Price.W))
	want := vp
	gold := ui.RGB(255, 214, 0)
	style := kline.DarkDefault()
	style.Up = gold
	style.Down = gold
	img := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			kline.Paint(p, kline.Split(r), bars[:10], nil, vp, kline.Pointer{}, -1, style)
		})
	}, 640, 360).Image()
	painted := false
	if img != nil {
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y && !painted; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if exampleNearColor(img.RGBAAt(x, y), gold) {
					painted = true
					break
				}
			}
		}
	}
	fmt.Println("candles painted:", painted)
	fmt.Println("viewport unchanged:", vp == want)
	// Output:
	// candles painted: true
	// viewport unchanged: true
}

func exampleNearColor(c color.RGBA, target ui.Color) bool {
	dr := int(c.R) - int(target.R)
	dg := int(c.G) - int(target.G)
	db := int(c.B) - int(target.B)
	return dr*dr+dg*dg+db*db <= 24*24
}

// ExamplePaintCompare draws an indexed pair, then falls back to the
// primary series when the compare base is unusable.
func ExamplePaintCompare() {
	primary := kline.Generate(12, 1)
	compare := kline.Generate(12, 2)
	panes := kline.Split(ui.Rect{W: 640, H: 360})
	vp := kline.Viewport{}
	vp.FitRight(len(primary), float64(panes.Price.W))
	draw := func(compare []kline.Candle) (indexed, painted bool) {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
				split := kline.Split(r)
				indexed = kline.PaintCompare(p, split, primary, compare, nil, vp, kline.Pointer{}, -1, kline.DarkDefault())
			})
		}, 640, 360)
		img := tt.Image()
		return indexed, img != nil
	}
	indexed, painted := draw(compare)
	fmt.Println("indexed:", indexed, painted)
	bad := append([]kline.Candle(nil), compare...)
	for i := range bad {
		bad[i].Close = math.NaN()
	}
	indexed, painted = draw(bad)
	fmt.Println("fallback:", indexed, painted)
	// Output:
	// indexed: true true
	// fallback: false true
}

// ExampleBarTail implements the optional incremental interface and checks
// append and revision against a full Calc.
func ExampleBarTail() {
	bars := kline.Generate(12, 9)
	tail := &exampleTail{}
	tail.Calc(bars[:10])
	tail.Append(bars[:11])
	appendOK := equalValues(tail.values, tailMean(bars[:11]))
	revised := append([]kline.Candle(nil), bars[:11]...)
	revised[10].Close += 2
	tail.Revise(revised, bars[10])
	reviseOK := equalValues(tail.values, tailMean(revised))
	fmt.Println("append equals Calc:", appendOK)
	fmt.Println("revise equals Calc:", reviseOK)
	fmt.Println("length:", len(tail.values))
	// Output:
	// append equals Calc: true
	// revise equals Calc: true
	// length: 11
}

// ExampleTimeAt maps a fractional bar index to the clock. It fills gaps by
// interpolating between neighboring bars and extrapolates at either end.
func ExampleTimeAt() {
	start := time.Date(2024, 1, 5, 9, 30, 0, 0, time.UTC)
	bars := []kline.Candle{
		{Time: start},
		{Time: start.AddDate(0, 0, 3)},
	}
	fmt.Println(kline.TimeAt(bars, 0).Format(time.RFC3339))
	fmt.Println(kline.TimeAt(bars, 0.5).Format(time.RFC3339))
	fmt.Println(kline.TimeAt(bars, 1).Format(time.RFC3339))
	// Output:
	// 2024-01-05T09:30:00Z
	// 2024-01-06T21:30:00Z
	// 2024-01-08T09:30:00Z
}

// ExampleLayer shows the host-side drawing store. Add returns the ID used
// for selection and deletion; Points are kept in bar time and price.
func ExampleLayer() {
	var layer kline.Layer
	line := layer.Add(kline.OverlayHLine, kline.Point{Price: 100})
	fmt.Println("kind:", line.Kind)
	fmt.Println("selected:", layer.Selected() != nil)
	fmt.Println("price updated:", layer.SetPrice(line.ID, 101))
	got := layer.ByID(line.ID)
	fmt.Println("price:", got != nil && got.Points[0].Price == 101)
	fmt.Println("deleted:", layer.Delete(line.ID))
	fmt.Println("remaining:", len(layer.Items()))
	// Output:
	// kind: hline
	// selected: true
	// price updated: true
	// price: true
	// deleted: true
	// remaining: 0
}

// ExampleChart_EditStyle starts from a theme and keeps the live hollow
// and limit switches while replacing one candle color.
func ExampleChart_EditStyle() {
	chart := kline.NewChart(kline.ChartOptions{Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"600519"}, Indicators: []string{}})
	chart.SetHollow(true)
	chart.SetLimitPct(0.10)
	chart.EditStyle(ui.LightTheme(), func(s *kline.Style) {
		s.Up = ui.RGB(220, 40, 40)
	})
	style := chart.Style(ui.LightTheme())
	fmt.Println("up customized:", style.Up == ui.RGB(220, 40, 40))
	fmt.Println("theme plot kept:", style.Plot == kline.LightDefault().Plot)
	fmt.Println("hollow kept:", style.HollowUp, chart.Hollow())
	fmt.Println("limit kept:", style.LimitPct, chart.LimitPct())
	// Output:
	// up customized: true
	// theme plot kept: true
	// hollow kept: true true
	// limit kept: 0.1 0.1
}

// ExampleChart_Close shows the hide/show lifecycle of a live chart. Close
// fences a callback already queued; a same-period SetPeriod reloads the
// current source and subscribes again.
func ExampleChart_Close() {
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	bars := []kline.Candle{
		{Time: time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC), Open: 9, High: 10, Low: 8, Close: 10},
		{Time: time.Date(2024, 1, 2, 10, 30, 0, 0, time.UTC), Open: 10, High: 11, Low: 9, Close: 10},
	}
	feed := &kline.StubLiveSource{}
	if err := feed.SetBars("A", hour, bars); err != nil {
		panic(err)
	}
	var queue []func()
	chart := kline.NewChart(kline.ChartOptions{
		Source: feed, Symbols: []string{"A"}, Period: hour, Indicators: []string{},
		Marshal: func(fn func()) { queue = append(queue, fn) },
	})
	drain := func() {
		for len(queue) > 0 {
			fn := queue[0]
			queue = queue[1:]
			fn()
		}
	}
	last := func() float64 {
		bar, _ := chart.Bar("A", chart.Bars()-1)
		return bar.Close
	}
	feed.Push("A", hour, kline.Candle{Time: bars[1].Time, Open: 10, High: 11, Low: 9, Close: 11})
	drain()
	fmt.Println("after dispatch:", last())
	feed.Push("A", hour, kline.Candle{Time: bars[1].Time, Open: 10, High: 12, Low: 9, Close: 12})
	if err := chart.Close(); err != nil {
		panic(err)
	}
	drain()
	fmt.Println("after close:", last())
	if err := chart.SetPeriod(hour); err != nil {
		panic(err)
	}
	fmt.Println("after reopen:", last())
	feed.Push("A", hour, kline.Candle{Time: bars[1].Time, Open: 10, High: 13, Low: 9, Close: 13})
	drain()
	fmt.Println("live again:", last())
	// Output:
	// after dispatch: 11
	// after close: 11
	// after reopen: 12
	// live again: 13
}

// ExampleSplitStack prepares the indicator set a host needs when it
// paints without Chart.View.
func ExampleSplitStack() {
	panes := kline.SplitStack(ui.Rect{W: 800, H: 600}, true, true, true)
	fmt.Println("MACD band:", panes.MACD.H > 0)
	fmt.Println("KDJ band:", panes.KDJ.H > 0)
	candles := kline.Generate(40, 7)
	macd := kline.NewMACD(12, 26, 9)
	kdj := kline.NewKDJ(9, 3, 3)
	macd.Calc(candles)
	kdj.Calc(candles)
	fmt.Println("MACD ready:", macd.DIF[len(macd.DIF)-1] != 0)
	fmt.Println("KDJ ready:", kdj.K[len(kdj.K)-1] != 0)
	// Output:
	// MACD band: true
	// KDJ band: true
	// MACD ready: true
	// KDJ ready: true
}

// ExampleChart_SetStyle shows that a zero color means "use the default",
// while a Clear bit makes that field explicitly transparent.
func ExampleChart_SetStyle() {
	chart := kline.NewChart(kline.ChartOptions{Indicators: []string{}})
	red := ui.RGB(220, 40, 40)
	chart.SetStyle(kline.Style{
		Up: red, Clear: kline.ClearGrid | kline.ClearBuy,
	})
	style := chart.Style(nil)
	fmt.Println("grid transparent:", style.Grid == (ui.Color{}))
	fmt.Println("buy transparent:", style.Buy == (ui.Color{}))
	fmt.Println("default text retained:", style.Text != (ui.Color{}))
	fmt.Println("sell follows down:", style.Sell == style.Down)
	// Output:
	// grid transparent: true
	// buy transparent: true
	// default text retained: true
	// sell follows down: true
}

// ExampleOverlaySpec_Rubber shows the callback contract for a two-anchor
// preview and endpoint hit test. Rubber coordinates are local to the price
// plot, and DrawOverlays clips them. This example calls the callbacks
// directly; it does not commit a pointer gesture through Chart.View.
func ExampleOverlaySpec_Rubber() {
	spec := kline.OverlaySpec{
		Kind: "ruler-example", Anchors: 2,
		Draw: func(*ui.Painter, ui.Rect, kline.Overlay, kline.Viewport, chart.Linear, []kline.Candle, kline.Style) {},
		Hit: func(o kline.Overlay, x, y, _, plotH float64, vp kline.Viewport, scale chart.Linear, cs []kline.Candle) (int, bool) {
			for i, point := range o.Points {
				if math.Hypot(x-kline.AnchorX(point, vp, cs), y-kline.PriceToLocalY(plotH, point.Price, scale)) <= kline.HitRadius {
					return i, true
				}
			}
			return 0, false
		},
		Rubber: func(_ *ui.Painter, plot ui.Rect, rubber kline.Rubber, _ kline.Viewport, scale chart.Linear, _ []kline.Candle, _ kline.Style) {
			y := kline.LocalYToPrice(float64(plot.H), rubber.Y, scale)
			fmt.Printf("preview delta: %.2f\n", y-rubber.From.Price)
		},
	}
	if err := kline.RegisterOverlay(spec); err != nil {
		fmt.Println("register:", err)
		return
	}
	defer kline.UnregisterOverlay(spec.Kind)
	candles := kline.Generate(10, 1)
	vp := kline.Viewport{BarWidth: 40}
	scale := chart.NewLinear(80, 120)
	plot := ui.Rect{W: 400, H: 200}
	rubber := kline.Rubber{From: kline.Point{Price: 100}, X: 200,
		Y: kline.PriceToLocalY(float64(plot.H), 110, scale)}
	spec.Rubber(nil, plot, rubber, vp, scale, candles, kline.DarkDefault())
	var layer kline.Layer
	line := layer.Add(spec.Kind, rubber.From, kline.Point{Price: 110})
	fmt.Println("committed anchors:", len(line.Points))
	hit, ok := layer.Hit(kline.AnchorX(line.Points[1], vp, candles),
		kline.PriceToLocalY(float64(plot.H), line.Points[1].Price, scale), 400, 200, vp, scale, candles)
	ok = ok && hit.Part == 1
	fmt.Println("endpoint hit:", ok)
	// Output:
	// preview delta: 10.00
	// committed anchors: 2
	// endpoint hit: true
}

type subscriptionExampleSource struct {
	bars   map[string][]kline.Candle
	subs   map[string]kline.BarHandler
	fail   map[string]bool
	active int
}

func (s *subscriptionExampleSource) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	return append([]kline.Candle(nil), s.bars[req.Symbol]...), false, nil
}

func (s *subscriptionExampleSource) SubscribeBar(symbol string, _ kline.Period, handler kline.BarHandler) error {
	if s.fail[symbol] {
		return errors.New("subscription unavailable")
	}
	if s.subs == nil {
		s.subs = map[string]kline.BarHandler{}
	}
	s.subs[symbol] = handler
	s.active = len(s.subs)
	return nil
}

func (s *subscriptionExampleSource) UnsubscribeBar(symbol string, _ kline.Period) error {
	delete(s.subs, symbol)
	s.active = len(s.subs)
	return nil
}

// ExampleLiveSource shows the replacement-source contract: history is
// installed, subscriptions are rolled back, and a retry subscribes the
// current symbols again.
func ExampleLiveSource() {
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	start := time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)
	bars := func(close float64) []kline.Candle {
		out := make([]kline.Candle, 3)
		for i := range out {
			out[i] = kline.Candle{Time: start.Add(time.Duration(i) * time.Hour),
				Open: close, High: close + 1, Low: close - 1, Close: close}
		}
		return out
	}
	src := &subscriptionExampleSource{
		fail: map[string]bool{"B": true},
		bars: map[string][]kline.Candle{"A": bars(10), "B": bars(20)},
	}
	chart := kline.NewChart(kline.ChartOptions{
		Source: &kline.SyntheticSource{Extra: -1}, Symbols: []string{"A", "B"},
		Period: hour, Indicators: []string{},
	})
	err := chart.SetSource(src)
	fmt.Println("subscribe failed:", err != nil)
	fmt.Println("replacement bars:", chart.Bars())
	fmt.Println("active after failure:", src.active)
	src.fail["B"] = false
	fmt.Println("retry:", chart.SetPeriod(chart.Period()))
	fmt.Println("active after retry:", src.active)
	// Output:
	// subscribe failed: true
	// replacement bars: 3
	// active after failure: 0
	// retry: <nil>
	// active after retry: 2
}

// ExampleLayer_SetPoint keeps a segment's anchor tied to bar time and
// shows that Items returns a snapshot of each point slice.
func ExampleLayer_SetPoint() {
	start := time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)
	candles := []kline.Candle{
		{Time: start}, {Time: start.Add(time.Hour)}, {Time: start.Add(2 * time.Hour)},
	}
	var layer kline.Layer
	line := layer.Add(kline.OverlaySegment,
		kline.Point{BarIndex: 0, Time: candles[0].Time, Price: 100},
		kline.Point{BarIndex: 1, Time: candles[1].Time, Price: 110})
	fmt.Println("anchor index:", func() int { i, _ := line.Points[0].Index(candles); return i }())
	candles = append([]kline.Candle{{Time: start.Add(-time.Hour)}}, candles...)
	fmt.Println("after prepend:", func() int { i, _ := layer.ByID(line.ID).Points[0].Index(candles); return i }())
	fmt.Println("point updated:", layer.SetPoint(line.ID, 1, kline.Point{
		BarIndex: 1, Time: candles[2].Time, Price: 112,
	}))
	snapshot := layer.Items()
	snapshot[0].Points[1].Price = 999
	fmt.Println("endpoint price:", layer.ByID(line.ID).Points[1].Price)
	// Output:
	// anchor index: 0
	// after prepend: 1
	// point updated: true
	// endpoint price: 112
}

// memorySource is the shortest useful DataSource: one sorted series,
// latest-window init, strictly-earlier forward paging, and explicit
// rejection of unsupported request kinds.
type memorySource struct {
	all []kline.Candle
}

func (s *memorySource) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	if req.Kind != kline.KindInit && req.Kind != kline.KindForward {
		return nil, false, kline.ErrUnsupportedKind
	}
	limit := req.Limit
	if limit <= 0 {
		limit = kline.DefaultLimit
	}
	end := len(s.all)
	if req.Kind == kline.KindForward {
		if req.Anchor.IsZero() {
			return nil, false, fmt.Errorf("forward requires anchor")
		}
		end = 0
		for end < len(s.all) && s.all[end].Time.Before(req.Anchor) {
			end++
		}
	}
	start := max(end-limit, 0)
	return append([]kline.Candle(nil), s.all[start:end]...), start > 0, nil
}

type exampleMean struct {
	values []float64
	line   [][]float64
}

func (m *exampleMean) Name() string     { return "EXAMPLE_MEAN" }
func (m *exampleMean) PaneKind() string { return kline.PaneMain }
func (m *exampleMean) Calc(cs []kline.Candle) {
	m.values = make([]float64, len(cs))
	for i, c := range cs {
		m.values[i] = (c.Open + c.Close) / 2
	}
	m.line = [][]float64{m.values}
}
func (m *exampleMean) DrawMain(*ui.Painter, ui.Rect, kline.Viewport, chart.Linear) {}
func (m *exampleMean) DrawSub(*ui.Painter, ui.Rect, kline.Viewport)                {}
func (m *exampleMean) ValueAt(i int) string {
	if i < 0 || i >= len(m.values) {
		return ""
	}
	return fmt.Sprintf("MEAN %.2f", m.values[i])
}
func (m *exampleMean) Series() [][]float64    { return m.line }
func (m *exampleMean) LineColors() []ui.Color { return []ui.Color{ui.RGB(230, 120, 20)} }

type exampleTail struct {
	values []float64
	line   [][]float64
}

func (t *exampleTail) Name() string     { return "EXAMPLE_TAIL" }
func (t *exampleTail) PaneKind() string { return kline.PaneMain }
func (t *exampleTail) Calc(cs []kline.Candle) {
	t.values = tailMean(cs)
	t.line = [][]float64{t.values}
}
func (t *exampleTail) Append(cs []kline.Candle) {
	if len(t.values) != len(cs)-1 {
		t.Calc(cs)
		return
	}
	t.values = append(t.values, cs[len(cs)-1].Close)
	t.line = [][]float64{t.values}
}
func (t *exampleTail) Revise(cs []kline.Candle, _ kline.Candle) {
	if len(t.values) != len(cs) {
		t.Calc(cs)
		return
	}
	t.values[len(t.values)-1] = cs[len(cs)-1].Close
	t.line = [][]float64{t.values}
}
func (t *exampleTail) DrawMain(*ui.Painter, ui.Rect, kline.Viewport, chart.Linear) {}
func (t *exampleTail) DrawSub(*ui.Painter, ui.Rect, kline.Viewport)                {}
func (t *exampleTail) ValueAt(i int) string {
	if i < 0 || i >= len(t.values) {
		return ""
	}
	return fmt.Sprintf("TAIL %.2f", t.values[i])
}
func (t *exampleTail) Series() [][]float64    { return t.line }
func (t *exampleTail) LineColors() []ui.Color { return []ui.Color{ui.RGB(40, 160, 220)} }

func tailMean(cs []kline.Candle) []float64 {
	out := make([]float64, len(cs))
	for i, c := range cs {
		out[i] = c.Close
	}
	return out
}

func equalValues(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
