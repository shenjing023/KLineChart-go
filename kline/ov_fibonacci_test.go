package kline

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui/chart"
)

func TestFibLevelPrices(t *testing.T) {
	p0, p1 := 100.0, 50.0
	want := map[float64]float64{
		0:     50,
		0.236: 61.8,
		0.382: 69.1,
		0.5:   75,
		0.618: 80.9,
		0.786: 89.3,
		1:     100,
	}
	if len(FibRatios) != len(want) {
		t.Fatalf("FibRatios len %d", len(FibRatios))
	}
	for i, r := range FibRatios {
		got := FibLevelPrice(p0, p1, r)
		if math.Abs(got-want[r]) > 1e-9 {
			t.Fatalf("ratio[%d]=%v price %v want %v", i, r, got, want[r])
		}
	}
	// 40 then 80 is an upward draw. Anchors stay in click order.
	// Ratio 0 is the second anchor and ratio 1 is the first.
	if got := FibLevelPrice(40, 80, 0); math.Abs(got-80) > 1e-9 {
		t.Fatalf("upward ratio 0 = %v, want second anchor 80", got)
	}
	if got := FibLevelPrice(40, 80, 1); math.Abs(got-40) > 1e-9 {
		t.Fatalf("upward ratio 1 = %v, want first anchor 40", got)
	}
	got := FibLevelPrice(40, 80, 0.5)
	if math.Abs(got-60) > 1e-9 {
		t.Fatalf("upward mid %v", got)
	}
}

func TestFibonacciHitEndpointAndLevel(t *testing.T) {
	scale := chart.NewLinear(0, 100)
	const plotH = 200.0
	vp := Viewport{Offset: 0, BarWidth: 10}
	fib := Overlay{ID: "f", Kind: OverlayFibonacci, Points: []Point{
		{BarIndex: 2, Price: 80},
		{BarIndex: 8, Price: 20},
	}}
	x0 := AnchorX(fib.Points[0], vp, nil)
	y0 := PriceToLocalY(plotH, 80, scale)
	x1 := AnchorX(fib.Points[1], vp, nil)
	y1 := PriceToLocalY(plotH, 20, scale)
	mid := FibLevelPrice(80, 20, 0.5)
	yMid := PriceToLocalY(plotH, mid, scale)
	xLeft, xRight := math.Min(x0, x1), math.Max(x0, x1)

	part, ok := HitOverlay(fib, x0, y0, 400, plotH, vp, scale, nil)
	if !ok || part != 0 {
		t.Fatalf("first endpoint: part %d ok %v", part, ok)
	}
	part, ok = HitOverlay(fib, x1+HitRadius-0.5, y1, 400, plotH, vp, scale, nil)
	if !ok || part != 1 {
		t.Fatalf("second endpoint: part %d ok %v", part, ok)
	}
	part, ok = HitOverlay(fib, (xLeft+xRight)/2, yMid, 400, plotH, vp, scale, nil)
	if !ok || part != HitBody {
		t.Fatalf("0.5 level: part %d ok %v", part, ok)
	}
	if _, ok := HitOverlay(fib, xLeft-HitRadius-2, yMid, 400, plotH, vp, scale, nil); ok {
		t.Fatal("past the left end of the span should miss the level")
	}
	if _, ok := HitOverlay(fib, xRight+HitRadius+2, yMid, 400, plotH, vp, scale, nil); ok {
		t.Fatal("past the right end of the span should miss the level")
	}
	if _, ok := HitOverlay(fib, (xLeft+xRight)/2, yMid+120, 400, plotH, vp, scale, nil); ok {
		t.Fatal("far from every level")
	}
}

func TestFibonacciDegenerateX(t *testing.T) {
	scale := chart.NewLinear(0, 100)
	const plotH = 200.0
	vp := Viewport{Offset: 0, BarWidth: 10}
	fib := Overlay{ID: "f", Kind: OverlayFibonacci, Points: []Point{
		{BarIndex: 4, Price: 70},
		{BarIndex: 4, Price: 30},
	}}
	x := AnchorX(fib.Points[0], vp, nil)
	yMid := PriceToLocalY(plotH, FibLevelPrice(70, 30, 0.5), scale)
	part, ok := HitOverlay(fib, x, yMid, 400, plotH, vp, scale, nil)
	if !ok || part != HitBody {
		t.Fatalf("degenerate span at level: part %d ok %v", part, ok)
	}
}
