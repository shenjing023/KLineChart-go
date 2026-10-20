package kline

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/chart"
)

func barOHLC(o, h, l, c float64) Candle {
	return Candle{Time: time.Unix(0, 0), Open: o, High: h, Low: l, Close: c}
}

func TestSnapPriceOff(t *testing.T) {
	b := barOHLC(100, 110, 90, 105)
	scale := chart.NewLinear(80, 120)
	const plotH = 200.0
	for _, price := range []float64{100, 105, 97.5} {
		got := SnapPrice(MagnetOff, b, price, plotH, scale)
		if got != price {
			t.Fatalf("off mode: price %v got %v", price, got)
		}
	}
}

func TestSnapPriceStrongNearestAndTie(t *testing.T) {
	scale := chart.NewLinear(0, 1000)
	const plotH = 400.0

	b := barOHLC(90, 110, 80, 100)
	got := SnapPrice(MagnetStrong, b, 95, plotH, scale)
	if got != 90 {
		t.Fatalf("nearest to 95 is open 90, got %v", got)
	}

	// Equal distance: Open and High both one unit from 101; Open wins.
	tie := barOHLC(100, 102, 50, 60)
	got = SnapPrice(MagnetStrong, tie, 101, plotH, scale)
	if got != 100 {
		t.Fatalf("tie should break to open, got %v", got)
	}

	// Far from every OHLC still snaps in strong mode.
	got = SnapPrice(MagnetStrong, b, 200, plotH, scale)
	if got != 110 {
		t.Fatalf("strong should snap to high 110, got %v", got)
	}
}

func TestSnapPriceWeakWithinRadius(t *testing.T) {
	scale := chart.NewLinear(0, 1000)
	const plotH = 1000.0
	b := barOHLC(10, 200, 5, 180)
	price := 203.0
	got := SnapPrice(MagnetWeak, b, price, plotH, scale)
	if got != 200 {
		t.Fatalf("weak within radius should snap to high 200, got %v", got)
	}
	yPtr := PriceToLocalY(plotH, price, scale)
	yHigh := PriceToLocalY(plotH, 200, scale)
	if math.Abs(yPtr-yHigh) > HitRadius+0.01 {
		t.Fatalf("test setup: expected within hit radius, y diff %v", math.Abs(yPtr-yHigh))
	}
}

func TestSnapPriceWeakOutsideRadius(t *testing.T) {
	scale := chart.NewLinear(0, 1000)
	const plotH = 1000.0
	b := barOHLC(100, 200, 50, 300)
	price := 250.0
	got := SnapPrice(MagnetWeak, b, price, plotH, scale)
	if got != price {
		t.Fatalf("weak outside radius should keep pointer price %v, got %v", price, got)
	}
	nearest := 200.0
	yPtr := PriceToLocalY(plotH, price, scale)
	yNear := PriceToLocalY(plotH, nearest, scale)
	if math.Abs(yPtr-yNear) <= HitRadius {
		t.Fatalf("test setup: expected outside hit radius, y diff %v", math.Abs(yPtr-yNear))
	}
}

func TestSnapPriceWeakNoPlotHeight(t *testing.T) {
	b := barOHLC(10, 20, 5, 15)
	scale := chart.NewLinear(0, 100)
	price := 12.0
	if got := SnapPrice(MagnetWeak, b, price, 0, scale); got != price {
		t.Fatalf("weak with plotH 0 should return price, got %v", got)
	}
	if got := SnapPrice(MagnetWeak, b, price, -1, scale); got != price {
		t.Fatalf("weak with negative plotH should return price, got %v", got)
	}
	if got := SnapPrice(MagnetStrong, b, price, 0, scale); got != 10 {
		t.Fatalf("strong with plotH 0 should still snap to open, got %v", got)
	}
}

func TestSnapPriceSkipsNaN(t *testing.T) {
	b := barOHLC(math.NaN(), 50, 40, math.NaN())
	scale := chart.NewLinear(0, 100)
	got := SnapPrice(MagnetStrong, b, 45, 200, scale)
	if got != 50 {
		t.Fatalf("should skip NaN and snap to high 50 (tie over low), got %v", got)
	}
}
