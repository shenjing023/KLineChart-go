package kline

import (
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func init() {
	mustRegister(Spec{
		Name: "MA",
		Kind: PaneMain,
		New:  func() Indicator { return NewMA(5, 20) },
	})
}

// MA is a simple moving average of Close for each period, drawn on the
// price plot. Periods are in bars. The usual pair is 5 and 20.
type MA struct {
	Periods []int
	// digits is the price precision ValueAt prints. Negative is two,
	// which is what a chart that never set PriceDecimals leaves.
	digits int
	colors []ui.Color
	lines  [][]float64
	// sums is the rolling close sum of each line after the last bar.
	sums []float64
}

// NewMA returns an MA over the given periods. Periods below 1 are dropped.
func NewMA(periods ...int) *MA {
	keep := make([]int, 0, len(periods))
	for _, p := range periods {
		if p >= 1 {
			keep = append(keep, p)
		}
	}
	return &MA{Periods: keep, digits: -1}
}

func (m *MA) Name() string     { return "MA" }
func (m *MA) PaneKind() string { return PaneMain }

// Lines returns the calculated averages, one slice per period, parallel
// to Periods. Nil before Calc.
func (m *MA) Lines() [][]float64 { return m.lines }

// Series and LineColors implement LinePlot. Paint strokes these lines
// and skips DrawMain.
func (m *MA) Series() [][]float64 { return m.lines }

func (m *MA) LineColors() []ui.Color { return m.colors }

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
	m.sums = make([]float64, len(m.Periods))
	for i, period := range m.Periods {
		m.lines[i], m.sums[i] = smaLine(cs, period)
	}
}

// Append extends each average by the bar now at the end of cs.
func (m *MA) Append(cs []Candle) {
	if m == nil || len(cs) == 0 {
		return
	}
	last := len(cs) - 1
	if len(m.lines) != len(m.Periods) || len(m.sums) != len(m.Periods) {
		m.Calc(cs)
		return
	}
	for pi := range m.Periods {
		if len(m.lines[pi]) != last {
			m.Calc(cs)
			return
		}
	}
	close := cs[last].Close
	for pi, period := range m.Periods {
		sum := m.sums[pi] + close
		if period >= 1 && last >= period {
			sum -= cs[last-period].Close
		}
		m.sums[pi] = sum
		v := math.NaN()
		if period >= 1 && last >= period-1 {
			v = sum / float64(period)
		}
		m.lines[pi] = append(m.lines[pi], v)
	}
}

// Revise replaces the last average after prev was overwritten in cs.
func (m *MA) Revise(cs []Candle, prev Candle) {
	if m == nil || len(cs) == 0 {
		return
	}
	n := len(cs)
	if len(m.lines) != len(m.Periods) || len(m.sums) != len(m.Periods) {
		m.Calc(cs)
		return
	}
	for pi := range m.Periods {
		if len(m.lines[pi]) != n {
			m.Calc(cs)
			return
		}
	}
	last := n - 1
	close := cs[last].Close
	for pi, period := range m.Periods {
		sum := m.sums[pi] - prev.Close + close
		m.sums[pi] = sum
		if period >= 1 && last >= period-1 {
			m.lines[pi][last] = sum / float64(period)
			continue
		}
		m.lines[pi][last] = math.NaN()
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

// SetPriceDigits makes ValueAt print prices with digits decimals, which
// is what ChartOptions.PriceDecimals asks a price-dimension indicator to
// do. The zero value keeps two.
func (m *MA) SetPriceDigits(digits int) {
	if m != nil {
		m.digits = digits
	}
}

// Values publishes each average as MA<period>. A warm-up gap is absent
// rather than NaN, so a host can tell "not ready" from a real number.
func (m *MA) Values(i int) map[string]float64 {
	if m == nil || i < 0 {
		return nil
	}
	var out map[string]float64
	for pi, period := range m.Periods {
		if pi >= len(m.lines) || i >= len(m.lines[pi]) {
			continue
		}
		v := m.lines[pi][i]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		if out == nil {
			out = make(map[string]float64, len(m.Periods))
		}
		out["MA"+strconv.Itoa(period)] = v
	}
	return out
}

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
		b = append(b, formatPrice(v, m.digits)...)
	}
	return string(b)
}
