// K-line is a native candlestick demo. The window is ui.View, drawn by
// MyGo itself: no HTML and no WebView. This file is the window chrome
// and the toolbar. The plots, pan, zoom, and crosshair live in kline.Chart.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func main() {
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     kline.Period{Span: 1, Unit: kline.UnitHour},
		Layout:     kline.LayoutStack,
		Indicators: []string{"MA", "MACD"},
	})
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "K-line",
			Width:     1100,
			Height:    1000,
			MinWidth:  640,
			MinHeight: 720,
			StateKey:  "kline",
			Content:   ui.View(func(c *ui.Context) { demoView(c, chart) }),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func demoView(c *ui.Context, chart *kline.Chart) {
	t := c.Theme()
	style := chart.Style(t)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(8, 14, 0, 14).Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, "K-line").Bold().FontSize(16)
			if chart.Layout() == kline.LayoutOverlay {
				ui.Textf(c, "%d bars · overlay", chart.Bars()).FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Textf(c, "%d bars · %d panes", chart.Bars(), len(chart.Symbols())).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Textf(c, "Period %s", chart.Period()).FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, "MA5").FontSize(12).TextColor(style.MA5)
			ui.Text(c, "MA20").FontSize(12).TextColor(style.MA20)
			ui.Text(c, "DIF").FontSize(12).TextColor(style.MA5)
			ui.Text(c, "DEA").FontSize(12).TextColor(style.MA20)
			for _, name := range chart.IndicatorNames() {
				if on := chart.IndicatorEnabled(name); on != nil {
					ui.Toggle(c, on, name)
				}
			}
			ui.Toggle(c, chart.HollowEnabled(), "Hollow")
			for _, preset := range colorPresets {
				st := chart.Style(t)
				var b ui.Element
				if st.Up == preset.up && st.Down == preset.down && st.LimitUp == preset.limitUp && st.LimitDown == preset.limitDown {
					b = ui.PrimaryButton(c, preset.label)
				} else {
					b = ui.Button(c, preset.label)
				}
				if b.Clicked() {
					st.Up = preset.up
					st.Down = preset.down
					st.LimitUp = preset.limitUp
					st.LimitDown = preset.limitDown
					chart.SetStyle(st)
				}
			}
			limitLabel := "Limit 10%"
			var limitBtn ui.Element
			if chart.LimitPct() > 0 {
				limitBtn = ui.PrimaryButton(c, limitLabel)
			} else {
				limitBtn = ui.Button(c, limitLabel)
			}
			if limitBtn.Clicked() {
				if chart.LimitPct() > 0 {
					chart.SetLimitPct(0)
				} else {
					chart.SetLimitPct(0.10)
				}
			}
			for _, choice := range periodChoices {
				var b ui.Element
				if chart.Period() == choice.period {
					b = ui.PrimaryButton(c, choice.label)
				} else {
					b = ui.Button(c, choice.label)
				}
				if b.Clicked() {
					_ = chart.SetPeriod(choice.period)
				}
			}
			for _, choice := range layoutChoices {
				var b ui.Element
				if chart.Layout() == choice.layout {
					b = ui.PrimaryButton(c, choice.label)
				} else {
					b = ui.Button(c, choice.label)
				}
				if b.Clicked() {
					chart.SetLayout(choice.layout)
				}
			}
			if chart.Layout() == kline.LayoutOverlay {
				ui.Text(c, "idx 100").FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, "BETA line").FontSize(12).TextColor(style.Compare)
			}
			for _, choice := range toolChoices {
				var b ui.Element
				if chart.Tool() == choice.tool {
					b = ui.PrimaryButton(c, choice.label)
				} else {
					b = ui.Button(c, choice.label)
				}
				if b.Clicked() {
					_ = chart.SetTool(choice.tool)
				}
			}
			for _, choice := range magnetChoices {
				var b ui.Element
				if chart.Magnet() == choice.mode {
					b = ui.PrimaryButton(c, choice.label)
				} else {
					b = ui.Button(c, choice.label)
				}
				if b.Clicked() {
					chart.SetMagnet(choice.mode)
				}
			}
			ui.Box(c).Grow(1)
			if ui.Button(c, "Reset view").Clicked() {
				chart.ResetView()
			}
		})
		hint := "Pan drags both panes. H-Line, Segment, and Fib draw on the pane you click, and pan is off while a draw tool is selected. Escape cancels a line that is still waiting for its second point. Delete removes the selected line. Right-click removes the line under the pointer, or cancels that unfinished line."
		if chart.Layout() == kline.LayoutOverlay {
			hint = "Overlay indexes both symbols to 100 at the first visible bar where both closes are positive. ALPHA stays candles. BETA is the close line. Volume and MACD are ALPHA only, and drawings stay on ALPHA. Pan and the wheel still move the shared viewport."
		}
		ui.Text(c, hint).Padding(4, 14, 6, 14).FontSize(12).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).WidthPercent(100).Children(func() {
			chart.View(c)
		})
	})
}

var periodChoices = []struct {
	label  string
	period kline.Period
}{
	{"1h", kline.Period{Span: 1, Unit: kline.UnitHour}},
	{"4h", kline.Period{Span: 4, Unit: kline.UnitHour}},
	{"1d", kline.Period{Span: 1, Unit: kline.UnitDay}},
}

var toolChoices = []struct {
	label string
	tool  string
}{
	{"Pan", kline.ToolPan},
	{"H-Line", kline.ToolHLine},
	{"Segment", kline.ToolSegment},
	{"Fib", kline.ToolFib},
}

var magnetChoices = []struct {
	label string
	mode  kline.MagnetMode
}{
	{"Magnet Off", kline.MagnetOff},
	{"Magnet Weak", kline.MagnetWeak},
	{"Magnet Strong", kline.MagnetStrong},
}

var layoutChoices = []struct {
	label  string
	layout string
}{
	{"Stack", kline.LayoutStack},
	{"Overlay", kline.LayoutOverlay},
}

// colorPresets recolor candle bodies. Classic is the default green-up
// red-down pair. US swaps them. Contrast is a high-contrast pair that
// stays off the plot background. Limit colors are the brighter step of
// the same pair. Hollow and the limit percent are left as they are.
var colorPresets = []struct {
	label                        string
	up, down, limitUp, limitDown ui.Color
}{
	{"Classic", ui.RGB(38, 166, 154), ui.RGB(239, 83, 80), ui.RGB(0, 230, 118), ui.RGB(255, 59, 48)},
	{"US", ui.RGB(239, 83, 80), ui.RGB(38, 166, 154), ui.RGB(255, 59, 48), ui.RGB(0, 230, 118)},
	{"Contrast", ui.RGB(255, 214, 0), ui.RGB(56, 189, 248), ui.RGB(255, 255, 255), ui.RGB(168, 85, 247)},
}
