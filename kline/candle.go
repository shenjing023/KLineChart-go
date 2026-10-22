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

	"github.com/egoist/mygo/ui"
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
	line, _ := smaLine(cs, period)
	return line
}

// smaLine is SMA plus the rolling sum left after the last bar. That sum
// is the last `period` closes, or every close when the series is shorter,
// and an append updates it in constant time.
func smaLine(cs []Candle, period int) ([]float64, float64) {
	if period < 1 || len(cs) == 0 {
		return nil, 0
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
	return out, sum
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

// labelFmt is the chart's price precision, time zone, and the host's
// time formatter. A nil loc is UTC. A nil format keeps the built-in
// layout the caller passes to when.
type labelFmt struct {
	digits int
	loc    *time.Location
	format func(time.Time) string
	// tick formats a time per label role. Nil uses format for every
	// role.
	tick func(time.Time, TickKind) string
	// volume formats an axis tick in the volume strip. Nil keeps the
	// plain whole number a scale prints.
	volume func(float64) string
}

// TickKind is where a formatted time appears. A host that supplies
// ChartOptions.FormatTick uses it to pick a granularity: the axis can
// carry a coarser label than the crosshair without the library knowing
// why.
type TickKind string

const (
	// TickAxis is a time-axis tick under the plot. Month ticks land on
	// the first of the month, so a day mask would print the same string
	// for every one of them.
	TickAxis TickKind = "axis"
	// TickCrosshair is the time tag on the crosshair.
	TickCrosshair TickKind = "crosshair"
	// TickReadout is the time inside Readout and the header.
	TickReadout TickKind = "readout"
)

func defaultLabels() labelFmt { return labelFmt{digits: 2} }

func (f labelFmt) price(v float64) string { return formatPrice(v, f.digits) }

// when formats t for display. A host formatter wins: it sees the time in
// the chart's zone and replaces the built-in layout, which is what a
// blind date mask needs. The layout is only a fallback, so a host that
// wants "2006-01-02" keeps that choice in one place rather than in the
// library.
func (f labelFmt) when(t time.Time, layout string) string {
	if f.format != nil {
		return f.format(f.inZone(t))
	}
	return formatWhen(t, layout, f.loc)
}

// at is when for one role. A host that set FormatTick answers per role;
// otherwise every role uses the one formatter.
func (f labelFmt) at(t time.Time, kind TickKind, layout string) string {
	if f.tick != nil {
		return f.tick(f.inZone(t), kind)
	}
	return f.when(t, layout)
}

// inZone is t in the display zone. All formatting paths go through it,
// so a host formatter and the built-in layout name the same instant.
func (f labelFmt) inZone(t time.Time) time.Time {
	if f.loc == nil || f.loc == time.UTC {
		return t.UTC()
	}
	return t.In(f.loc)
}

func formatWhen(t time.Time, layout string, loc *time.Location) string {
	if loc == nil || loc == time.UTC {
		return t.UTC().Format(layout)
	}
	return t.In(loc).Format(layout)
}

// TextBaseline is the distance from the y a text line is drawn at to its
// baseline. MeasureText returns the line box, which is the same height at
// every size (the font's line height), while the glyphs scale with the
// requested size and end on the baseline. So centring a short label by
// subtracting MeasureText's height leaves it low, and by a different
// amount at each size.
const TextBaseline float32 = 14

// CenterText draws span with its glyphs centred in box. Use it for a
// label inside a pill or tag instead of subtracting MeasureText's height:
// that height is the line box, not the ink, so the usual
// "(box.H-h)/2" sits low and the error changes with the size.
//
// It paints; it does not reserve layout. Call it from a Draw callback.
func CenterText(p *ui.Painter, box ui.Rect, span ui.Span) {
	if p == nil || span.Text == "" {
		return
	}
	w, _ := p.MeasureText(0, span)
	x := box.X + (box.W-w)/2
	// Put the baseline at the box's middle plus half the cap height, so
	// the visible glyphs straddle the centre. Cap height is roughly 0.72
	// of the point size for the font MyGo ships.
	capH := span.Size * 0.72
	y := box.Y + box.H/2 + capH/2 - TextBaseline
	p.RichText(x, y, 0, span)
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
		labels.at(c.Time, TickReadout, "2006-01-02 15:04"),
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
