// K-line is a native candlestick demo. The window is ui.View, drawn by
// MyGo itself: no HTML and no WebView. Candles, volume, pan, zoom, and
// the crosshair are painted in this process; ui/chart only supplies
// the price and time scales.
package main

import (
	"log"
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

const (
	sampleBars = 600
	sampleSeed = 42
)

// app is the state the window shows.
type app struct {
	candles []kline.Candle
	ma5     []float64
	ma20    []float64
	vp      kline.Viewport
	// fitted is set once the price plot has a width, so the first frame
	// opens on the latest bars instead of bar 0.
	fitted bool
	// hovering and hoverX/hoverY are the pointer in the chart element's
	// local DIPs. plotW is the price-plot width from the last paint,
	// which wheel and drag use before the next layout.
	hovering       bool
	hoverX, hoverY float32
	plotW          float64
}

func newApp() *app {
	cs := kline.Generate(sampleBars, sampleSeed)
	return &app{
		candles: cs,
		ma5:     kline.SMA(cs, 5),
		ma20:    kline.SMA(cs, 20),
		vp:      kline.Viewport{BarWidth: kline.DefaultBarWidth},
	}
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	style := kline.StyleFrom(t)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(10, 14).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "K-line").Bold().FontSize(16)
			ui.Textf(c, "%d bars", len(a.candles)).FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, "MA5").FontSize(12).TextColor(style.MA5)
			ui.Text(c, "MA20").FontSize(12).TextColor(style.MA20)
			ui.Text(c, a.readout()).Font("monospace").FontSize(12).NoWrap().Grow(1)
			if ui.Button(c, "Reset view").Clicked() {
				a.vp.BarWidth = kline.DefaultBarWidth
				a.fitted = false
			}
		})
		ui.Text(c, "Drag to pan · wheel to zoom · the crosshair reads the bar under the pointer").
			Padding(0, 14, 8, 14).FontSize(12).TextColor(t.TextMuted)

		chart := ui.Box(c.Key("chart")).Grow(1).WidthPercent(100).
			Label("Candlestick chart").Cursor(ui.CursorCrosshair)
		chart.HandleInput(a.onInput)
		// Dragged is how far the pointer moved since the last frame while
		// the button is down. Positive dx follows the pointer to the right.
		if dx, _, ok := chart.Dragged(); ok && dx != 0 {
			a.vp.Pan(float64(dx), a.plotW, len(a.candles))
		}
		if x, y, over := chart.PointerPosition(); over {
			a.hovering = true
			a.hoverX, a.hoverY = x, y
		} else if _, _, pressed := chart.Dragged(); !pressed {
			a.hovering = false
		}
		chart.Draw(func(p *ui.Painter, r ui.Rect) {
			a.paint(p, r, t)
		})
	})
}

// onInput takes the wheel. MyGo delivers InputScroll in DIPs. Positive
// DY is the direction that scrolls content downward, and that zooms the
// bars out. Positive DX scrolls toward later bars, so it pans the
// opposite way from a drag to the right (a drag follows the pointer).
func (a *app) onInput(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll {
		return false
	}
	if ev.DY != 0 {
		// ev.X is local to the chart box. The price plot starts at its
		// left edge, so that x is already the zoom anchor.
		a.vp.Zoom(float64(ev.DY), float64(ev.X), a.plotW, len(a.candles))
	}
	if ev.DX != 0 {
		a.vp.Pan(-float64(ev.DX), a.plotW, len(a.candles))
	}
	return ev.DX != 0 || ev.DY != 0
}

func (a *app) paint(p *ui.Painter, r ui.Rect, t *ui.Theme) {
	panes := kline.Split(r)
	a.plotW = float64(panes.Price.W)
	// The plot width is only known once layout has run, which is inside
	// Draw. Fitting here puts the latest bars on screen on the first
	// frame instead of waiting for a second build.
	if !a.fitted && panes.Price.W > 1 {
		a.vp.FitRight(len(a.candles), a.plotW)
		a.fitted = true
	}
	a.vp.Clamp(len(a.candles), a.plotW)
	ptr := kline.Pointer{}
	if a.hovering {
		ptr = kline.Pointer{On: true, X: r.X + a.hoverX, Y: r.Y + a.hoverY}
	}
	kline.Paint(p, panes, a.candles, a.ma5, a.ma20, a.vp, ptr, kline.StyleFrom(t))
}

func (a *app) readout() string {
	n := len(a.candles)
	if n == 0 {
		return "No bars"
	}
	i := n - 1
	label := "Last"
	if a.hovering {
		if idx, ok := a.vp.IndexAt(float64(a.hoverX), n); ok {
			i = idx
			label = "Hover"
		}
	}
	ma5, ma20 := math.NaN(), math.NaN()
	if i < len(a.ma5) {
		ma5 = a.ma5[i]
	}
	if i < len(a.ma20) {
		ma20 = a.ma20[i]
	}
	return label + "  " + kline.FormatBar(a.candles[i], ma5, ma20)
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "K-line",
			Width:     1100,
			Height:    680,
			MinWidth:  640,
			MinHeight: 420,
			StateKey:  "kline",
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
