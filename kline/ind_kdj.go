package kline

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func init() {
	mustRegister(Spec{
		Name: "KDJ",
		Kind: PaneSub,
		New:  func() Indicator { return NewKDJ(9, 3, 3) },
	})
}

// KDJ is the stochastic oscillator, drawn in its own sub-pane.
// N, M1, and M2 are the bar counts, usually 9, 3, and 3.
//
// RSV is (Close − lowest low) / (highest high − lowest low) × 100 over
// the last N bars. A flat window (high == low) uses 1 as the divisor,
// which is the KLineChart rule. K and D then follow
//
//	K = ((M1−1)×prevK + RSV) / M1
//	D = ((M2−1)×prevD + K) / M2
//	J = 3K − 2D
//
// The first K and D use 50 where there is no previous value. For the
// usual (3, 3) that is (2×prev + today) / 3. K, D, and J start together
// at bar N−1.
type KDJ struct {
	N, M1, M2              int
	K, D, J                []float64
	KColor, DColor, JColor ui.Color
	// pk and pd are K and D after the last bar. pre* is that pair before
	// the last bar, so revising the bar does not walk the earlier ones.
	pk, pd     float64
	have       bool
	preK, preD float64
	preHave    bool
}

// NewKDJ returns a KDJ with the given periods. Draw colors start as
// DarkDefault; Paint replaces them from the active Style before drawing.
func NewKDJ(n, m1, m2 int) *KDJ {
	s := DarkDefault()
	return &KDJ{
		N: n, M1: m1, M2: m2,
		KColor: s.KDJK,
		DColor: s.KDJD,
		JColor: s.KDJJ,
	}
}

func (k *KDJ) Name() string     { return "KDJ" }
func (k *KDJ) PaneKind() string { return PaneSub }

// SetColors replaces the K, D, and J lines.
func (k *KDJ) SetColors(kc, dc, jc ui.Color) {
	if k == nil {
		return
	}
	k.KColor, k.DColor, k.JColor = kc, dc, jc
}

func (k *KDJ) Calc(cs []Candle) {
	n := len(cs)
	k.K = nanSlice(n)
	k.D = nanSlice(n)
	k.J = nanSlice(n)
	k.pk, k.pd, k.have = 0, 0, false
	k.preK, k.preD, k.preHave = 0, 0, false
	period, m1, m2 := k.N, k.M1, k.M2
	if n == 0 || period < 1 || m1 < 1 || m2 < 1 {
		return
	}
	var prevK, prevD float64
	have := false
	for i := period - 1; i < n; i++ {
		if i == n-1 {
			k.preK, k.preD, k.preHave = prevK, prevD, have
		}
		kv, dv, jv, ok := k.point(cs, i, prevK, prevD, have)
		if !ok {
			continue
		}
		k.K[i], k.D[i], k.J[i] = kv, dv, jv
		prevK, prevD = kv, dv
		have = true
	}
	k.pk, k.pd, k.have = prevK, prevD, have
}

// point is K, D, and J at i. The window is the last N bars. A flat
// window uses 1 as the divisor. The first value uses 50 when have is
// false.
func (k *KDJ) point(cs []Candle, i int, pk, pd float64, have bool) (kv, dv, jv float64, ok bool) {
	period, m1, m2 := k.N, k.M1, k.M2
	if i < 0 || period < 1 || m1 < 1 || m2 < 1 || i < period-1 || i >= len(cs) {
		return math.NaN(), math.NaN(), math.NaN(), false
	}
	hn := cs[i].High
	ln := cs[i].Low
	for j := i - (period - 1); j <= i; j++ {
		if cs[j].High > hn {
			hn = cs[j].High
		}
		if cs[j].Low < ln {
			ln = cs[j].Low
		}
	}
	den := hn - ln
	if den == 0 {
		den = 1
	}
	rsv := (cs[i].Close - ln) / den * 100
	if !have {
		pk, pd = 50, 50
	}
	kv = (float64(m1-1)*pk + rsv) / float64(m1)
	dv = (float64(m2-1)*pd + kv) / float64(m2)
	jv = 3*kv - 2*dv
	return kv, dv, jv, true
}

