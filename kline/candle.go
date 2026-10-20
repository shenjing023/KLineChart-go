// Package kline draws a minimal candlestick chart: OHLC bodies, a
// built-in volume strip, indicators on the price plot and in sub-panes,
// and the viewport math for pan and zoom. MyGo's ui/chart package has
// scales and axes, not candles and not pan/zoom, so this package owns
// that geometry.
package kline

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"time"
)

// Candle is one OHLC bar. Time is the bar's open time. Prices and Volume
// are unitless sample values, not a quote from a market.
type Candle struct {
	Time                   time.Time
	Open, High, Low, Close float64
	Volume                 float64
}

// Generate builds n regular bars with a deterministic random walk.
// seed fixes the series, so the demo needs no network and tests can
// replay it. Bars are one hour apart, starting 2024-01-02 09:30 UTC.
// n <= 0 returns nil.
func Generate(n int, seed int64) []Candle {
	if n <= 0 {
		return nil
	}
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15))
	start := time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)
	price := 100.0
	out := make([]Candle, n)
	for i := range out {
		// A slight upward drift so the series is not a flat line, with
		// enough noise that both up and down bars appear.
		change := (rng.Float64() - 0.48) * 1.6
		open := price
		close := open + change
		high := math.Max(open, close) + rng.Float64()*0.9
		low := math.Min(open, close) - rng.Float64()*0.9
		if low < 1 {
			shift := 1 - low
			low += shift
			open += shift
			close += shift
			high += shift
		}
		span := math.Abs(close - open)
		volume := 600 + rng.Float64()*4200 + span*1800
		out[i] = Candle{
			Time:   start.Add(time.Duration(i) * time.Hour),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: volume,
		}
		price = close
	}
	return out
}

// SMA is the simple moving average of Close over period bars.
// Values before the window is full are NaN. period < 1 returns nil.
func SMA(cs []Candle, period int) []float64 {
	if period < 1 || len(cs) == 0 {
		return nil
	}
	out := make([]float64, len(cs))
	var sum float64
	for i := range cs {
		sum += cs[i].Close
		if i >= period {
			sum -= cs[i-period].Close
		}
		if i < period-1 {
			out[i] = math.NaN()
			continue
		}
		out[i] = sum / float64(period)
	}
	return out
}

// FormatPrice renders a price with two decimals.
func FormatPrice(v float64) string { return formatPrice(v, 2) }

// FormatPriceDigits renders a price with digits after the decimal.
// Negative digits use two, the same as FormatPrice. Zero digits is a
// whole number.
func FormatPriceDigits(v float64, digits int) string { return formatPrice(v, digits) }

func formatPrice(v float64, digits int) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	if digits < 0 {
		digits = 2
	}
	return strconv.FormatFloat(v, 'f', digits, 64)
}

// labelFmt is the chart's price precision and time zone. A nil loc is UTC.
type labelFmt struct {
	digits int
	loc    *time.Location
}

func defaultLabels() labelFmt { return labelFmt{digits: 2} }

func (f labelFmt) price(v float64) string { return formatPrice(v, f.digits) }

func (f labelFmt) when(t time.Time, layout string) string { return formatWhen(t, layout, f.loc) }

func formatWhen(t time.Time, layout string, loc *time.Location) string {
	if loc == nil || loc == time.UTC {
		return t.UTC().Format(layout)
	}
	return t.In(loc).Format(layout)
}

// FormatVolume renders a volume as a whole number.
func FormatVolume(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "—"
	}
	return strconv.FormatFloat(math.Round(v), 'f', 0, 64)
}

// FormatBar is the HUD line for one bar: time, OHLCV, and any finite
// moving averages. Times are UTC and prices use two decimals.
// FormatBarIn takes the precision and location ChartOptions uses.
func FormatBar(c Candle, ma5, ma20 float64) string {
	return formatBar(c, ma5, ma20, defaultLabels())
}

// FormatBarIn is FormatBar with an explicit price precision and time
// zone. Negative digits use two. A nil loc is UTC.
func FormatBarIn(c Candle, ma5, ma20 float64, digits int, loc *time.Location) string {
	if digits < 0 {
		digits = 2
	}
	return formatBar(c, ma5, ma20, labelFmt{digits: digits, loc: loc})
}

func formatBar(c Candle, ma5, ma20 float64, labels labelFmt) string {
	s := fmt.Sprintf("%s  O %s  H %s  L %s  C %s  V %s",
		labels.when(c.Time, "2006-01-02 15:04"),
		labels.price(c.Open), labels.price(c.High), labels.price(c.Low), labels.price(c.Close),
		FormatVolume(c.Volume))
	if !math.IsNaN(ma5) {
		s += "  MA5 " + labels.price(ma5)
	}
	if !math.IsNaN(ma20) {
		s += "  MA20 " + labels.price(ma20)
	}
	return s
}
