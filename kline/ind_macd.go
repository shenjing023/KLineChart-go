package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func init() {
	mustRegister(Spec{
		Name: "MACD",
		Kind: PaneSub,
		New:  func() Indicator { return NewMACD(12, 26, 9) },
	})
}

// MACD is the moving-average convergence divergence of Close.
// Short, Long, and Signal are the bar periods, usually 12, 26, and 9.
//
// DIF is EMA(short) - EMA(long). DEA is the EMA of DIF over Signal.
// Each EMA's first value is the SMA of that many samples, which is the
// KLineChart seed, and later values are
// (2*value + (period-1)*prev) / (period+1). The histogram is
// 2*(DIF-DEA). DIF starts once both EMAs exist. DEA starts once Signal
// DIF values exist.
type MACD struct {
	Short, Long, Signal                int
	DIF, DEA, Hist                     []float64
	DIFColor, DEAColor, Up, Down, Zero ui.Color
	// run is the state after the last bar. prev is the state before it,
	// so a revise of that bar does not walk the series again.
	run, prev macdRun
}

// macdRun is the EMA state after n closes. closeSum is only the seed
// for the first EMA of each length; later values use emaS and emaL.
type macdRun struct {
	closeSum     float64
	emaS, emaL   float64
	haveS, haveL bool
	difSum, dea  float64
	n            int
}

// NewMACD returns a MACD with the given periods. Draw colors start as
// DarkDefault; Paint replaces them from the active Style before drawing.
func NewMACD(short, long, signal int) *MACD {
	s := DarkDefault()
	return &MACD{
		Short: short, Long: long, Signal: signal,
		DIFColor: s.MACDDIF,
		DEAColor: s.MACDDEA,
		Up:       s.Up,
		Down:     s.Down,
		Zero:     s.MACDZero,
	}
}

func (m *MACD) Name() string     { return "MACD" }
func (m *MACD) PaneKind() string { return PaneSub }

// SetColors replaces the DIF line, DEA line, histogram, and zero line.
func (m *MACD) SetColors(dif, dea, up, down, zero ui.Color) {
	if m == nil {
		return
	}
	m.DIFColor, m.DEAColor, m.Up, m.Down, m.Zero = dif, dea, up, down, zero
}

func (m *MACD) Calc(cs []Candle) {
	n := len(cs)
	m.DIF = nanSlice(n)
	m.DEA = nanSlice(n)
	m.Hist = nanSlice(n)
	m.run = macdRun{}
	m.prev = macdRun{}
	if n == 0 || m.Short < 1 || m.Long < 1 || m.Signal < 1 || m.Short == m.Long {
		return
	}
	st := macdRun{}
	for i := 0; i < n; i++ {
		prev := st
		st, m.DIF[i], m.DEA[i], m.Hist[i] = m.step(st, cs[i].Close)
		if i == n-1 {
			m.prev = prev
		}
	}
	m.run = st
}

// step applies one close. i is st.n. DIF, DEA, and the histogram stay
// NaN until their seeds exist, matching a full Calc.
func (m *MACD) step(st macdRun, close float64) (macdRun, float64, float64, float64) {
	dif, dea, hist := math.NaN(), math.NaN(), math.NaN()
	short, long, signal := m.Short, m.Long, m.Signal
	if short < 1 || long < 1 || signal < 1 || short == long {
		st.n++
		return st, dif, dea, hist
	}
	i := st.n
	st.closeSum += close
	if i >= short-1 {
		if i > short-1 {
			st.emaS = (2*close + float64(short-1)*st.emaS) / float64(short+1)
		} else {
			st.emaS = st.closeSum / float64(short)
		}
		st.haveS = true
	}
	if i >= long-1 {
		if i > long-1 {
			st.emaL = (2*close + float64(long-1)*st.emaL) / float64(long+1)
		} else {
			st.emaL = st.closeSum / float64(long)
		}
		st.haveL = true
	}
	maxPeriod := long
	if short > maxPeriod {
		maxPeriod = short
	}
	if i >= maxPeriod-1 && st.haveS && st.haveL {
		d := st.emaS - st.emaL
		dif = d
		st.difSum += d
		if i >= maxPeriod+signal-2 {
			if i > maxPeriod+signal-2 {
				st.dea = (d*2 + st.dea*float64(signal-1)) / float64(signal+1)
			} else {
				st.dea = st.difSum / float64(signal)
			}
			dea = st.dea
			hist = (d - dea) * 2
		}
	}
	st.n++
	return st, dif, dea, hist
}

func (m *MACD) tailLen(n int) bool {
	return m != nil && m.Short >= 1 && m.Long >= 1 && m.Signal >= 1 && m.Short != m.Long &&
		len(m.DIF) == n && len(m.DEA) == n && len(m.Hist) == n && m.run.n == n
}

// Append extends DIF, DEA, and the histogram by the last close in cs.
func (m *MACD) Append(cs []Candle) {
	if m == nil || len(cs) == 0 || !m.tailLen(len(cs)-1) {
		if m != nil {
			m.Calc(cs)
		}
		return
	}
	prev := m.run
	st, dif, dea, hist := m.step(m.run, cs[len(cs)-1].Close)
	m.DIF = append(m.DIF, dif)
	m.DEA = append(m.DEA, dea)
	m.Hist = append(m.Hist, hist)
	m.prev = prev
	m.run = st
}

