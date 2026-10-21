package kline

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDarkAndLightDefaultsFollowTheme(t *testing.T) {
	dark := DarkDefault()
	light := LightDefault()
	if dark != StyleFrom(ui.DarkTheme()) {
		t.Fatalf("DarkDefault = %+v, StyleFrom(dark) = %+v", dark, StyleFrom(ui.DarkTheme()))
	}
	if light != StyleFrom(ui.LightTheme()) {
		t.Fatalf("LightDefault = %+v, StyleFrom(light) = %+v", light, StyleFrom(ui.LightTheme()))
	}
	if StyleFrom(nil) != dark {
		t.Fatal("a nil theme uses the dark default")
	}
	if dark.Up != ui.RGB(239, 83, 80) || dark.Down != ui.RGB(38, 166, 154) {
		t.Fatalf("candle colors %+v %+v", dark.Up, dark.Down)
	}
	if dark.LimitUp == (ui.Color{}) || dark.LimitDown == (ui.Color{}) || dark.LimitUp == dark.Up || dark.LimitDown == dark.Down {
		t.Fatalf("limit colors should be set and brighter than the body colors: %+v %+v", dark.LimitUp, dark.LimitDown)
	}
	if dark.LimitPct != 0 || light.LimitPct != 0 {
		t.Fatal("limit paint starts off")
	}
	if dark.Up != light.Up || dark.Down != light.Down {
		t.Fatal("rising stays red and falling stays green on both themes")
	}
	if dark.Plot == light.Plot || dark.Grid == light.Grid || dark.Cross == light.Cross {
		t.Fatalf("plot, grid, and crosshair should follow the theme: dark %+v light %+v", dark, light)
	}
	if dark.MA5 == (ui.Color{}) || dark.MA20 == (ui.Color{}) || light.MA5 == (ui.Color{}) || light.MA20 == (ui.Color{}) {
		t.Fatal("MA colors are empty")
	}
	if dark.MACDDIF == (ui.Color{}) || dark.MACDDEA == (ui.Color{}) || dark.MACDZero == (ui.Color{}) {
		t.Fatalf("MACD colors %+v %+v %+v", dark.MACDDIF, dark.MACDDEA, dark.MACDZero)
	}
	if dark.BOLLMid == (ui.Color{}) || dark.BOLLUp == (ui.Color{}) || dark.BOLLLow == (ui.Color{}) {
		t.Fatalf("BOLL colors %+v %+v %+v", dark.BOLLMid, dark.BOLLUp, dark.BOLLLow)
	}
	if dark.KDJK == (ui.Color{}) || dark.KDJD == (ui.Color{}) || dark.KDJJ == (ui.Color{}) {
		t.Fatalf("KDJ colors %+v %+v %+v", dark.KDJK, dark.KDJD, dark.KDJJ)
	}
	if dark.MA5 == light.MA5 && dark.MACDDIF == light.MACDDIF {
		t.Fatal("MA and MACD colors should follow each theme's accent")
	}
	if dark.HollowUp || light.HollowUp {
		t.Fatal("hollow rising candles start off")
	}
}

func TestPartialStyleKeepsExplicitTransparent(t *testing.T) {
	s := completeStyle(Style{Up: ui.RGB(9, 9, 9), Clear: ClearGrid | ClearOverlay})
	if s.Grid != (ui.Color{}) || s.Overlay != (ui.Color{}) {
		t.Fatalf("transparent fields were filled: grid %+v overlay %+v", s.Grid, s.Overlay)
	}
	if s.Plot.A == 0 || s.Text.A == 0 || s.Up != ui.RGB(9, 9, 9) {
		t.Fatalf("partial merge lost a default or the override: %+v", s)
	}
	full := DarkDefault()
	full.Grid = ui.Color{}
	got := completeStyle(full)
	if got.Grid.A != 0 {
		t.Fatal("a fully specified transparent grid was replaced")
	}
}

// clearCase names one Clear bit and the field it empties.
type clearCase struct {
	name string
	bit  ColorClear
	get  func(Style) ui.Color
}