// Append extends K, D, and J by the bar now at the end of cs.
func (k *KDJ) Append(cs []Candle) {
	if k == nil || len(cs) == 0 {
		return
	}
	n := len(cs)
	if len(k.K) != n-1 || len(k.D) != n-1 || len(k.J) != n-1 {
		k.Calc(cs)
		return
	}
	k.preK, k.preD, k.preHave = k.pk, k.pd, k.have
	kv, dv, jv, ok := k.point(cs, n-1, k.pk, k.pd, k.have)
	k.K = append(k.K, kv)
	k.D = append(k.D, dv)
	k.J = append(k.J, jv)
	if ok {
		k.pk, k.pd, k.have = kv, dv, true
	}
}

// Revise replaces the last K, D, and J from the state saved before that bar.
func (k *KDJ) Revise(cs []Candle, _ Candle) {
	if k == nil || len(cs) == 0 {
		return
	}
	n := len(cs)
	if len(k.K) != n || len(k.D) != n || len(k.J) != n {
		k.Calc(cs)
		return
	}
	kv, dv, jv, ok := k.point(cs, n-1, k.preK, k.preD, k.preHave)
	last := n - 1
	k.K[last], k.D[last], k.J[last] = kv, dv, jv
	if ok {
		k.pk, k.pd, k.have = kv, dv, true
		return
	}
	k.pk, k.pd, k.have = k.preK, k.preD, k.preHave
}

// YDomain is the padded range of K, D, and J in first..last. The scale
// always covers 0–100, then widens when a value falls outside that band.
func (k *KDJ) YDomain(first, last int) (lo, hi float64) {
	lo, hi = 0, 100
	if k == nil {
		return lo, hi
	}
	if first < 0 {
		first = 0
	}
	n := len(k.K)
	if last >= n {
		last = n - 1
	}
	if first <= last {
		for i := first; i <= last; i++ {
			for _, series := range [][]float64{k.K, k.D, k.J} {
				if i >= len(series) {
					continue
				}
				v := series[i]
				if math.IsNaN(v) || math.IsInf(v, 0) {
					continue
				}
				lo = math.Min(lo, v)
				hi = math.Max(hi, v)
			}
		}
	}
	span := hi - lo
	if span < 1e-6 {
		return 0, 100
	}
	pad := span * 0.08
	return lo - pad, hi + pad
}

// Scale is the sub-pane Y scale for bars first..last.
func (k *KDJ) Scale(first, last int) chart.Linear {
	lo, hi := k.YDomain(first, last)
	s := chart.NewLinear(lo, hi).Nice(4)
	if s.Hi <= s.Lo {
		s.Hi = s.Lo + 1
	}
	return s
}

func (k *KDJ) DrawMain(*ui.Painter, ui.Rect, Viewport, chart.Linear) {}

func (k *KDJ) DrawSub(p *ui.Painter, rect ui.Rect, vp Viewport) {
	if k == nil || len(k.K) == 0 || rect.W < 2 || rect.H < 2 {
		return
	}
	first, last := vp.Visible(len(k.K), float64(rect.W))
	yScale := k.Scale(first, last)
	drawMA(p, rect, k.K, vp, yScale, first, last, k.KColor)
	drawMA(p, rect, k.D, vp, yScale, first, last, k.DColor)
	drawMA(p, rect, k.J, vp, yScale, first, last, k.JColor)
}

func (k *KDJ) ValueAt(i int) string {
	if k == nil || i < 0 || i >= len(k.K) || math.IsNaN(k.K[i]) {
		return ""
	}
	s := "K " + FormatPrice(k.K[i])
	if i < len(k.D) && !math.IsNaN(k.D[i]) {
		s += "  D " + FormatPrice(k.D[i])
	}
	if i < len(k.J) && !math.IsNaN(k.J[i]) {
		s += "  J " + FormatPrice(k.J[i])
	}
	return s
}
