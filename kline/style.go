package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// Style is the chart's colors and the candle body variant.
// Up is a rising close (close >= open). MACD histogram bars use Up and
// Down. HollowUp draws that rising body as an outline and keeps a
// falling body filled, which is the usual A-share candle.
//
// LimitUp and LimitDown are the A-share limit-up and limit-down body
// colors. They are brighter than Up and Down. CandleBodyColor uses them
// only when the caller passes a positive limitPct; the chart's default
// threshold is 0, so ordinary candles stay Up and Down until a host sets
// the percent.
type Style struct {
	Up, Down           ui.Color
	LimitUp, LimitDown ui.Color
	MA5, MA20          ui.Color
	MACDDIF            ui.Color
	MACDDEA            ui.Color
	MACDZero           ui.Color
	// BOLLMid is the SMA. BOLLUp and BOLLLow are the bands around it.
	BOLLMid, BOLLUp, BOLLLow ui.Color
	// KDJK, KDJD, and KDJJ are the three stochastic lines.
	KDJK, KDJD, KDJJ ui.Color
	Grid, Cross      ui.Color
	Text, Muted      ui.Color
	Plot, VolumeBg   ui.Color
	LabelFill        ui.Color
	LabelText        ui.Color
	// Overlay is a drawing that is not selected. OverlayHot is the
	// selected one, including its handles.
	Overlay, OverlayHot ui.Color
	// Compare is the second series in overlay mode, drawn as a close
	// line on the indexed price plot.
	Compare ui.Color
	// HollowUp leaves a rising candle's body empty. Falling candles stay
	// filled either way. The default is filled bodies for both.
	HollowUp bool
	// LimitPct is the close-to-close move, as a fraction, that paints a
	// body with LimitUp or LimitDown. 0.10 is a 10% limit. Zero or
	// negative never uses the limit colors. CandleBodyColor reads the
	// percent it is passed; Paint reads this field.
	LimitPct float64
}

// DarkDefault is the chart style on MyGo's dark theme.
func DarkDefault() Style { return StyleFrom(ui.DarkTheme()) }

// LightDefault is the chart style on MyGo's light theme.
func LightDefault() Style { return StyleFrom(ui.LightTheme()) }

// StyleFrom builds colors that stay readable on the window theme.
// Rising bars are green and falling bars are red, matching KLineChart's
// default, on both light and dark themes. A nil theme is DarkDefault.
func StyleFrom(t *ui.Theme) Style {
	if t == nil {
		t = ui.DarkTheme()
	}
	fill, labelText := t.Inverse, t.InverseText
	if fill == (ui.Color{}) {
		fill = t.Text
	}
	if labelText == (ui.Color{}) {
		labelText = t.Background
	}
	return Style{
		Up:         ui.RGB(38, 166, 154),
		Down:       ui.RGB(239, 83, 80),
		LimitUp:    ui.RGB(0, 230, 118),
		LimitDown:  ui.RGB(255, 59, 48),
		MA5:        t.Accent,
		MA20:       t.Warning,
		MACDDIF:    t.Accent,
		MACDDEA:    t.Warning,
		MACDZero:   t.Border,
		BOLLMid:    t.Text,
		BOLLUp:     t.Accent,
		BOLLLow:    t.Warning,
		KDJK:       t.Accent,
		KDJD:       t.Warning,
		KDJJ:       t.Text,
		Grid:       t.Border,
		Cross:      t.TextMuted,
		Text:       t.Text,
		Muted:      t.TextMuted,
		Plot:       t.Background,
		VolumeBg:   t.Surface,
		LabelFill:  fill,
		LabelText:  labelText,
		Overlay:    ui.RGB(217, 70, 239),
		OverlayHot: ui.RGB(255, 214, 0),
		// Pink, away from the candle green and red, the blue MA accent,
		// and the magenta drawing color.
		Compare: ui.RGB(219, 39, 119),
	}
}

// CandleBodyColor is the wick and body color for c.
// limitPct <= 0 never uses LimitUp or LimitDown: a close at or above open
// is Up, and a lower close is Down. There is no limit rule without a
// percent, because a bar with close == high is not by itself a limit-up.
//
// When limitPct > 0 and prevClose is a positive finite price,
// (close-prevClose)/prevClose >= limitPct uses LimitUp and
// <= -limitPct uses LimitDown. Anything inside the band, or a bar with
// no usable previous close, uses Up or Down. Volume and the MACD
// histogram stay on Up and Down; they do not call this.
func (s Style) CandleBodyColor(c Candle, prevClose, limitPct float64) ui.Color {
	if limitPct > 0 && prevClose > 0 && !math.IsNaN(prevClose) && !math.IsNaN(c.Close) {
		chg := (c.Close - prevClose) / prevClose
		if chg >= limitPct {
			return s.LimitUp
		}
		if chg <= -limitPct {
			return s.LimitDown
		}
	}
	if c.Close >= c.Open {
		return s.Up
	}
	return s.Down
}

// completeStyle fills a partial palette from DarkDefault. A style that
// sets Plot, Grid, or Text is used as given. A style that only overrides
// a candle color would otherwise paint with zero grid and plot colors.
func completeStyle(s Style) Style {
	if s.Plot.A != 0 || s.Grid.A != 0 || s.Text.A != 0 {
		return s
	}
	base := DarkDefault()
	base.HollowUp = s.HollowUp
	base.LimitPct = s.LimitPct
	base.Up = pickColor(base.Up, s.Up)
	base.Down = pickColor(base.Down, s.Down)
	base.LimitUp = pickColor(base.LimitUp, s.LimitUp)
	base.LimitDown = pickColor(base.LimitDown, s.LimitDown)
	base.MA5 = pickColor(base.MA5, s.MA5)
	base.MA20 = pickColor(base.MA20, s.MA20)
	base.MACDDIF = pickColor(base.MACDDIF, s.MACDDIF)
	base.MACDDEA = pickColor(base.MACDDEA, s.MACDDEA)
	base.MACDZero = pickColor(base.MACDZero, s.MACDZero)
	base.BOLLMid = pickColor(base.BOLLMid, s.BOLLMid)
	base.BOLLUp = pickColor(base.BOLLUp, s.BOLLUp)
	base.BOLLLow = pickColor(base.BOLLLow, s.BOLLLow)
	base.KDJK = pickColor(base.KDJK, s.KDJK)
	base.KDJD = pickColor(base.KDJD, s.KDJD)
	base.KDJJ = pickColor(base.KDJJ, s.KDJJ)
	base.Grid = pickColor(base.Grid, s.Grid)
	base.Cross = pickColor(base.Cross, s.Cross)
	base.Text = pickColor(base.Text, s.Text)
	base.Muted = pickColor(base.Muted, s.Muted)
	base.Plot = pickColor(base.Plot, s.Plot)
	base.VolumeBg = pickColor(base.VolumeBg, s.VolumeBg)
	base.LabelFill = pickColor(base.LabelFill, s.LabelFill)
	base.LabelText = pickColor(base.LabelText, s.LabelText)
	base.Overlay = pickColor(base.Overlay, s.Overlay)
	base.OverlayHot = pickColor(base.OverlayHot, s.OverlayHot)
	base.Compare = pickColor(base.Compare, s.Compare)
	return base
}

func pickColor(base, override ui.Color) ui.Color {
	if override.A == 0 {
		return base
	}
	return override
}
