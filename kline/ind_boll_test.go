package kline

import (
	"math"
	"testing"
)

// bollCloses is 21 closes. Index 19 is the first full BOLL(20, 2) window
// of 1..20. The next bar, 21, slides the window to 2..21.
func bollCloses() []Candle {
	cs := make([]Candle, 21)
	for i := range cs {
		c := float64(i + 1)
		cs[i] = Candle{Open: c, High: c, Low: c, Close: c}
	}
	return cs
}

func TestBOLLMatchesSMAAndIsSymmetric(t *testing.T) {
	cs := bollCloses()
	b := NewBOLL(20, 2)
	if b.Name() != "BOLL" || b.PaneKind() != PaneMain {
		t.Fatalf("name %q kind %q", b.Name(), b.PaneKind())
	}
	b.Calc(cs)
	if len(b.Mid) != len(cs) || len(b.Up) != len(cs) || len(b.Low) != len(cs) {
		t.Fatalf("lengths mid %d up %d low %d", len(b.Mid), len(b.Up), len(b.Low))
	}
	sma := SMA(cs, 20)
	for i := range cs {
		if math.IsNaN(sma[i]) != math.IsNaN(b.Mid[i]) || (!math.IsNaN(sma[i]) && math.Abs(b.Mid[i]-sma[i]) > 1e-9) {
			t.Fatalf("MID[%d] = %v, SMA20 %v", i, b.Mid[i], sma[i])
		}
		if i < 19 {
			if !math.IsNaN(b.Up[i]) || !math.IsNaN(b.Low[i]) {
				t.Fatalf("index %d should still be warming up: up %v low %v", i, b.Up[i], b.Low[i])
			}
			continue
		}
		// Population standard deviation: mean of squared deviations, then sqrt.
		// KLineChart divides by the window length, not N-1.
		var sum float64
		for j := i - 19; j <= i; j++ {
			d := cs[j].Close - b.Mid[i]
			sum += d * d
		}
		std := math.Sqrt(sum / 20)
		assertClose(t, "UP", b.Up[i], b.Mid[i]+2*std)
		assertClose(t, "LOW", b.Low[i], b.Mid[i]-2*std)
		if math.Abs((b.Up[i]-b.Mid[i])-(b.Mid[i]-b.Low[i])) > 1e-9 {
			t.Fatalf("bands not symmetric at %d: up %v mid %v low %v", i, b.Up[i], b.Mid[i], b.Low[i])
		}
	}
	// Window 1..20. Mid is 10.5. Population std is sqrt(33.25).
	assertClose(t, "MID[19]", b.Mid[19], 10.5)
	assertClose(t, "UP[19]", b.Up[19], 22.032562594670797)
	assertClose(t, "LOW[19]", b.Low[19], -1.0325625946707966)
	assertClose(t, "MID[20]", b.Mid[20], 11.5)
	header := b.Series()
	if len(header) != 3 || &header[0][0] != &b.Mid[0] || &header[1][0] != &b.Up[0] || &header[2][0] != &b.Low[0] {
		t.Fatal("Series should be the three lines from the last Calc")
	}
	again := b.Series()
	if &header[0] != &again[0] {
		t.Fatal("Series allocated a new header")
	}
	b.Calc(cs)
	if &header[0] != &b.Series()[0] {
		t.Fatal("Calc replaced the Series header")
	}

	if b.ValueAt(18) != "" || b.ValueAt(-1) != "" || b.ValueAt(len(cs)) != "" {
		t.Fatal("ValueAt should be empty before the window fills and outside the series")
	}
	got := b.ValueAt(19)
	want := "MID " + FormatPrice(b.Mid[19]) + "  UP " + FormatPrice(b.Up[19]) + "  LOW " + FormatPrice(b.Low[19])
	if got != want {
		t.Fatalf("ValueAt %q, want %q", got, want)
	}
}

func TestBOLLWidensMainScale(t *testing.T) {
	// The visible bars are flat at 50. Their BOLL window still contains
	// the earlier 0/100 alternation, so the bands sit well outside the
	// candles on screen. The price scale has to include those bands.
	cs := make([]Candle, 25)
	for i := range cs {
		c := 50.0
		if i < 20 {
			c = 0
			if i%2 == 1 {
				c = 100
			}
		}
		cs[i] = Candle{Open: c, High: c, Low: c, Close: c}
	}
	b := NewBOLL(20, 2)
	b.Calc(cs)
	vp := Viewport{Offset: 20, BarWidth: 10}
	plain := MainScale(cs, nil, vp, 50)
	with := MainScale(cs, []Indicator{b}, vp, 50)
	if with.Lo >= plain.Lo {
		t.Fatalf("lower band should pull the price scale down: with %v plain %v", with.Lo, plain.Lo)
	}
	if with.Hi <= plain.Hi {
		t.Fatalf("upper band should lift the price scale: with %v plain %v", with.Hi, plain.Hi)
	}
	indexed, _, _ := CompareScales(cs, nil, []Indicator{b}, vp, 50)
	plainIndex, _, _ := CompareScales(cs, nil, nil, vp, 50)
	if indexed.Lo >= plainIndex.Lo || indexed.Hi <= plainIndex.Hi {
		t.Fatalf("overlay scale should include BOLL: with %v..%v plain %v..%v", indexed.Lo, indexed.Hi, plainIndex.Lo, plainIndex.Hi)
	}
}

func TestRegistryListsBOLLAndKDJ(t *testing.T) {
	got := map[string]string{}
	for _, spec := range Registry() {
		ind := spec.New()
		got[spec.Name] = spec.Kind
		if ind.Name() != spec.Name || ind.PaneKind() != spec.Kind {
			t.Fatalf("spec %+v built %s/%s", spec, ind.Name(), ind.PaneKind())
		}
	}
	if got["BOLL"] != PaneMain || got["KDJ"] != PaneSub {
		t.Fatalf("registry %v", got)
	}
	boll := gotSpec(t, "BOLL").New().(*BOLL)
	if boll.Period != 20 || boll.Mult != 2 {
		t.Fatalf("default BOLL %+v", boll)
	}
	kdj := gotSpec(t, "KDJ").New().(*KDJ)
	if kdj.N != 9 || kdj.M1 != 3 || kdj.M2 != 3 {
		t.Fatalf("default KDJ %+v", kdj)
	}
}

func gotSpec(t *testing.T, name string) Spec {
	t.Helper()
	for _, spec := range Registry() {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("missing spec %s", name)
	return Spec{}
}
