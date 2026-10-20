package kline

import (
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func init() {
	Register(Spec{
		Name: "MA",
		Kind: PaneMain,
		New:  func() Indicator { return NewMA(5, 20) },
	})
}

// MA is a simple moving average of Close for each period, drawn on the
// price plot. Periods are in bars. The usual pair is 5 and 20.
type MA struct {
	Periods []int
	colors  []ui.Color
	lines   [][]float64
}

// NewMA returns an MA over the given periods. Periods below 1 are dropped.
func NewMA(periods ...int) *MA {
	keep := make([]int, 0, len(periods))
	for _, p := range periods {
		if p >= 1 {
			keep = append(keep, p)
		}
	}
	return &MA{Periods: keep}
}

func (m *MA) Name() string     { return "MA" }
func (m *MA) PaneKind() string { return PaneMain }

// Lines returns the calculated averages, one slice per period, parallel
// to Periods. Nil before Calc.
func (m *MA) Lines() [][]float64 { return m.lines }

// SetColors sets the line color for each period. A missing color keeps
// the previous one. Paint calls this from the theme before DrawMain.
func (m *MA) SetColors(colors []ui.Color) {
	if m == nil {
		return
	}
	if len(m.colors) < len(m.Periods) {
		next := make([]ui.Color, len(m.Periods))
		copy(next, m.colors)
		m.colors = next
	}
	for i := range colors {
		if i >= len(m.colors) {
			break
		}
		m.colors[i] = colors[i]
	}
}

func (m *MA) Calc(cs []Candle) {
	m.lines = make([][]float64, len(m.Periods))
	for i, period := range m.Periods {
		m.lines[i] = SMA(cs, period)
	}
}

func (m *MA) DrawMain(p *ui.Painter, priceRect ui.Rect, vp Viewport, y chart.Linear) {
	if m == nil || len(m.lines) == 0 || priceRect.W < 2 {
		return
	}
	n := 0
	for _, line := range m.lines {
		if len(line) > n {
			n = len(line)
		}
	}
	first, last := vp.Visible(n, float64(priceRect.W))
	for i, line := range m.lines {
		color := ui.Color{}
		if i < len(m.colors) {
			color = m.colors[i]
		}
		if color == (ui.Color{}) {
			continue
		}
		drawMA(p, priceRect, line, vp, y, first, last, color)
	}
}

func (m *MA) DrawSub(*ui.Painter, ui.Rect, Viewport) {}

func (m *MA) ValueAt(i int) string {
	if m == nil || i < 0 {
		return ""
	}
	var b []byte
	for pi, period := range m.Periods {
		if pi >= len(m.lines) || i >= len(m.lines[pi]) {
			continue
		}
		v := m.lines[pi][i]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if len(b) > 0 {
			b = append(b, "  "...)
		}
		b = append(b, "MA"...)
		b = strconv.AppendInt(b, int64(period), 10)
		b = append(b, ' ')
		b = append(b, FormatPrice(v)...)
	}
	return string(b)
}
