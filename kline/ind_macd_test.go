package kline

import (
	"math"
	"testing"
)

// macdCloses is a short walk long enough for MACD(12,26,9). The expected
// DIF/DEA/histogram values are the KLineChart calculation: EMA seeds are
// the SMA of the first period closes, DEA is the EMA of DIF with the same
// seed rule, and the histogram is 2*(DIF-DEA). DIF starts at index 25.
// DEA and the histogram start at index 33.
func macdCloses() []Candle {
	closes := []float64{
		10, 11, 12, 11, 13, 14, 13, 15, 16, 15,
		17, 18, 17, 19, 20, 19, 21, 22, 21, 23,
		24, 23, 25, 26, 25, 27, 28, 27, 29, 30,
		29, 31, 32, 31, 33, 34, 33, 35, 36, 35,
	}
	cs := make([]Candle, len(closes))
	for i, c := range closes {
		cs[i] = Candle{Open: c, High: c, Low: c, Close: c}
	}
	return cs
}

func TestMACDFixture(t *testing.T) {
	cs := macdCloses()
	m := NewMACD(12, 26, 9)
	if m.Name() != "MACD" || m.PaneKind() != PaneSub {
		t.Fatalf("name %q kind %q", m.Name(), m.PaneKind())
	}
	m.Calc(cs)
	if len(m.DIF) != len(cs) || len(m.DEA) != len(cs) || len(m.Hist) != len(cs) {
		t.Fatalf("lengths dif %d dea %d hist %d", len(m.DIF), len(m.DEA), len(m.Hist))
	}
	for i := 0; i < 25; i++ {
		if !math.IsNaN(m.DIF[i]) || !math.IsNaN(m.DEA[i]) || !math.IsNaN(m.Hist[i]) {
			t.Fatalf("index %d should be warming up: dif %v dea %v hist %v", i, m.DIF[i], m.DEA[i], m.Hist[i])
		}
	}
	if math.IsNaN(m.DIF[25]) || !math.IsNaN(m.DEA[25]) || !math.IsNaN(m.Hist[25]) {
		t.Fatalf("first DIF at 25, DEA still empty: dif %v dea %v hist %v", m.DIF[25], m.DEA[25], m.Hist[25])
	}
	assertClose(t, "DIF[25]", m.DIF[25], 4.639265428331843)
	assertClose(t, "DIF[33]", m.DIF[33], 4.613313080331249)
	assertClose(t, "DEA[33]", m.DEA[33], 4.650984501812228)
	assertClose(t, "HIST[33]", m.Hist[33], -0.07534284296195715)
	assertClose(t, "DIF[39]", m.DIF[39], 4.617505096246344)
	assertClose(t, "DEA[39]", m.DEA[39], 4.654279424290604)
	assertClose(t, "HIST[39]", m.Hist[39], -0.07354865608852101)
	for i := 33; i < len(cs); i++ {
		want := 2 * (m.DIF[i] - m.DEA[i])
		if math.Abs(m.Hist[i]-want) > 1e-12 {
			t.Fatalf("HIST[%d] = %v, want 2*(DIF-DEA) %v", i, m.Hist[i], want)
		}
	}
	if m.ValueAt(24) != "" {
		t.Fatalf("ValueAt before DIF: %q", m.ValueAt(24))
	}
	if got, want := m.ValueAt(25), "DIF "+FormatPrice(m.DIF[25]); got != want {
		t.Fatalf("ValueAt DIF only %q, want %q", got, want)
	}
	got := m.ValueAt(39)
	want := "DIF " + FormatPrice(m.DIF[39]) + "  DEA " + FormatPrice(m.DEA[39]) + "  HIST " + FormatPrice(m.Hist[39])
	if got != want {
		t.Fatalf("ValueAt %q, want %q", got, want)
	}
	lo, hi := m.YDomain(0, len(cs)-1)
	if lo >= 0 || hi <= 0 || lo >= hi {
		t.Fatalf("MACD domain should span the histogram around zero: %v %v", lo, hi)
	}
}

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %.15f, want %.15f", name, got, want)
	}
}
