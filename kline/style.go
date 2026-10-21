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
	// Buy and Sell color trade markers. Zero means the rising and
	// falling candle colors: Buy follows Up, Sell follows Down.
	Buy, Sell ui.Color
	// Clear names colors that stay transparent. On a partial style,
	// a zero color otherwise means "not set" and completeStyle fills
	// it from DarkDefault, so a transparent grid cannot be expressed
	// by leaving Grid at zero. Set the bit and the field stays
	// transparent. A style that already sets Plot, Grid, or Text is
	// kept, including any transparent field, and these bits still
	// force the named fields to transparent.
	Clear ColorClear
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

// StyleFrom builds colors that stay readable on the window theme. The
// defaults follow mainland China market conventions: rising bars are
// red and falling bars are green on both light and dark themes. A nil
// theme is DarkDefault.
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
	up, down := ui.RGB(239, 83, 80), ui.RGB(38, 166, 154)
	return Style{
		Up:         up,
		Down:       down,
		Buy:        up,
		Sell:       down,
		LimitUp:    ui.RGB(255, 59, 48),
		LimitDown:  ui.RGB(0, 230, 118),
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

// ColorClear marks Style fields that are intentionally transparent.
type ColorClear uint64

const (
	ClearUp ColorClear = 1 << iota
	ClearDown
	ClearLimitUp
	ClearLimitDown
	ClearMA5
	ClearMA20
	ClearMACDDIF
	ClearMACDDEA
	ClearMACDZero
	ClearBOLLMid
	ClearBOLLUp
	ClearBOLLLow
	ClearKDJK
	ClearKDJD
	ClearKDJJ
	ClearGrid
	ClearCross
	ClearText
	ClearMuted
	ClearPlot
	ClearVolumeBg
	ClearLabelFill
	ClearLabelText
	ClearOverlay
	ClearOverlayHot
	ClearCompare
	ClearBuy
	ClearSell
)

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
// sets Plot, Grid, or Text is used as given, so a transparent field on
// that palette stays transparent. A style that only overrides a candle
// color would otherwise paint with zero grid and plot colors. Clear
// forces the named fields to transparent either way.
func completeStyle(s Style) Style {
	if s.Plot.A != 0 || s.Grid.A != 0 || s.Text.A != 0 {
		if s.Buy.A == 0 && s.Clear&ClearBuy == 0 {
			s.Buy = s.Up
		}
		if s.Sell.A == 0 && s.Clear&ClearSell == 0 {
			s.Sell = s.Down
		}
		return applyClear(s)
	}
	base := DarkDefault()
	base.HollowUp = s.HollowUp
	base.LimitPct = s.LimitPct
	base.Clear = s.Clear
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
	base.Buy = pickColor(base.Up, s.Buy)
	base.Sell = pickColor(base.Down, s.Sell)
	return applyClear(base)
}

func applyClear(s Style) Style {
	if s.Clear == 0 {
		return s
	}
	if s.Clear&ClearUp != 0 {
		s.Up = ui.Color{}
	}
	if s.Clear&ClearDown != 0 {
		s.Down = ui.Color{}
	}
	if s.Clear&ClearLimitUp != 0 {
		s.LimitUp = ui.Color{}
	}
	if s.Clear&ClearLimitDown != 0 {
		s.LimitDown = ui.Color{}
	}
	if s.Clear&ClearMA5 != 0 {
		s.MA5 = ui.Color{}
	}
	if s.Clear&ClearMA20 != 0 {
		s.MA20 = ui.Color{}
	}
	if s.Clear&ClearMACDDIF != 0 {
		s.MACDDIF = ui.Color{}
	}
	if s.Clear&ClearMACDDEA != 0 {
		s.MACDDEA = ui.Color{}
	}
	if s.Clear&ClearMACDZero != 0 {
		s.MACDZero = ui.Color{}
	}
	if s.Clear&ClearBOLLMid != 0 {
		s.BOLLMid = ui.Color{}
	}
	if s.Clear&ClearBOLLUp != 0 {
		s.BOLLUp = ui.Color{}
	}
	if s.Clear&ClearBOLLLow != 0 {
		s.BOLLLow = ui.Color{}
	}
	if s.Clear&ClearKDJK != 0 {
		s.KDJK = ui.Color{}
	}
	if s.Clear&ClearKDJD != 0 {
		s.KDJD = ui.Color{}
	}
	if s.Clear&ClearKDJJ != 0 {
		s.KDJJ = ui.Color{}
	}
	if s.Clear&ClearGrid != 0 {
		s.Grid = ui.Color{}
	}
	if s.Clear&ClearCross != 0 {
		s.Cross = ui.Color{}
	}
	if s.Clear&ClearText != 0 {
		s.Text = ui.Color{}
	}
	if s.Clear&ClearMuted != 0 {
		s.Muted = ui.Color{}
	}
	if s.Clear&ClearPlot != 0 {
		s.Plot = ui.Color{}
	}
	if s.Clear&ClearVolumeBg != 0 {
		s.VolumeBg = ui.Color{}
	}
	if s.Clear&ClearLabelFill != 0 {
		s.LabelFill = ui.Color{}
	}
	if s.Clear&ClearLabelText != 0 {
		s.LabelText = ui.Color{}
	}
	if s.Clear&ClearOverlay != 0 {
		s.Overlay = ui.Color{}
	}
	if s.Clear&ClearOverlayHot != 0 {
		s.OverlayHot = ui.Color{}
	}
	if s.Clear&ClearCompare != 0 {
		s.Compare = ui.Color{}
	}
	if s.Clear&ClearBuy != 0 {
		s.Buy = ui.Color{}
	}
	if s.Clear&ClearSell != 0 {
		s.Sell = ui.Color{}
	}
	return s
}

func pickColor(base, override ui.Color) ui.Color {
	if override.A == 0 {
		return base
	}
	return override
}
