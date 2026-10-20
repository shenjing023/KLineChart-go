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
