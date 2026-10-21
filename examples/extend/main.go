// Command extend is a runnable example of the extension points: a host
// indicator registered through kline.Register, and a host drawing tool
// registered through kline.RegisterOverlay. Both are registered from
// init, before NewChart, so the chart picks them up when it is built.
//
//	go run ./examples/extend
//
// The chart is painted headlessly here so the example can also serve as
// a smoke test of the whole registration path without a window.
package main

import (
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

func init() {
	if err := kline.Register(kline.Spec{
		Name: "RANGE",
		Kind: kline.PaneMain,
		New:  func() kline.Indicator { return &rangeLine{} },
	}); err != nil {
		log.Fatal(err)
	}
	if err := kline.RegisterOverlay(kline.OverlaySpec{
		Kind:    "mark",
		Anchors: 1,
		Draw:    drawMark,
		Hit:     hitMark,
	}); err != nil {
		log.Fatal(err)
	}
}

func main() {
	chart := kline.NewChart(kline.ChartOptions{
		Symbols: []string{"600519"},
		// RANGE is a main-pane line, so it widens the price scale the
		// same way the built-in MA does.
		Indicators: []string{"RANGE", "MA"},
	})
	if err := chart.LoadError(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("period:", chart.Period())
	fmt.Println("bars:", chart.Bars())
	fmt.Println("RANGE registered:", chart.IndicatorOn("RANGE"))
	fmt.Println("RANGE in the toolbar order:", chart.IndicatorNames())

	// The host tool is selected like a built-in one.
	if err := chart.SetTool("mark"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("tool:", chart.Tool())

	// Paint one frame in memory: this is what a window would do. Moving
	// the pointer over the plot makes the header carry each indicator's
	// ValueAt for the hovered bar, which is where RANGE shows up.
	tester := ui.NewTester(chart.View, 900, 620)
	tester.Frame()
	box, ok := tester.Find("600519 chart")
	if ok {
		tester.Move(box.X+box.W/2, box.Y+30)
	}
	fmt.Println("header shows the indicator:", strings.Contains(chart.Readout("600519"), "RANGE"))

	if err := chart.SetIndicator("RANGE", false); err != nil {
		log.Fatal(err)
	}
	fmt.Println("after toggle:", chart.IndicatorOn("RANGE"))
}

// rangeLine is a main-pane indicator drawn as a line plot: the high-low
// range of each bar, scaled onto the price axis. Series is what the chart
// strokes, so DrawMain stays empty.
type rangeLine struct {
	values []float64
	series [][]float64
	color  ui.Color
}

func (r *rangeLine) Name() string     { return "RANGE" }
func (r *rangeLine) PaneKind() string { return kline.PaneMain }

func (r *rangeLine) Calc(cs []kline.Candle) {
	r.values = make([]float64, len(cs))
	for i, c := range cs {
		r.values[i] = c.High - c.Low
	}
	if cap(r.series) < 1 {
		r.series = make([][]float64, 1)
	}
	r.series = r.series[:1]
	r.series[0] = r.values
}

func (r *rangeLine) DrawMain(*ui.Painter, ui.Rect, kline.Viewport, chart.Linear) {}
func (r *rangeLine) DrawSub(*ui.Painter, ui.Rect, kline.Viewport)                {}

// ValueAt is the header text for the hovered bar.
func (r *rangeLine) ValueAt(i int) string {
	if i < 0 || i >= len(r.values) {
		return ""
	}
	return "RANGE " + kline.FormatPrice(r.values[i])
}

func (r *rangeLine) Series() [][]float64 { return r.series }

func (r *rangeLine) LineColors() []ui.Color {
	if r.color == (ui.Color{}) {
		return []ui.Color{ui.RGB(255, 140, 0)}
	}
	return []ui.Color{r.color}
}

// drawMark paints a diamond at the anchor. It must not change o.Points:
// the chart passes the layer's own slice.
func drawMark(p *ui.Painter, plot ui.Rect, o kline.Overlay, vp kline.Viewport, scale chart.Linear, cs []kline.Candle, style kline.Style) {
	if len(o.Points) == 0 {
		return
	}
	x, y := anchorPixel(plot, o.Points[0], vp, scale, cs)
	color := style.Overlay
	if o.Selected {
		color = style.OverlayHot
	}
	const s = 5
	p.Fill(ui.Rect{X: x - s, Y: y - s, W: 2 * s, H: 2 * s}, color, 1)
}

// hitMark reports the anchor when the pointer is within HitRadius.
func hitMark(o kline.Overlay, x, y, _, plotH float64, vp kline.Viewport, scale chart.Linear, cs []kline.Candle) (int, bool) {
	if len(o.Points) == 0 {
		return 0, false
	}
	ax := kline.AnchorX(o.Points[0], vp, cs)
	ay := kline.PriceToLocalY(plotH, o.Points[0].Price, scale)
	if math.Hypot(x-ax, y-ay) <= kline.HitRadius {
		return kline.HitBody, true
	}
	return 0, false
}

func anchorPixel(plot ui.Rect, pt kline.Point, vp kline.Viewport, scale chart.Linear, cs []kline.Candle) (x, y float32) {
	return plot.X + float32(kline.AnchorX(pt, vp, cs)),
		plot.Y + float32(kline.PriceToLocalY(float64(plot.H), pt.Price, scale))
}