// Revise recomputes the last bar from the state saved before it.
func (m *MACD) Revise(cs []Candle, _ Candle) {
	if m == nil || len(cs) == 0 || !m.tailLen(len(cs)) {
		if m != nil {
			m.Calc(cs)
		}
		return
	}
	st, dif, dea, hist := m.step(m.prev, cs[len(cs)-1].Close)
	last := len(cs) - 1
	m.DIF[last], m.DEA[last], m.Hist[last] = dif, dea, hist
	m.run = st
}

// YDomain is the padded range of DIF, DEA, and the histogram in
// first..last, widened so zero stays on the scale.
func (m *MACD) YDomain(first, last int) (lo, hi float64) {
	lo, hi = 0, 0
	if m == nil {
		return -1, 1
	}
	if first < 0 {
		first = 0
	}
	n := len(m.DIF)
	if last >= n {
		last = n - 1
	}
	any := false
	if first <= last {
		for i := first; i <= last; i++ {
			for _, series := range [][]float64{m.DIF, m.DEA, m.Hist} {
				if i >= len(series) {
					continue
				}
				v := series[i]
				if math.IsNaN(v) || math.IsInf(v, 0) {
					continue
				}
				any = true
				lo = math.Min(lo, v)
				hi = math.Max(hi, v)
			}
		}
	}
	if !any {
		return -1, 1
	}
	span := hi - lo
	if span < 1e-6 {
		span = math.Max(math.Abs(hi)*0.2, 1)
		lo -= span / 2
		hi += span / 2
		span = hi - lo
	}
	pad := span * 0.08
	return lo - pad, hi + pad
}

// Scale is the sub-pane Y scale for bars first..last.
func (m *MACD) Scale(first, last int) chart.Linear {
	lo, hi := m.YDomain(first, last)
	s := chart.NewLinear(lo, hi).Nice(4)
	if s.Hi <= s.Lo {
		s.Hi = s.Lo + 1
	}
	return s
}

func (m *MACD) DrawMain(*ui.Painter, ui.Rect, Viewport, chart.Linear) {}

// Values publishes MACD.DIF, MACD.DEA, and MACD.HIST. A warm-up gap is
// absent rather than NaN, so a host can tell "not ready" from a number.
func (m *MACD) Values(i int) map[string]float64 {
	if m == nil || i < 0 {
		return nil
	}
	put := func(out map[string]float64, key string, line []float64) map[string]float64 {
		if i >= len(line) {
			return out
		}
		v := line[i]
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return out
		}
		if out == nil {
			out = make(map[string]float64, 3)
		}
		out[key] = v
		return out
	}
	var out map[string]float64
	out = put(out, "MACD.DIF", m.DIF)
	out = put(out, "MACD.DEA", m.DEA)
	out = put(out, "MACD.HIST", m.Hist)
	return out
}

func (m *MACD) DrawSub(p *ui.Painter, rect ui.Rect, vp Viewport) {
	if m == nil || len(m.DIF) == 0 || rect.W < 2 || rect.H < 2 {
		return
	}
	first, last := vp.Visible(len(m.DIF), float64(rect.W))
	yScale := m.Scale(first, last)
	zero := chart.Y(rect, yScale.At(0))
	p.Line(rect.X, zero, rect.X+rect.W, zero, 1, m.Zero)

	if first > last {
		return
	}
	bodyW := float32(vp.BarWidth * bodyFrac)
	if bodyW < 1 {
		bodyW = 1
	}
	for i := first; i <= last && i < len(m.Hist); i++ {
		v := m.Hist[i]
		if math.IsNaN(v) {
			continue
		}
		color := m.Down
		if v >= 0 {
			color = m.Up
		}
		y := chart.Y(rect, yScale.At(v))
		top := min(y, zero)
		bot := max(y, zero)
		h := bot - top
		if h < 1 {
			h = 1
			top = zero - 0.5
		}
		cx := rect.X + float32(vp.BarLeft(i)+vp.BarWidth/2)
		p.Fill(ui.Rect{X: cx - bodyW/2, Y: top, W: bodyW, H: h}, color.Alpha(0.85), 0)
	}
	drawMA(p, rect, m.DIF, vp, yScale, first, last, m.DIFColor)
	drawMA(p, rect, m.DEA, vp, yScale, first, last, m.DEAColor)
}

func (m *MACD) ValueAt(i int) string {
	if m == nil || i < 0 || i >= len(m.DIF) || math.IsNaN(m.DIF[i]) {
		return ""
	}
	s := "DIF " + FormatPrice(m.DIF[i])
	if i < len(m.DEA) && !math.IsNaN(m.DEA[i]) {
		s += "  DEA " + FormatPrice(m.DEA[i])
	}
	if i < len(m.Hist) && !math.IsNaN(m.Hist[i]) {
		s += "  HIST " + FormatPrice(m.Hist[i])
	}
	return s
}

func nanSlice(n int) []float64 {
	if n <= 0 {
		return nil
	}
	s := make([]float64, n)
	for i := range s {
		s[i] = math.NaN()
	}
	return s
}
