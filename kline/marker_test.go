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
	if err := a.SetMarkers("ALPHA", []Marker{
		{Time: bar.Time, Side: SideBuy, Qty: 2},
		{Time: inside, Side: SideBuy, Label: "add"},
		{Time: bar.Time, Side: SideSell, Price: bar.High},
	}); err != nil {
		t.Fatal(err)
	}
	if err := a.AppendMarker("ALPHA", Marker{Time: bar.Time, Side: SideSell, Label: "S"}); err != nil {
		t.Fatal(err)
	}
	got, err := a.Markers("ALPHA")
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
	beta, err := a.Markers("BETA")
	if err != nil || len(beta) != 0 {
		t.Fatalf("beta markers leaked: %d %v", len(beta), err)
	}
	if err := a.ClearMarkers("ALPHA"); err != nil {
		t.Fatal(err)
	}
	if n, _ := a.Markers("ALPHA"); len(n) != 0 {
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
	if err := a.SetMarkers("ALPHA", keep); err != nil {
		t.Fatal(err)
	}
	err := a.SetMarkers("NOPE", []Marker{{Time: bar, Side: SideBuy}})
	if !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("symbol err %v", err)
	}
	err = a.SetMarkers("ALPHA", []Marker{{Time: bar, Side: SideBuy}, {Time: bar, Side: 0}})
	if !errors.Is(err, ErrUnknownMarker) {
		t.Fatalf("side err %v", err)
	}
	got, _ := a.Markers("ALPHA")
	if len(got) != 1 || got[0].Side != SideBuy {
		t.Fatalf("rejected set changed markers: %+v", got)
	}
	host := []Marker{{Time: bar, Side: SideSell}}
	if err := a.SetMarkers("ALPHA", host); err != nil {
		t.Fatal(err)
	}
	host[0].Side = SideBuy
	got, _ = a.Markers("ALPHA")
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
		Symbols:    []string{"ALPHA"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Indicators: []string{},
	})
	// 01:30 sits in the 01:00 hour bar and in the 00:00 four-hour bar.
	at := start.Add(90 * time.Minute)
	outside := start.Add(-time.Hour)
	past := start.Add(8 * time.Hour)
	if err := a.SetMarkers("ALPHA", []Marker{
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
	if !strings.Contains(a.Readout("ALPHA"), "B @") {
		t.Fatalf("readout %q", a.Readout("ALPHA"))
	}
	a.crossIndex = -1
	if strings.Contains(a.Readout("ALPHA"), "B @") {
		t.Fatalf("last bar should not show the 00:00 marker: %q", a.Readout("ALPHA"))
	}
}

func TestMarkerPrependShiftsIndex(t *testing.T) {
	a := newApp()
	a.fitted = true
	a.plotW = 800
	const watch = 20
	when := a.panes[0].candles[watch].Time
	if err := a.SetMarkers("ALPHA", []Marker{{Time: when, Side: SideBuy, Label: "B"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetMarkers("BETA", []Marker{{Time: a.panes[1].candles[watch].Time, Side: SideSell}}); err != nil {
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
	if err := a.SetMarkers("ALPHA", []Marker{{Time: next.Time, Side: SideBuy, Label: "open"}}); err != nil {
		t.Fatal(err)
	}
	if a.panes[0].marks[0].bar != -1 {
		t.Fatalf("future marker placed at %d", a.panes[0].marks[0].bar)
	}
	stub.Push("ALPHA", hour, next)
	beta := a.panes[1].candles[len(a.panes[1].candles)-1]
	beta.Time = next.Time
	stub.Push("BETA", hour, beta)
	want := len(a.panes[0].candles) - 1
	if a.panes[0].marks[0].bar != want {
		t.Fatalf("after append bar %d want %d", a.panes[0].marks[0].bar, want)
	}
	a.crossIndex = want
	if !strings.Contains(a.Readout("ALPHA"), "open") {
		t.Fatalf("readout %q", a.Readout("ALPHA"))
	}
	if strings.Contains(a.Readout("BETA"), "open") {
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
	us.Up = ui.RGB(239, 83, 80)
	us.Down = ui.RGB(38, 166, 154)
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
	if err := a.SetMarkers("BETA", []Marker{{Time: when, Side: SideSell, Price: 50, Label: "be"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetMarkers("ALPHA", []Marker{{Time: a.panes[0].candles[len(a.panes[0].candles)-5].Time, Side: SideBuy}}); err != nil {
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
