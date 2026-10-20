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
	if dark.Up != ui.RGB(38, 166, 154) || dark.Down != ui.RGB(239, 83, 80) {
		t.Fatalf("candle colors %+v %+v", dark.Up, dark.Down)
	}
	if dark.LimitUp == (ui.Color{}) || dark.LimitDown == (ui.Color{}) || dark.LimitUp == dark.Up || dark.LimitDown == dark.Down {
		t.Fatalf("limit colors should be set and brighter than the body colors: %+v %+v", dark.LimitUp, dark.LimitDown)
	}
	if dark.LimitPct != 0 || light.LimitPct != 0 {
		t.Fatal("limit paint starts off")
	}
	if dark.Up != light.Up || dark.Down != light.Down {
		t.Fatal("rising stays green and falling stays red on both themes")
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
