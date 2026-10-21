package kline

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"time"
)

const defaultExtra = 1800

// maxSyntheticBars bounds one generated walk. The default Extra (1800)
// plus the default init window fits inside it; a host asking for a longer
// window gets the newest maxSyntheticBars bars instead of an ever-growing
// slice.
const maxSyntheticBars = 1 << 16

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
// The walk ends on a fixed newest bar and is generated backwards from
// it, so a bar's prices depend on how far it is from that bar, not on the
// window a request asked for. A forward batch therefore joins the init
// window without a jump, and two windows that share open times agree on
// every shared bar.
//
// Extra adds history to DefaultLimit, independently of request page size.
// Zero means defaultExtra. Negative serves exactly the requested init
// window with no older history. Total history is capped at 65,536 bars.
// The default symbols use deterministic seeds: 600519 is 42 and 000001
// is 99. It implements DataSource only and is not a LiveSource.
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
	if !req.Period.Loadable() {
		// A week has no session calendar here: the chart builds it by
		// aggregating daily bars, so asking this source for one is a
		// request it cannot serve rather than a walk it should invent.
		return nil, false, fmt.Errorf("%w: %s has no synthetic calendar", ErrInvalidPeriod, req.Period)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > maxSyntheticBars {
		limit = maxSyntheticBars
	}
	if req.Kind != KindInit && req.Kind != KindForward {
		return nil, false, fmt.Errorf("%w: %q", ErrUnsupportedKind, req.Kind)
	}
	if req.Kind == KindForward && req.Anchor.IsZero() {
		return nil, false, fmt.Errorf("kline: forward request requires anchor")
	}
	if _, err := req.Period.Duration(); err != nil {
		return nil, false, err
	}
	count := DefaultLimit + min(s.extra(), maxSyntheticBars-DefaultLimit)
	if s.Extra < 0 {
		if req.Kind == KindForward {
			return []Candle{}, false, nil
		}
		count = limit
	}
	seed := symbolSeed(req.Symbol)
	bars, err := s.series(seed, req.Period, count)
	if err != nil {
		return nil, false, err
	}
	switch req.Kind {
	case KindInit:
		if len(bars) == 0 {
			return nil, false, nil
		}
		start := max(len(bars)-limit, 0)
		return cloneCandles(bars[start:]), start > 0, nil
	case KindForward:
		end := len(bars)
		for end > 0 && !bars[end-1].Time.Before(req.Anchor) {
			end--
		}
		start := max(end-limit, 0)
		if end == 0 {
			return []Candle{}, false, nil
		}
		return cloneCandles(bars[start:end]), start > 0, nil
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

// series generates the walk ending at the newest session. count is how
// many bars it holds; the walk is generated backwards from the newest
// bar, so a longer walk does not shift the bars it already had.
func (s *SyntheticSource) series(seed int64, period Period, count int) ([]Candle, error) {
	opens, err := ashareOpens(ashareLatestClose, period, count)
	if err != nil {
		return nil, err
	}
	bars := make([]Candle, len(opens))
	synthesizeWalk(bars, opens, seed)
	return bars, nil
}

// ashareOpens returns the newest n opens ending no later than end, in
// ascending order. One-hour bars use the four regular hourly starts.
// Sub-hour bars walk each two-hour session at their period. Four-hour and
// daily periods use one 09:30 open per weekday. Weekends are skipped.
// Exchange holidays are not guessed without an authoritative calendar.
//
// A period whose step does not fit a two-hour session, such as 3h, has no
// opens at all. It returns ErrInvalidPeriod instead of scanning the
// calendar forever.
func ashareOpens(end time.Time, period Period, n int) ([]time.Time, error) {
	if n <= 0 {
		return nil, nil
	}
	if _, err := period.Duration(); err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, min(n, 4096))
	day := end.In(ashareLocation)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, ashareLocation)
	// A weekday has opens for every valid period, so the first weekday
	// settles whether this period can walk the calendar at all.
	for day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		day = day.AddDate(0, 0, -1)
	}
	if len(ashareDayOpens(day, period)) == 0 {
		return nil, fmt.Errorf("%w: %s has no A-share session bars", ErrInvalidPeriod, period)
	}
	// Bound the scan as well, so a future calendar rule that leaves a
	// weekday empty cannot spin. A walk with one bar per weekday, such as
	// 4h or 1d, needs about 7/5 days per bar.
	for scanned := 0; len(out) < n; scanned++ {
		if scanned > n*7/5+366 {
			return nil, fmt.Errorf("%w: %s has no A-share session bars", ErrInvalidPeriod, period)
		}
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
	return out, nil
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

// synthesizeWalk fills bars backwards from the newest one. Each bar's
// prices are drawn in order of distance from the newest open, so a bar
// keeps its OHLCV whatever window a request slices it from. The close of
// bar i is the open of bar i+1. The walk is multiplicative, so a long
// series cannot drift through zero and needs no re-anchoring that would
// depend on the window length.
func synthesizeWalk(bars []Candle, opens []time.Time, seed int64) {
	u := uint64(seed)
	rng := rand.New(rand.NewPCG(u, u^0x9e3779b97f4a7c15))
	closePx := 60 + rng.Float64()*80
	for i := len(bars) - 1; i >= 0; i-- {
		chg := (rng.Float64() - 0.48) * 0.016
		open := closePx / (1 + chg)
		high := math.Max(open, closePx) * (1 + rng.Float64()*0.004)
		low := math.Min(open, closePx) * (1 - rng.Float64()*0.004)
		bars[i] = Candle{
			Time:   opens[i],
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePx,
			Volume: 600 + rng.Float64()*4200 + math.Abs(closePx-open)*1800,
		}
		closePx = open
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
