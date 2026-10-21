package kline

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"time"
)

const defaultExtra = 1800

// ashareLocation is a fixed UTC+8 zone. Mainland China has no daylight
// saving, so the synthetic calendar does not depend on host tzdata.
var ashareLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// ashareLatestClose is the close of the newest synthetic session.
var ashareLatestClose = time.Date(2024, 6, 28, 15, 0, 0, 0, ashareLocation)

// otherOrigin remains the arbitrary start used by focused tests that do
// not exercise the A-share calendar.
var otherOrigin = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

// SyntheticSource builds deterministic OHLCV bars in memory on the
// regular mainland China A-share weekday session: 09:30-11:30 and
// 13:00-15:00 Asia/Shanghai. Weekend gaps are omitted. Exchange
// holidays are not guessed because no authoritative holiday calendar
// is bundled, so this is demo/test data, not a production trading
// calendar.
//
// Extra is how many bars exist before an init window. Zero means
// defaultExtra. Negative means the init window is the whole history.
// The default symbols use deterministic seeds: 600519 is 42 and 000001
// is 99. It implements DataSource only and is not a LiveSource.
type SyntheticSource struct {
	Extra int
}

type syntheticSeries struct {
	newest time.Time
	bars   []Candle
}

// GetBars implements DataSource.
func (s *SyntheticSource) GetBars(req BarRequest) ([]Candle, bool, error) {
	if s == nil {
		s = &SyntheticSource{}
	}
	if !req.Period.Valid() {
		return nil, false, ErrInvalidPeriod
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	seed := symbolSeed(req.Symbol)
	series, err := s.series(seed, req.Period, max(limit+s.extra(), limit))
	if err != nil {
		return nil, false, err
	}
	switch req.Kind {
	case KindInit:
		if len(series.bars) == 0 {
			return nil, false, nil
		}
		start := max(len(series.bars)-limit, 0)
		bars := cloneCandles(series.bars[start:])
		return bars, start > 0, nil
	case KindForward:
		if req.Anchor.IsZero() {
			return nil, false, fmt.Errorf("kline: forward request requires anchor")
		}
		end := len(series.bars)
		for end > 0 && !series.bars[end-1].Time.Before(req.Anchor) {
			end--
		}
		start := max(end-limit, 0)
		if end == 0 {
			return []Candle{}, false, nil
		}
		return cloneCandles(series.bars[start:end]), start > 0, nil
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

func (s *SyntheticSource) series(seed int64, period Period, count int) (syntheticSeries, error) {
	opens := ashareOpens(ashareLatestClose, period, count)
	if len(opens) == 0 {
		return syntheticSeries{}, fmt.Errorf("kline: no A-share bars for %s", period)
	}
	bars := make([]Candle, len(opens))
	synthesizeWalk(bars, opens, seed)
	return syntheticSeries{
		newest: bars[len(bars)-1].Time,
		bars:   bars,
	}, nil
}

// ashareOpens returns the newest n opens ending no later than end, in
// ascending order. One-hour bars use the four regular hourly starts.
// Sub-hour bars walk each two-hour session at their period. Four-hour and
// daily periods use one 09:30 open per weekday. Weekends are skipped.
// Exchange holidays are not guessed without an authoritative calendar.
func ashareOpens(end time.Time, period Period, n int) []time.Time {
	if n <= 0 {
		return nil
	}
	out := make([]time.Time, 0, n)
	day := end.In(ashareLocation)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, ashareLocation)
	for len(out) < n {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			opens := ashareDayOpens(day, period)
			for i := len(opens) - 1; i >= 0 && len(out) < n; i-- {
				if opens[i].Before(end) {
					out = append(out, opens[i])
				}
			}
		}
		day = day.AddDate(0, 0, -1)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func ashareDayOpens(day time.Time, period Period) []time.Time {
	step, err := period.Duration()
	if err != nil {
		return nil
	}
	morning := day.Add(9*time.Hour + 30*time.Minute)
	afternoon := day.Add(13 * time.Hour)
	if period.Unit == UnitDay || step >= 4*time.Hour {
		return []time.Time{morning}
	}
	var out []time.Time
	for t := morning; !t.After(day.Add(11*time.Hour + 30*time.Minute).Add(-step)); t = t.Add(step) {
		out = append(out, t)
	}
	for t := afternoon; !t.After(day.Add(15 * time.Hour).Add(-step)); t = t.Add(step) {
		out = append(out, t)
	}
	return out
}

func synthesizeWalk(bars []Candle, opens []time.Time, seed int64) {
	u := uint64(seed)
	rng := rand.New(rand.NewPCG(u, u^0x9e3779b97f4a7c15))
	price := 100.0
	for i := range bars {
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
		bars[i] = Candle{
			Time:   opens[i],
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePx,
			Volume: 600 + rng.Float64()*4200 + math.Abs(closePx-open)*1800,
		}
		price = closePx
	}
}

func symbolSeed(symbol string) int64 {
	switch symbol {
	case "600519", "ALPHA":
		return 42
	case "000001", "BETA":
		return 99
	default:
		h := fnv.New64a()
		_, _ = h.Write([]byte(symbol))
		return int64(h.Sum64() & 0x7fffffffffffffff)
	}
}

// sampleWalk remains an arbitrary-time fixture for focused tests. The
// A-share SyntheticSource above owns the session calendar.
func sampleWalk(n int, seed int64, start time.Time, step time.Duration) []Candle {
	if n <= 0 {
		return nil
	}
	out := make([]Candle, n)
	opens := make([]time.Time, n)
	for i := range opens {
		opens[i] = start.Add(time.Duration(i) * step)
	}
	synthesizeWalk(out, opens, seed)
	return out
}
