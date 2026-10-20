package kline

import (
	"math"

	"github.com/egoist/mygo/ui/chart"
)

// MagnetMode controls whether new overlay points snap to the bar's OHLC.
type MagnetMode int

const (
	MagnetOff MagnetMode = iota
	MagnetWeak
	MagnetStrong
)

// knownMagnet returns mode when it is off, weak, or strong, and
// MagnetOff for any other value. NewChart uses it so an out-of-range
// option does not snap.
func knownMagnet(mode MagnetMode) MagnetMode {
	switch mode {
	case MagnetOff, MagnetWeak, MagnetStrong:
		return mode
	default:
		return MagnetOff
	}
}

// SnapPrice maps a pointer price onto the nearest OHLC of bar when magnet
// is on. Weak mode only snaps when that OHLC is within HitRadius DIPs of
// the pointer in local Y. Ties prefer Open, then High, Low, Close. Any
// mode other than weak or strong returns price unchanged.
func SnapPrice(mode MagnetMode, bar Candle, price, plotH float64, scale chart.Linear) float64 {
	if mode != MagnetWeak && mode != MagnetStrong {
		return price
	}
	ohlc := []float64{bar.Open, bar.High, bar.Low, bar.Close}
	best := price
	bestDist := math.Inf(1)
	found := false
	for _, p := range ohlc {
		if math.IsNaN(p) {
			continue
		}
		d := math.Abs(p - price)
		if !found || d < bestDist {
			bestDist = d
			best = p
			found = true
		}
	}
	if !found {
		return price
	}
	if mode == MagnetStrong {
		return best
	}
	if plotH <= 0 {
		return price
	}
	py := PriceToLocalY(plotH, price, scale)
	by := PriceToLocalY(plotH, best, scale)
	if math.Abs(py-by) <= HitRadius {
		return best
	}
	return price
}
