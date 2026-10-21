// Command extend is a runnable example of the extension points: a host
// indicator registered through kline.Register, and a host drawing tool
// registered through kline.RegisterOverlay. Both are registered from
// init, before NewChart, so the chart picks them up when it is built.
//
//	go run ./examples/extend
//
// Add -headless to run the same view as a console smoke without a window.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

func init() {
	if err := kline.Register(kline.Spec{
		Name: "MID",
		Kind: kline.PaneMain,
		New:  func() kline.Indicator { return &midLine{} },
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

type extendExample struct {
	chart *kline.Chart
	err   error
}

func newExtendExample() *extendExample {
	a := &extendExample{chart: kline.NewChart(kline.ChartOptions{
		Source:  &kline.SyntheticSource{Extra: -1},
		Symbols: []string{"600519"},
		// MID is a main-pane line. It stays in price units, so it
		// widens the price scale the same way the built-in MA does.
		Indicators: []string{"MID", "MA"},
		Style: kline.Style{
			Overlay: ui.RGB(0, 220, 255), OverlayHot: ui.RGB(255, 80, 190),
		},
	})}
	a.err = a.chart.LoadError()
	return a
}

func (a *extendExample) view(c *ui.Context) {
	ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
		ui.Row(c).Padding(8, 12, 8, 12).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "HOST EXTENSIONS").Bold()
			if ui.Button(c, "Toggle MID").Clicked() {
				a.err = a.chart.SetIndicator("MID", !a.chart.IndicatorOn("MID"))
			}
			for _, tool := range []struct{ label, kind string }{{"Pan", kline.ToolPan}, {"Mark", "mark"}} {
				var button ui.Element
				if a.chart.Tool() == tool.kind {
					button = ui.PrimaryButton(c, tool.label)
				} else {
					button = ui.Button(c, tool.label)
				}
				if button.Clicked() {
					a.err = a.chart.SetTool(tool.kind)
				}
			}
			ui.Textf(c, "MID: %t | Tool: %s", a.chart.IndicatorOn("MID"), a.chart.Tool()).FontSize(12)
		})
		ui.Text(c, "Mark: click to place, drag to move; Delete or right-click to remove.").
			Padding(0, 12, 6, 12).FontSize(12)
		if a.err != nil {
			ui.Textf(c, "Error: %v", a.err).Padding(0, 12, 6, 12)
		}
		ui.Column(c).Grow(1).WidthPercent(100).Children(func() { a.chart.View(c) })
	})
}

func (a *extendExample) smoke(w io.Writer) error {
	if a.err != nil {
		return a.err
	}
	tester := ui.NewTester(a.view, 960, 680)
	if err := tester.Click("Mark"); err != nil {
		return err
	}
	box, ok := tester.Find("600519 chart")
	if !ok {
		return fmt.Errorf("chart missing")
	}
	tester.ClickAt(box.X+140, box.Y+80)
	tester.Press(box.X+140, box.Y+80)
	tester.Move(box.X+172, box.Y+100)
	tester.Release(box.X+172, box.Y+100)
	fmt.Fprintln(w, "tool:", a.chart.Tool())
	fmt.Fprintln(w, "header shows MID:", strings.Contains(a.chart.Readout("600519"), "MID"))
	tester.Key(0, ui.KeyDelete)
	if err := tester.Click("Toggle MID"); err != nil {
		return err
	}
	fmt.Fprintln(w, "MID after toggle:", a.chart.IndicatorOn("MID"))
	return a.err
}

func main() {
	headless := flag.Bool("headless", false, "run the shared view smoke without a window")
	flag.Parse()
	if *headless {
		a := newExtendExample()
		defer a.chart.Close()
		if err := a.smoke(os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	mygo.App.WhenReady(func() {
		a := newExtendExample()
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: "kline extensions", Width: 960, Height: 680,
			MinWidth: 800, MinHeight: 480, Content: ui.View(a.view),
		})
		window.OnClosed(func() { _ = a.chart.Close() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// midLine is a main-pane indicator in price units: (open+close)/2.
// Series is what the chart strokes, so DrawMain stays empty.
type midLine struct {
	values []float64
	series [][]float64
	color  ui.Color
}

func (r *midLine) Name() string     { return "MID" }
func (r *midLine) PaneKind() string { return kline.PaneMain }

func (r *midLine) Calc(cs []kline.Candle) {
	r.values = make([]float64, len(cs))
	for i, c := range cs {
		r.values[i] = (c.Open + c.Close) / 2
	}
	if cap(r.series) < 1 {
		r.series = make([][]float64, 1)
	}
	r.series = r.series[:1]
	r.series[0] = r.values
}

func (r *midLine) DrawMain(*ui.Painter, ui.Rect, kline.Viewport, chart.Linear) {}
func (r *midLine) DrawSub(*ui.Painter, ui.Rect, kline.Viewport)                {}

// ValueAt is the header text for the hovered bar.
func (r *midLine) ValueAt(i int) string {
	if i < 0 || i >= len(r.values) {
		return ""
	}
	return "MID " + kline.FormatPrice(r.values[i])
}

func (r *midLine) Series() [][]float64 { return r.series }

func (r *midLine) LineColors() []ui.Color {
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
	var path ui.Path
	path.MoveTo(x, y-s)
	path.LineTo(x+s, y)
	path.LineTo(x, y+s)
	path.LineTo(x-s, y)
	path.Close()
	p.FillPath(&path, color)
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
