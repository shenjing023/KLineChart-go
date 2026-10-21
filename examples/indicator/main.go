// Command indicator demonstrates a host SubPlot and BarTail, including manual
// append/revise and a second indicator registered after the chart was created.
// Its continuous hourly fixture is illustrative, not an exchange calendar.
//
// go run ./examples/indicator
package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

func bandSpec(name string, window int) kline.Spec {
	return kline.Spec{Name: name, Kind: kline.PaneSub,
		New: func() kline.Indicator { return &hostBand{name: name, window: window} }}
}

func init() {
	if err := kline.Register(bandSpec("HOSTBAND", 3)); err != nil {
		log.Fatal(err)
	}
}

// hostBand plots a mean of close-open. It keeps only values, not cached
// extrema: Scale always reflects the visible window, even after a revision
// removes its former minimum or maximum.
type hostBand struct {
	name         string
	window       int
	values, mean []float64
}

var (
	_ kline.Indicator = (*hostBand)(nil)
	_ kline.SubPlot   = (*hostBand)(nil)
	_ kline.BarTail   = (*hostBand)(nil)
)

func (h *hostBand) Name() string {
	if h.name == "" {
		return "HOSTBAND"
	}
	return h.name
}
func (h *hostBand) PaneKind() string { return kline.PaneSub }
func (h *hostBand) span() int {
	if h.window <= 0 {
		return 3
	}
	return h.window
}

func (h *hostBand) Calc(cs []kline.Candle) {
	h.values = make([]float64, len(cs))
	h.mean = make([]float64, len(cs))
	for i, c := range cs {
		h.values[i] = c.Close - c.Open
		h.mean[i] = meanTail(h.values[:i+1], h.span())
	}
}

func meanTail(values []float64, n int) float64 {
	if len(values) < n {
		return math.NaN()
	}
	sum := 0.0
	for _, v := range values[len(values)-n:] {
		sum += v
	}
	return sum / float64(n)
}

func (h *hostBand) Append(cs []kline.Candle) {
	if len(cs) == 0 || len(h.values) != len(cs)-1 {
		h.Calc(cs)
		return
	}
	c := cs[len(cs)-1]
	h.values = append(h.values, c.Close-c.Open)
	h.mean = append(h.mean, meanTail(h.values, h.span()))
}

func (h *hostBand) Revise(cs []kline.Candle, _ kline.Candle) {
	if len(cs) == 0 || len(h.values) != len(cs) {
		h.Calc(cs)
		return
	}
	i := len(cs) - 1
	h.values[i] = cs[i].Close - cs[i].Open
	h.mean[i] = meanTail(h.values, h.span())
}

func (h *hostBand) DrawMain(*ui.Painter, ui.Rect, kline.Viewport, chart.Linear) {}

func (h *hostBand) DrawSub(p *ui.Painter, rect ui.Rect, vp kline.Viewport) {
	if p == nil || rect.W < 2 || rect.H < 2 {
		return
	}
	first, last := vp.Visible(len(h.mean), float64(rect.W))
	scale := h.Scale(first, last) // Identical window to the SubPlot axis.
	var path ui.Path
	drawing := false
	for i := first; i <= last; i++ {
		v := h.mean[i]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			drawing = false
			continue
		}
		x := rect.X + float32(vp.BarLeft(i)+vp.BarWidth/2)
		y := chart.Y(rect, scale.At(v))
		if !drawing {
			path.MoveTo(x, y)
			drawing = true
		} else {
			path.LineTo(x, y)
		}
	}
	p.StrokePath(&path, 1.5, ui.RGB(0, 200, 255))
}

func (h *hostBand) ValueAt(i int) string {
	if i < 0 || i >= len(h.mean) || math.IsNaN(h.mean[i]) || math.IsInf(h.mean[i], 0) {
		return ""
	}
	return h.Name() + " " + kline.FormatPrice(h.mean[i])
}

