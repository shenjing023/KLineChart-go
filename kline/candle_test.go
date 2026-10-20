package kline

import (
	"math"
	"testing"
	"time"
)

func TestGenerateOHLC(t *testing.T) {
	cs := Generate(600, 42)
	if len(cs) != 600 {
		t.Fatalf("len %d", len(cs))
	}
	if again := Generate(600, 42); again[0] != cs[0] || again[599] != cs[599] {
		t.Fatal("Generate is not deterministic for seed 42")
	}
	var up, down int
	for i, c := range cs {
		if c.High < math.Max(c.Open, c.Close) || c.Low > math.Min(c.Open, c.Close) {
			t.Fatalf("bar %d OHLC out of order: %+v", i, c)
		}
		if c.Low <= 0 || c.Volume <= 0 {
			t.Fatalf("bar %d non-positive low or volume: %+v", i, c)
		}
		if i > 0 && !c.Time.After(cs[i-1].Time) {
			t.Fatalf("bar %d time %s does not follow %s", i, c.Time, cs[i-1].Time)
		}
		if c.Close >= c.Open {
			up++
		} else {
			down++
		}
	}
	if up == 0 || down == 0 {
		t.Fatalf("expected both up and down bars, up=%d down=%d", up, down)
	}
	if Generate(0, 1) != nil || Generate(-3, 1) != nil {
		t.Fatal("non-positive count should return nil")
	}
}

func TestSMA(t *testing.T) {
	cs := []Candle{{Close: 1}, {Close: 2}, {Close: 3}, {Close: 4}, {Close: 5}}
	got := SMA(cs, 3)
	if !math.IsNaN(got[0]) || !math.IsNaN(got[1]) {
		t.Fatalf("warmup should be NaN: %v", got[:2])
	}
	want := []float64{2, 3, 4}
	for i, w := range want {
		if math.Abs(got[i+2]-w) > 1e-9 {
			t.Fatalf("sma[%d] = %v, want %v", i+2, got[i+2], w)
		}
	}
	if SMA(cs, 0) != nil {
		t.Fatal("period 0 should return nil")
	}
}

func TestFormatBar(t *testing.T) {
	c := Candle{
		Time: time.Date(2024, 3, 5, 14, 30, 0, 0, time.UTC),
		Open: 10, High: 12, Low: 9, Close: 11, Volume: 1500.4,
	}
	s := FormatBar(c, 10.5, math.NaN())
	if want := "2024-03-05 14:30  O 10.00  H 12.00  L 9.00  C 11.00  V 1500  MA5 10.50"; s != want {
		t.Fatalf("format %q", s)
	}
	loc := time.FixedZone("UTC+8", 8*3600)
	got := FormatBarIn(c, 10.5, math.NaN(), 4, loc)
	want := "2024-03-05 22:30  O 10.0000  H 12.0000  L 9.0000  C 11.0000  V 1500  MA5 10.5000"
	if got != want {
		t.Fatalf("FormatBarIn %q", got)
	}
	if FormatPriceDigits(1.2, 0) != "1" {
		t.Fatalf("zero digits %q", FormatPriceDigits(1.2, 0))
	}
	if FormatPriceDigits(math.NaN(), 4) != "—" {
		t.Fatal("NaN")
	}
}
