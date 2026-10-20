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
	otherSeed  = 99
)

// series is one symbol. Every series in the window shares the hourly
// clock from kline.Generate, so a bar index is the same time on each.
type series struct {
	name    string
	candles []kline.Candle
	ma5     []float64
	ma20    []float64
}

// app is the state the window shows. One viewport and one crossIndex
// are shared by every pane.
type app struct {
	panes []series
	vp    kline.Viewport
	// fitted is set once a price plot has a width, so the first frame
	// opens on the latest bars instead of bar 0.
	fitted bool
	// plotW is the price-plot width from the last paint. Wheel and drag
	// on any pane use it; the panes are the same width.
	plotW float64
	// crossIndex is the bar under the pointer, or -1 when the pointer
	// is outside every pane. crossPane is the pane that has the pointer,
	// which is the only one that draws the horizontal line.
	crossIndex int
	crossPane  int
	hoverX     float32
	hoverY     float32
}

func newApp() *app {
	return &app{
		panes: []series{
			newSeries("ALPHA", sampleSeed),
			newSeries("BETA", otherSeed),
		},
		vp:         kline.Viewport{BarWidth: kline.DefaultBarWidth},
		crossIndex: -1,
		crossPane:  -1,
	}
}

func newSeries(name string, seed int64) series {
	cs := kline.Generate(sampleBars, seed)
	return series{name: name, candles: cs, ma5: kline.SMA(cs, 5), ma20: kline.SMA(cs, 20)}
}

func (a *app) bars() int {
	if len(a.panes) == 0 {
		return 0
	}
	n := len(a.panes[0].candles)
	for _, s := range a.panes[1:] {
		if len(s.candles) < n {
			n = len(s.candles)
		}
	}
	return n
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	style := kline.StyleFrom(t)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		ui.Row(c).Padding(8, 14, 0, 14).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "K-line").Bold().FontSize(16)
			ui.Textf(c, "%d bars · %d panes", a.bars(), len(a.panes)).FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, "MA5").FontSize(12).TextColor(style.MA5)
			ui.Text(c, "MA20").FontSize(12).TextColor(style.MA20)
			ui.Box(c).Grow(1)
			if ui.Button(c, "Reset view").Clicked() {
				a.vp.BarWidth = kline.DefaultBarWidth
				a.fitted = false
			}
		})
		ui.Text(c, "Drag or scroll either pane. Both share the time range, and the crosshair stays on the same bar.").
			Padding(4, 14, 6, 14).FontSize(12).TextColor(t.TextMuted)

		// Headers are built before the charts report the pointer, so they
		// show crossIndex from the previous frame. The charts record the
		// new index, and a changed index asks for one more frame so every
		// header catches up before the next input.
		nextIndex := -1
		nextPane := -1
		hoverX, hoverY := float32(0), float32(0)
		anyHover := false
		anyPress := false
		for i := range a.panes {
			s := &a.panes[i]
			ui.Row(c).Padding(2, 14, 2, 14).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, s.name).Bold().FontSize(13)
				ui.Text(c, a.readout(i)).Font("monospace").FontSize(12).NoWrap().Grow(1)
			})
			chart := ui.Box(c.Key(s.name)).Grow(1).WidthPercent(100).
				Label(s.name + " chart").Cursor(ui.CursorCrosshair)
			chart.HandleInput(a.onInput)
			if dx, _, pressed := chart.Dragged(); pressed {
				anyPress = true
				if dx != 0 {
					a.vp.Pan(float64(dx), a.plotW, a.bars())
				}
			}
			if x, y, over := chart.PointerPosition(); over {
				anyHover = true
				nextPane = i
				hoverX, hoverY = x, y
				if bar, ok := a.vp.IndexAt(float64(x), a.bars()); ok {
					nextIndex = bar
				}
			}
			showTime := i == len(a.panes)-1
			pane := i
			chart.Draw(func(p *ui.Painter, r ui.Rect) {
				a.paintPane(p, r, t, pane, showTime)
			})
		}
		if !anyHover && !anyPress {
			nextIndex = -1
			nextPane = -1
		}
		if nextIndex != a.crossIndex || nextPane != a.crossPane || hoverX != a.hoverX || hoverY != a.hoverY {
			a.crossIndex = nextIndex
			a.crossPane = nextPane
			a.hoverX, a.hoverY = hoverX, hoverY
			c.AnimationFrame()
		}
	})
}

// onInput takes the wheel from whichever pane the pointer is over.
// MyGo delivers InputScroll in DIPs. Positive DY is the direction that
// scrolls content downward, and that zooms the bars out. Positive DX
// scrolls toward later bars. The viewport is shared, so the other pane
// moves with this one.
func (a *app) onInput(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll {
		return false
	}
	if ev.DY != 0 {
		// ev.X is local to the pane. Each price plot starts at the
		// pane's left edge, so that x is the shared zoom anchor.
		a.vp.Zoom(float64(ev.DY), float64(ev.X), a.plotW, a.bars())
	}
	if ev.DX != 0 {
		a.vp.Pan(-float64(ev.DX), a.plotW, a.bars())
	}
	return ev.DX != 0 || ev.DY != 0
}

func (a *app) paintPane(p *ui.Painter, r ui.Rect, t *ui.Theme, i int, showTime bool) {
	var panes kline.Panes
	if showTime {
		panes = kline.Split(r)
	} else {
		panes = kline.SplitNoTime(r)
	}
	a.plotW = float64(panes.Price.W)
	// The plot width is only known once layout has run, which is inside
	// Draw. Fitting here puts the latest bars on screen on the first
	// frame instead of waiting for a second build.
	if !a.fitted && panes.Price.W > 1 {
		a.vp.FitRight(a.bars(), a.plotW)
		a.fitted = true
	}
	a.vp.Clamp(a.bars(), a.plotW)
	ptr := kline.Pointer{}
	if a.crossPane == i {
		ptr = kline.Pointer{On: true, X: r.X + a.hoverX, Y: r.Y + a.hoverY}
	}
	s := a.panes[i]
	kline.Paint(p, panes, s.candles, s.ma5, s.ma20, a.vp, ptr, a.crossIndex, kline.StyleFrom(t))
}

func (a *app) readout(i int) string {
	if i < 0 || i >= len(a.panes) {
		return ""
	}
	s := a.panes[i]
	n := len(s.candles)
	if n == 0 {
		return "No bars"
	}
	idx := n - 1
	label := "Last"
	if a.crossIndex >= 0 && a.crossIndex < n {
		idx = a.crossIndex
		label = "Hover"
	}
	ma5, ma20 := math.NaN(), math.NaN()
	if idx < len(s.ma5) {
		ma5 = s.ma5[idx]
	}
	if idx < len(s.ma20) {
		ma20 = s.ma20[idx]
	}
	return label + "  " + kline.FormatBar(s.candles[idx], ma5, ma20)
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "K-line",
			Width:     1100,
			Height:    900,
			MinWidth:  640,
			MinHeight: 640,
			StateKey:  "kline",
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
