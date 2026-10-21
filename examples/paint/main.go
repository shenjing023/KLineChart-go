// Command paint owns its chrome, viewport, crosshair, and drawing Layer.
// It calls Split and Paint/PaintCompare directly, without Chart.View.
//
//	go run ./examples/paint
package main

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

// All input, view construction, and drawing run on the UI thread.
// No background producer accesses this state, so no mutex is needed.
type paintState struct {
	name, compareName string
	bars, compare     []kline.Candle
	comparing         bool
	indexed           bool
	vp                kline.Viewport
	layer             kline.Layer
	hit               kline.Hit
	local, ptr        kline.Pointer
	index             int
	frame             ui.Rect
	panes             kline.Panes
	fitted            bool
	pressX            float32
	pressOn           bool
}

func newPaintState() *paintState {
	period := kline.Period{Span: 1, Unit: kline.UnitHour}
	source := &kline.SyntheticSource{Extra: -1}
	load := func(symbol string) []kline.Candle {
		bars, _, err := source.GetBars(kline.BarRequest{
			Symbol: symbol, Period: period, Kind: kline.KindInit, Limit: 600,
		})
		if err != nil {
			panic(err)
		}
		return bars
	}
	s := &paintState{
		name: "600519", compareName: "000001",
		bars: load("600519"), compare: load("000001"),
		vp: kline.Viewport{BarWidth: kline.DefaultBarWidth}, index: -1,
	}
	s.layer.Add(kline.OverlayHLine, kline.Point{Price: s.bars[len(s.bars)-1].Close})
	s.layer.Select("")
	return s
}

// DrawOverlays and Layer.Hit both take raw primary prices, even when
// PaintCompare draws an indexed axis. Never use its index scale here.
func (s *paintState) rawScale() chart.Linear {
	w := float64(s.panes.Price.W)
	if s.comparing {
		_, raw, _ := kline.CompareScales(s.bars, s.compare, nil, s.vp, w)
		return raw
	}
	return kline.MainScale(s.bars, nil, s.vp, w)
}

func contains(r ui.Rect, x, y float32) bool {
	return r.W > 0 && r.H > 0 && x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

func (s *paintState) updateHover() {
	s.index, s.ptr, s.hit = -1, kline.Pointer{}, kline.Hit{}
	if !s.local.On {
		return
	}
	// Input and PointerPosition are element-local; Paint wants window DIPs.
	x, y := s.frame.X+s.local.X, s.frame.Y+s.local.Y
	price := s.panes.Price
	inPrice := contains(price, x, y)
	if !inPrice && !contains(s.panes.Volume, x, y) {
		return // Axes, the pane gap, and host chrome have no crosshair.
	}
	plotX, plotY := float64(x-price.X), float64(y-price.Y)
	if i, ok := s.vp.IndexAt(plotX, len(s.bars)); ok {
		s.index = i
		s.ptr = kline.Pointer{On: true, X: x, Y: y}
	}
	if inPrice {
		s.hit, _ = s.layer.Hit(plotX, plotY, float64(price.W), float64(price.H), s.vp, s.rawScale(), s.bars)
	}
}

func (s *paintState) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputScroll, ui.InputPointerDown, ui.InputPointerMove, ui.InputPointerUp:
		s.local = kline.Pointer{On: true, X: ev.X, Y: ev.Y}
		s.updateHover()
	default:
		return false
	}
	w := float64(s.panes.Price.W)
	x, y := s.frame.X+ev.X, s.frame.Y+ev.Y
	inPlot := contains(s.panes.Price, x, y) || contains(s.panes.Volume, x, y)
	switch ev.Kind {
	case ui.InputScroll:
		if !inPlot || ev.DY == 0 || w < 2 {
			return false
		}
		s.vp.Zoom(float64(ev.DY), float64(x-s.panes.Price.X), w, len(s.bars))
	case ui.InputPointerDown:
		if ev.Button != 0 || !inPlot || w < 2 {
			return false
		}
		s.pressX, s.pressOn = ev.X, true
		s.layer.Select(s.hit.ID)
	case ui.InputPointerMove:
		if s.pressOn && w >= 2 {
			s.vp.Pan(float64(ev.X-s.pressX), w, len(s.bars))
			s.pressX = ev.X
		}
	case ui.InputPointerUp:
		if ev.Button != 0 {
			return false
		}
		s.pressOn = false
	}
	// Pan and zoom change the bar under a stationary pointer too.
	s.updateHover()
	return true
}

