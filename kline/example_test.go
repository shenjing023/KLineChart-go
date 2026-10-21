// Package kline_test holds runnable examples for the chart library. Each
// Example is compiled and run by `go test`, so an example that drifts
// from the exported API fails the build instead of rotting in a doc.
package kline_test

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
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

// Example_periods renders and parses a bar size.
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
	// Zero digits keeps the whole part: the value is formatted, not
	// rounded.
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
