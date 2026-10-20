package kline

import "github.com/egoist/mygo/ui"

// Style is the chart's colors and the candle body variant.
// Up is a rising close (close >= open). MACD histogram bars use Up and
// Down. HollowUp draws that rising body as an outline and keeps a
// falling body filled, which is the usual A-share candle.
type Style struct {
	Up, Down  ui.Color
	MA5, MA20 ui.Color
	MACDDIF   ui.Color
	MACDDEA   ui.Color
	MACDZero  ui.Color
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