func (h *hostBand) Scale(first, last int) chart.Linear {
	lo, hi := -1.0, 1.0
	first, last = max(first, 0), min(last, len(h.mean)-1)
	for i := first; i <= last; i++ {
		v := h.mean[i]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return chart.NewLinear(lo, hi).Nice(3)
}

type indicatorFeed []kline.Candle

func (f indicatorFeed) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	if req.Symbol != "600519" {
		return nil, false, kline.ErrUnknownSymbol
	}
	if req.Period != (kline.Period{Span: 1, Unit: kline.UnitHour}) {
		return nil, false, kline.ErrInvalidPeriod
	}
	end := len(f)
	switch req.Kind {
	case kline.KindInit:
	case kline.KindForward:
		if req.Anchor.IsZero() {
			return nil, false, fmt.Errorf("fixture: missing forward anchor")
		}
		end = sort.Search(len(f), func(i int) bool { return !f[i].Time.Before(req.Anchor) })
	default:
		return nil, false, kline.ErrUnsupportedKind
	}
	limit := req.Limit
	if limit <= 0 {
		limit = kline.DefaultLimit
	}
	first := max(end-limit, 0)
	return append([]kline.Candle(nil), f[first:end]...), first > 0, nil
}

type indicatorDemo struct {
	chart *kline.Chart
	err   error
}

func newIndicatorDemo() (*indicatorDemo, error) {
	chart := kline.NewChart(kline.ChartOptions{Source: indicatorFeed(kline.Generate(120, 13)),
		Symbols: []string{"600519"}, Indicators: []string{"MA", "HOSTBAND"}})
	if err := chart.LoadError(); err != nil {
		return nil, err
	}
	return &indicatorDemo{chart: chart}, nil
}

func (d *indicatorDemo) changeTail(appendBar bool) error {
	last, ok := d.chart.Bar("600519", d.chart.Bars()-1)
	if !ok {
		return fmt.Errorf("fixture has no last bar")
	}
	if appendBar {
		last.Time = last.Time.Add(time.Hour)
		last.Open = last.Close
		last.Close = last.Open + 1 + float64(d.chart.Bars()%5)
	} else {
		delta := 6.0
		if last.Close >= last.Open {
			delta = -6
		}
		last.Close = last.Open + delta
	}
	last.High, last.Low = math.Max(last.Open, last.Close)+0.5, math.Min(last.Open, last.Close)-0.5
	last.Volume += 100
	if appendBar {
		return d.chart.AppendBar("600519", last)
	}
	return d.chart.UpdateLast("600519", last)
}

func (d *indicatorDemo) registerFast() error {
	if err := kline.Register(bandSpec("HOSTFAST", 1)); err != nil && !errors.Is(err, kline.ErrDuplicateIndicator) {
		return err
	}
	d.chart.SyncIndicators() // New specs arrive disabled; turn this one on explicitly.
	return d.chart.SetIndicator("HOSTFAST", true)
}

func indicatorView(d *indicatorDemo) func(*ui.Context) {
	chart := d.chart
	return func(c *ui.Context) {
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
			ui.Row(c).Padding(10, 16, 4, 16).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "HOST SUB-PANE").Bold().FontSize(15)
				ui.Box(c).Grow(1)
				if ui.Button(c, "Append").Clicked() {
					d.err = d.changeTail(true)
					c.Invalidate()
				}
				if ui.Button(c, "Revise").Clicked() {
					d.err = d.changeTail(false)
					c.Invalidate()
				}
				if ui.Button(c, "Register fast").Clicked() {
					d.err = d.registerFast()
					c.Invalidate()
				}
			})
			ui.Row(c).Padding(0, 16, 4, 16).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Toggle(c, chart.IndicatorEnabled("HOSTBAND"), "HOSTBAND").Changed() {
					c.Invalidate()
				}
				if enabled := chart.IndicatorEnabled("HOSTFAST"); enabled != nil {
					if ui.Toggle(c, enabled, "HOSTFAST").Changed() {
						c.Invalidate()
					}
				}
				ui.Text(c, fmt.Sprintf("%d bars | 3-bar mean of close-open; fast = 1 bar", chart.Bars())).FontSize(12)
			})
			if d.err != nil {
				ui.Text(c, "error: "+d.err.Error()).Padding(0, 16, 8, 16)
			}
			ui.Column(c).Grow(1).WidthPercent(100).Children(func() { chart.View(c) })
		})
	}
}

func main() {
	d, err := newIndicatorDemo()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := d.chart.Close(); err != nil {
			log.Print(err)
		}
	}()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "kline host indicator", Width: 960, Height: 760,
			MinWidth: 640, MinHeight: 500, Content: ui.View(indicatorView(d))})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