func clearCases() []clearCase {
	return []clearCase{
		{"Up", ClearUp, func(s Style) ui.Color { return s.Up }},
		{"Down", ClearDown, func(s Style) ui.Color { return s.Down }},
		{"LimitUp", ClearLimitUp, func(s Style) ui.Color { return s.LimitUp }},
		{"LimitDown", ClearLimitDown, func(s Style) ui.Color { return s.LimitDown }},
		{"MA5", ClearMA5, func(s Style) ui.Color { return s.MA5 }},
		{"MA20", ClearMA20, func(s Style) ui.Color { return s.MA20 }},
		{"MACDDIF", ClearMACDDIF, func(s Style) ui.Color { return s.MACDDIF }},
		{"MACDDEA", ClearMACDDEA, func(s Style) ui.Color { return s.MACDDEA }},
		{"MACDZero", ClearMACDZero, func(s Style) ui.Color { return s.MACDZero }},
		{"BOLLMid", ClearBOLLMid, func(s Style) ui.Color { return s.BOLLMid }},
		{"BOLLUp", ClearBOLLUp, func(s Style) ui.Color { return s.BOLLUp }},
		{"BOLLLow", ClearBOLLLow, func(s Style) ui.Color { return s.BOLLLow }},
		{"KDJK", ClearKDJK, func(s Style) ui.Color { return s.KDJK }},
		{"KDJD", ClearKDJD, func(s Style) ui.Color { return s.KDJD }},
		{"KDJJ", ClearKDJJ, func(s Style) ui.Color { return s.KDJJ }},
		{"Grid", ClearGrid, func(s Style) ui.Color { return s.Grid }},
		{"Cross", ClearCross, func(s Style) ui.Color { return s.Cross }},
		{"Text", ClearText, func(s Style) ui.Color { return s.Text }},
		{"Muted", ClearMuted, func(s Style) ui.Color { return s.Muted }},
		{"Plot", ClearPlot, func(s Style) ui.Color { return s.Plot }},
		{"VolumeBg", ClearVolumeBg, func(s Style) ui.Color { return s.VolumeBg }},
		{"LabelFill", ClearLabelFill, func(s Style) ui.Color { return s.LabelFill }},
		{"LabelText", ClearLabelText, func(s Style) ui.Color { return s.LabelText }},
		{"Overlay", ClearOverlay, func(s Style) ui.Color { return s.Overlay }},
		{"OverlayHot", ClearOverlayHot, func(s Style) ui.Color { return s.OverlayHot }},
		{"Compare", ClearCompare, func(s Style) ui.Color { return s.Compare }},
		{"Buy", ClearBuy, func(s Style) ui.Color { return s.Buy }},
		{"Sell", ClearSell, func(s Style) ui.Color { return s.Sell }},
	}
}

// Every Clear bit empties its field, and no bit empties a neighbour.
func TestClearBitsClearTheirFields(t *testing.T) {
	cases := clearCases()
	seen := make(map[ColorClear]string, len(cases))
	for _, tc := range cases {
		if prev, dup := seen[tc.bit]; dup {
			t.Fatalf("%s and %s share a bit", prev, tc.name)
		}
		seen[tc.bit] = tc.name
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A fully specified palette keeps its fields as given, so
			// only the Clear bit can empty one.
			s := DarkDefault()
			s.Clear = tc.bit
			got := completeStyle(s)
			if c := tc.get(got); c != (ui.Color{}) {
				t.Fatalf("%s stayed %+v", tc.name, c)
			}
			for _, other := range cases {
				if other.name == tc.name {
					continue
				}
				if other.get(got) == (ui.Color{}) {
					t.Fatalf("clearing %s also emptied %s", tc.name, other.name)
				}
			}
		})
	}

	// A partial palette takes the same bits after the merge.
	partial := completeStyle(Style{Up: ui.RGB(9, 9, 9), Clear: ClearGrid | ClearOverlay | ClearBuy})
	if partial.Grid != (ui.Color{}) || partial.Overlay != (ui.Color{}) || partial.Buy != (ui.Color{}) {
		t.Fatalf("partial clear: grid %+v overlay %+v buy %+v", partial.Grid, partial.Overlay, partial.Buy)
	}
	if partial.Plot.A == 0 || partial.Sell == (ui.Color{}) {
		t.Fatalf("partial clear emptied an unset neighbour: %+v", partial)
	}

	// Every bit value is covered by the table.
	var all ColorClear
	for _, tc := range cases {
		all |= tc.bit
	}
	if all != ColorClear(1<<len(cases))-1 {
		t.Fatalf("bits cover %b, want %b", all, ColorClear(1<<len(cases))-1)
	}
}

