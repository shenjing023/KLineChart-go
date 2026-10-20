package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func init() {
	Register(Spec{
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
	short, long, signal := m.Short, m.Long, m.Signal
	if n == 0 || short < 1 || long < 1 || signal < 1 || short == long {
		return
	}
	var closeSum float64
	var emaShort, emaLong float64
	var dif, difSum, dea float64
	haveShort, haveLong := false, false
	maxPeriod := long
	if short > maxPeriod {
		maxPeriod = short
	}
	for i := 0; i < n; i++ {
		close := cs[i].Close
		closeSum += close
		if i >= short-1 {
			if i > short-1 {
				emaShort = (2*close + float64(short-1)*emaShort) / float64(short+1)
			} else {
				emaShort = closeSum / float64(short)
			}
			haveShort = true
		}
		if i >= long-1 {
			if i > long-1 {
				emaLong = (2*close + float64(long-1)*emaLong) / float64(long+1)
			} else {
				emaLong = closeSum / float64(long)
			}
			haveLong = true
		}
		if i >= maxPeriod-1 && haveShort && haveLong {
			dif = emaShort - emaLong
			m.DIF[i] = dif
			difSum += dif
			if i >= maxPeriod+signal-2 {
				if i > maxPeriod+signal-2 {
					dea = (dif*2 + dea*float64(signal-1)) / float64(signal+1)
				} else {
					dea = difSum / float64(signal)
				}
				m.DEA[i] = dea
				m.Hist[i] = (dif - dea) * 2
			}
		}
	}
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