func (s *paintState) draw(p *ui.Painter, r ui.Rect) {
	s.frame, s.panes = r, kline.Split(r)
	w := float64(s.panes.Price.W)
	if !s.fitted && w >= 2 {
		s.vp.FitRight(len(s.bars), w)
		s.fitted = true
	}
	s.vp.Clamp(len(s.bars), w)
	s.updateHover()
	style := kline.DarkDefault()
	s.indexed = false
	if s.comparing {
		s.indexed = kline.PaintCompare(p, s.panes, s.bars, s.compare, nil, s.vp, s.ptr, s.index, style)
	} else {
		kline.Paint(p, s.panes, s.bars, nil, s.vp, s.ptr, s.index, style)
	}
	kline.DrawOverlays(p, s.panes.Price, s.layer.Items(), s.vp, s.rawScale(), s.bars, style, nil)
}

func (s *paintState) status() string {
	mode := "Paint"
	if s.comparing {
		mode = "PaintCompare (indexed to 100)"
		if !s.indexed {
			mode = "PaintCompare (raw fallback)"
		}
	}
	first, last := s.vp.Visible(len(s.bars), float64(s.panes.Price.W))
	hit := "none"
	if s.hit.ID != "" {
		hit = s.hit.ID
	}
	return fmt.Sprintf("%s | bars %d..%d | width %.2f | offset %.2f | Layer hit: %s",
		mode, first, last, s.vp.BarWidth, s.vp.Offset, hit)
}

func (s *paintState) readout() string {
	if s.index < 0 {
		return "Hover price/volume for OHLC; drag to pan; wheel up/down to zoom; click the price line to select."
	}
	b := s.bars[s.index]
	text := fmt.Sprintf("Hover #%d %s UTC  O %.2f H %.2f L %.2f C %.2f V %.0f",
		s.index, b.Time.UTC().Format("2006-01-02 15:04"), b.Open, b.High, b.Low, b.Close, b.Volume)
	if contains(s.panes.Price, s.ptr.X, s.ptr.Y) {
		text += fmt.Sprintf(" | raw pointer %.2f", kline.LocalYToPrice(float64(s.panes.Price.H), float64(s.ptr.Y-s.panes.Price.Y), s.rawScale()))
	}
	return text
}

func paintView(state *paintState) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Fill().Padding(0, 16, 12, 16).Background(c.Theme().Background).Children(func() {
			ui.Row(c).Padding(10, 0, 6, 0).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "PAINT API").Bold().FontSize(15)
				ui.Text(c, state.name+" / "+state.compareName+" (synthetic 1h)").FontSize(12).
					TextColor(c.Theme().TextMuted)
				if ui.Toggle(c, &state.comparing, "PaintCompare").Changed() {
					state.updateHover()
					c.Invalidate()
				}
			})
			plot := ui.Box(c).Grow(1).WidthPercent(100).Focusable().
				Cursor(ui.CursorCrosshair).Label("paint plot")
			plot.HandleInput(state.input)
			x, y, over := plot.PointerPosition()
			state.local = kline.Pointer{On: over, X: x, Y: y}
			state.updateHover() // Also clears hover when no move event reaches this element.
			plot.Draw(func(p *ui.Painter, r ui.Rect) {
				before, readout := state.status(), state.readout()
				state.draw(p, r)
				if before != state.status() || readout != state.readout() {
					c.Invalidate() // Layout/fitting can change the status after the view was built.
				}
			})
			ui.Text(c, state.status()).FontSize(11).NoWrap()
			ui.Text(c, state.readout()).FontSize(11).NoWrap()
		})
	}
}

func main() {
	state := newPaintState()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "kline paint API",
			Width:     1000,
			Height:    760,
			MinWidth:  640,
			MinHeight: 500,
			Content:   ui.View(paintView(state)),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