func TestSetHollowMatchesStoredStyle(t *testing.T) {
	c := NewChart(ChartOptions{})
	c.SetStyle(Style{Up: ui.RGB(1, 2, 3)})
	c.SetHollow(true)
	if !c.style.HollowUp || !c.Hollow() || !c.Style(nil).HollowUp {
		t.Fatalf("stored %v live %v style %v", c.style.HollowUp, c.Hollow(), c.Style(nil).HollowUp)
	}
	c.SetHollow(false)
	if c.style.HollowUp || c.Hollow() || c.Style(nil).HollowUp {
		t.Fatal("hollow stayed on")
	}
	d := NewChart(ChartOptions{})
	d.SetHollow(true)
	if d.styleSet || d.style.HollowUp {
		t.Fatal("hollow on the theme path stored a style")
	}
	if !d.Style(nil).HollowUp {
		t.Fatal("theme path dropped the live hollow switch")
	}
	only := NewChart(ChartOptions{Hollow: true, Style: Style{Up: ui.RGB(4, 5, 6)}})
	if !only.style.HollowUp || !only.Style(nil).HollowUp {
		t.Fatal("Hollow option did not land on the stored style")
	}
}

func TestCandleBodyColorUsesLimitOnlyWhenPctIsSet(t *testing.T) {
	s := DarkDefault()
	up := Candle{Open: 10, Close: 11}
	down := Candle{Open: 11, Close: 10}
	flat := Candle{Open: 10, Close: 10}
	if got := s.CandleBodyColor(up, 10, 0); got != s.Up {
		t.Fatalf("pct 0 rising %v", got)
	}
	if got := s.CandleBodyColor(down, 12, -1); got != s.Down {
		t.Fatalf("negative pct falling %v", got)
	}
	if got := s.CandleBodyColor(flat, 10, 0); got != s.Up {
		t.Fatalf("flat close is rising by the >= rule: %v", got)
	}
	// +10% exactly is a limit-up. 9% is not. close == high is not required.
	limitUp := Candle{Open: 100, High: 110, Low: 100, Close: 110}
	if got := s.CandleBodyColor(limitUp, 100, 0.10); got != s.LimitUp {
		t.Fatalf("exact +10%% %v", got)
	}
	inside := Candle{Open: 100, Close: 109}
	if got := s.CandleBodyColor(inside, 100, 0.10); got != s.Up {
		t.Fatalf("+9%% should stay Up: %v", got)
	}
	limitDown := Candle{Open: 100, Close: 90}
	if got := s.CandleBodyColor(limitDown, 100, 0.10); got != s.LimitDown {
		t.Fatalf("exact -10%% %v", got)
	}
	mildDown := Candle{Open: 100, Close: 91}
	if got := s.CandleBodyColor(mildDown, 100, 0.10); got != s.Down {
		t.Fatalf("-9%% should stay Down: %v", got)
	}
	// No previous close, or a non-positive one, cannot be a limit move.
	if got := s.CandleBodyColor(limitUp, 0, 0.10); got != s.Up {
		t.Fatalf("prev 0 %v", got)
	}
	if got := s.CandleBodyColor(limitUp, 0, 0.10); got != s.Up {
		t.Fatal(got)
	}
	if got := s.CandleBodyColor(Candle{Open: 1, Close: 2}, -5, 0.10); got != s.Up {
		t.Fatalf("negative prev %v", got)
	}
}
