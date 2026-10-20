package kline

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"time"
)

const defaultExtra = 1800

// hourlyOrigin is the open time of bar 0 from Generate, so an hourly
// init window is that series and not a different walk.
var hourlyOrigin = time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)

// otherOrigin is the open time of the newest bar for periods other than
// one hour. Those series are regenerated at the period step; they are
// not aggregated from the hourly walk.
var otherOrigin = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

// SyntheticSource builds deterministic OHLCV bars in memory.
// Extra is how many bars exist before an init window. Zero means
// defaultExtra. Negative means the init window is the whole history.
// ALPHA and BETA use the same seeds as the original demo (42 and 99).
type SyntheticSource struct {
	Extra int
}

// GetBars implements DataSource.
func (s *SyntheticSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	if s == nil {
		s = &SyntheticSource{}
	}
	if !req.Period.Valid() {
		return nil, false, ErrInvalidPeriod
	}
	step, err := req.Period.Duration()
	if err != nil {
		return nil, false, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	seed := symbolSeed(req.Symbol)
	switch req.Kind {
	case KindInit:
		bars := s.initBars(seed, step, limit)
		return bars, s.extra() > 0, nil
	case KindForward:
		if req.Anchor.IsZero() {
			return nil, false, fmt.Errorf("kline: forward request requires anchor")
		}
		return s.forwardBars(seed, step, limit, req.Anchor)
	default:
		return nil, false, fmt.Errorf("%w: %q", ErrUnsupportedKind, req.Kind)
	}
}

func (s *SyntheticSource) extra() int {
	if s == nil || s.Extra == 0 {
		return defaultExtra
	}
	if s.Extra < 0 {
		return 0
	}
	return s.Extra
}

func (s *SyntheticSource) initBars(seed int64, step time.Duration, limit int) []Candle {
	if step == time.Hour {
		return Generate(limit, seed)
	}
	start := otherOrigin.Add(-time.Duration(limit-1) * step)
	return sampleWalk(limit, seed, start, step)
}

func (s *SyntheticSource) forwardBars(seed int64, step time.Duration, limit int, anchor time.Time) ([]Candle, bool, error) {
	floor := s.floor(step)
	if !anchor.After(floor) {
		return []Candle{}, false, nil
	}
	available := int(anchor.Sub(floor) / step)
	if available > limit {
		available = limit
	}
	if available <= 0 {
		return []Candle{}, false, nil
	}
	start := anchor.Add(-time.Duration(available) * step)
	// A different stream from Generate, so older hourly bars stay
	// deterministic without rewriting the init window's walk.
	bars := sampleWalk(available, seed^0x51ed, start, step)
	return bars, start.After(floor), nil
}

func (s *SyntheticSource) floor(step time.Duration) time.Time {
	extra := time.Duration(s.extra()) * step
	if step == time.Hour {
		return hourlyOrigin.Add(-extra)
	}
	first := otherOrigin.Add(-time.Duration(DefaultLimit-1) * step)
	return first.Add(-extra)
}

func symbolSeed(symbol string) int64 {
	switch symbol {
	case "ALPHA":
		return 42
	case "BETA":
		return 99
	default:
		h := fnv.New64a()
		_, _ = h.Write([]byte(symbol))
		return int64(h.Sum64() & 0x7fffffffffffffff)
	}
}

// sampleWalk is the same OHLC construction as Generate, at an arbitrary
// start and step. Generate itself is unchanged so the hourly demo and
// its tests keep their bytes.
func sampleWalk(n int, seed int64, start time.Time, step time.Duration) []Candle {
	if n <= 0 {
		return nil
	}
	u := uint64(seed)
	rng := rand.New(rand.NewPCG(u, u^0x9e3779b97f4a7c15))
	price := 100.0
	out := make([]Candle, n)
	for i := range out {
		change := (rng.Float64() - 0.48) * 1.6
		open := price
		closePx := open + change
		high := math.Max(open, closePx) + rng.Float64()*0.9
		low := math.Min(open, closePx) - rng.Float64()*0.9
		if low < 1 {
			shift := 1 - low
			low += shift
			open += shift
			closePx += shift
			high += shift
		}
		span := math.Abs(closePx - open)
		volume := 600 + rng.Float64()*4200 + span*1800
		out[i] = Candle{
			Time:   start.Add(time.Duration(i) * step),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePx,
			Volume: volume,
		}
		price = closePx
	}
	return out
}
