package kline

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrReplayPeriod is returned when a period change during replay asks
// for bars finer than the series the replay loaded. Those bars are not
// in the revealed prefix, and the chart does not ask the source for them.
var ErrReplayPeriod = errors.New("kline: replay period is finer than the loaded bars")

// ErrReplayClosed is returned when Step, Seek, or SetPeriod runs after
// Close, or after the chart has dropped the replay.
var ErrReplayClosed = errors.New("kline: replay is closed")

// ErrReplayActive is returned by AppendBar and UpdateLast while a replay
// owns the chart's series. The replay reveals one base bar at a time, so
// a host that appended directly would put a bar on screen that the
// cursor has not reached. StartReplay, Step, Seek, and Tick append
// through an internal path and are not affected.
var ErrReplayActive = errors.New("kline: a replay owns the series")

// ReplayBar is one base bar that just became visible. Bars aligns with
// Chart.Symbols and is the base series, not the higher-period aggregate
// on screen. The chart never receives a bar past this one.
type ReplayBar struct {
	Index int
	Time  time.Time
	Bars  []Candle
}

// Replay reveals a loaded series one base bar at a time. The full series
// stays here. The chart's panes only ever hold the revealed prefix, so
// paint, the price and volume scales, indicators, the crosshair, the
// readout, and overlay compare cannot see a future bar.
//
// Play does not start a goroutine. The host calls Tick, or embeds
// Chart.View, which ticks once per frame and asks for another frame
// while playback is running. Close and Pause stop that. A live
// subscription is dropped when the replay starts, so a pushed bar cannot
// append itself ahead of the cursor.
type Replay struct {
	chart      *Chart
	base       [][]Candle
	basePeriod Period
	cursor     int
	speed      float64
	playing    bool
	playAt     time.Time
	playCursor float64
	now        func() time.Time
	hook       func(ReplayBar)
	closed     bool
	// opGen identifies the current top-level replay operation. A
	// callback can pause, close, replace, or otherwise advance the
	// replay; the caller that is still unwinding must stop its batch.
	opGen uint64
	// hadMore is each pane's more flag when the replay started. Close
	// puts it back, so paging left still works on the revealed prefix.
	hadMore []bool
}

// StartReplay copies bars and shows none of them until Step or Seek.
// Keys must include every symbol on the chart, in any order. Every
// series must share open times. The chart's current period is the base
// period: a later SetPeriod may aggregate it, and may not go finer.
// Drawings already on the chart stay. Markers stay and stay hidden
// until the cursor reaches their bar. The live subscription is dropped.
// A successful start clears LoadError: the replay brings its own bars,
// so a failed first load no longer describes the chart.
func StartReplay(chart *Chart, bars map[string][]Candle) (*Replay, error) {
	if chart == nil {
		return nil, fmt.Errorf("kline: nil chart")
	}
	if chart.symbolErr != nil {
		return nil, chart.symbolErr
	}
	if !chart.period.Valid() {
		return nil, ErrInvalidPeriod
	}
	base := make([][]Candle, len(chart.panes))
	for i, s := range chart.panes {
		series, ok := bars[s.name]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownSymbol, s.name)
		}
		if err := checkBarOrder(series); err != nil {
			return nil, fmt.Errorf("%w: %s", err, s.name)
		}
		base[i] = cloneCandles(series)
	}
	if err := alignedSeries(base); err != nil {
		return nil, err
	}
	_ = chart.dropLive()
	if chart.replay != nil {
		chart.replay.Close()
	}
	// Close set the chart closed so a later SetPeriod reloads the suffix.
	// A new replay owns the series again, so that pending reload is gone.
	chart.closed = false
	chart.loadErr = nil
	r := &Replay{
		chart:      chart,
		base:       base,
		basePeriod: chart.period,
		now:        time.Now,
		hadMore:    make([]bool, len(chart.panes)),
	}
	for i := range chart.panes {
		r.hadMore[i] = chart.panes[i].more
	}
	chart.replay = r
	for i := range chart.panes {
		chart.panes[i].candles = nil
		chart.panes[i].held = nil
		chart.panes[i].more = false
		chart.recalc(&chart.panes[i])
		chart.reindexMarks(&chart.panes[i], r.markerView())
	}
	chart.crossIndex = -1
	chart.crossPane = -1
	chart.cancelGesture()
	return r, nil
}

func alignedSeries(base [][]Candle) error {
	if len(base) == 0 {
		return fmt.Errorf("%w: no series", ErrMisaligned)
	}
	n := len(base[0])
	for i := 1; i < len(base); i++ {
		if len(base[i]) != n {
			return fmt.Errorf("%w: series lengths differ", ErrMisaligned)
		}
		for j := 0; j < n; j++ {
			if !base[i][j].Time.Equal(base[0][j].Time) {
				return fmt.Errorf("%w: open times differ at %s", ErrMisaligned, base[0][j].Time.UTC().Format("2006-01-02 15:04"))
			}
		}
	}
	return nil
}

// Cursor is how many base bars are revealed. Zero means the chart is empty.
func (r *Replay) Cursor() int {
	if r == nil {
		return 0
	}
	return r.cursor
}

// OnBar sets the callback for each base bar that becomes visible.
// Step and a forward Seek call it once per bar, after that bar is on
// the chart. StepBack does not. nil clears it. The callback runs on the
// caller of Step, Seek, or Tick and must not call those methods.
func (r *Replay) OnBar(fn func(ReplayBar)) {
	if r == nil {
		return
	}
	r.hook = fn
}

// SetClock replaces the clock Play and Tick read. nil restores time.Now.
// Tests pass a clock they move themselves. A playing replay restarts its
// speed anchor so a clock change does not reveal a pile of bars at once.
func (r *Replay) SetClock(now func() time.Time) {
	if r == nil {
		return
	}
	r.now = now
	if r.playing {
		r.playAt = r.clock()
		r.playCursor = float64(r.cursor)
	}
}

// Step reveals n more base bars. n <= 0 does nothing. The cursor stops
// at the last bar. Each new bar is reported to OnBar.
func (r *Replay) Step(n int) error {
	if err := r.ready(); err != nil {
		return err
	}
	gen := r.nextOp()
	return r.advance(n, true, gen)
}

// StepBack hides n base bars and rebuilds the prefix. It does not report
// bars to OnBar. n past the start leaves the chart empty.
func (r *Replay) StepBack(n int) error {
	if err := r.ready(); err != nil {
		return err
	}
	gen := r.nextOp()
	if n <= 0 {
		return nil
	}
	i := r.cursor - n
	if i < 0 {
		i = 0
	}
	return r.seekIndex(i, gen)
}

// SeekIndex reveals the first i base bars. A forward seek reports each
// newly revealed bar. A backward seek rebuilds the prefix and reports
// nothing. i is clamped to the series.
func (r *Replay) SeekIndex(i int) error {
	if err := r.ready(); err != nil {
		return err
	}
	return r.seekIndex(i, r.nextOp())
}

func (r *Replay) seekIndex(i int, gen uint64) error {
	if err := r.opReady(gen); err != nil {
		return err
	}
	if i < 0 {
		i = 0
	}
	if n := len(r.base[0]); i > n {
		i = n
	}
	if i >= r.cursor {
		return r.advance(i-r.cursor, true, gen)
	}
	r.cursor = i
	r.rebuild()
	r.retarget()
	return nil
}

// SeekTo reveals every base bar whose open time is at or before t.
// A zero t, or a t before the first bar, shows nothing.
func (r *Replay) SeekTo(t time.Time) error {
	if err := r.ready(); err != nil {
		return err
	}
	gen := r.nextOp()
	if len(r.base) == 0 || len(r.base[0]) == 0 || t.IsZero() {
		return r.seekIndex(0, gen)
	}
	cs := r.base[0]
	i := sort.Search(len(cs), func(i int) bool { return cs[i].Time.After(t) })
	return r.seekIndex(i, gen)
}

// Play reveals bars at barsPerSec from the current cursor. Tick applies
// the clock. A non-positive speed pauses. Playback stops at the last bar.
func (r *Replay) Play(barsPerSec float64) {
	if r == nil || r.closed || len(r.base) == 0 {
		return
	}
	r.nextOp()
	if barsPerSec < 0 {
		barsPerSec = 0
	}
	r.speed = barsPerSec
	r.playing = barsPerSec > 0 && r.cursor < len(r.base[0])
	r.playAt = r.clock()
	r.playCursor = float64(r.cursor)
}

// Pause stops playback. The cursor stays where it is.
func (r *Replay) Pause() {
	if r == nil {
		return
	}
	r.nextOp()
	r.playing = false
}

// Playing reports whether Tick will reveal bars.
func (r *Replay) Playing() bool {
	if r == nil {
		return false
	}
	return r.playing
}

// Speed is the bars per second passed to Play.
func (r *Replay) Speed() float64 {
	if r == nil {
		return 0
	}
	return r.speed
}

// Tick reveals the bars the clock says are due since Play. It does
// nothing while paused. A clock that jumps backward does not hide bars.
func (r *Replay) Tick() {
	if r == nil || r.closed || !r.playing || r.speed <= 0 || len(r.base) == 0 {
		return
	}
	gen := r.nextOp()
	if r.cursor >= len(r.base[0]) {
		r.playing = false
		return
	}
	elapsed := r.clock().Sub(r.playAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	want := int(r.playCursor + elapsed*r.speed)
	if want <= r.cursor {
		return
	}
	if want > len(r.base[0]) {
		want = len(r.base[0])
	}
	if err := r.advance(want-r.cursor, false, gen); err != nil {
		// The chart refused the bar, so the cursor cannot move. Stop
		// asking for frames: View invalidates while Playing, and the
		// same reveal would fail on every later frame.
		r.playing = false
		return
	}
	if gen != r.opGen || !r.playing {
		// Pause or another operation ran from OnBar. That operation
		// owns the playback state from this point.
		return
	}
	if r.cursor >= len(r.base[0]) {
		r.playing = false
	}
}

// Close stops playback and detaches the replay from the chart. The bars
// already revealed stay. Further Step and Seek return ErrReplayClosed.
// Close does not restore the hidden suffix, but each pane gets back the
// more flag it had when the replay started, so a visible bar that still
// has older history can be paged in from the source. A later SetPeriod,
// including the period already shown, or SetSource loads the suffix
// again: Close is not final, the same as Chart.Close.
func (r *Replay) Close() {
	if r == nil {
		return
	}
	r.playing = false
	r.closed = true
	if r.chart != nil && r.chart.replay == r {
		r.chart.replay = nil
		// The revealed bars are a prefix, not the loaded window, so the
		// chart must reload before a same-period SetPeriod is a no-op.
		r.chart.closed = true
		for i := range r.chart.panes {
			if i < len(r.hadMore) {
				r.chart.panes[i].more = r.hadMore[i]
			}
		}
	}
}

func (r *Replay) ready() error {
	if r == nil || r.closed || r.chart == nil || r.chart.replay != r || len(r.base) == 0 {
		return ErrReplayClosed
	}
	return nil
}

// nextOp starts a top-level operation. Nested public calls are not used
// internally, so exactly one generation belongs to each Step, Seek,
// Tick, playback-state change, or period change.
func (r *Replay) nextOp() uint64 {
	r.opGen++
	return r.opGen
}

func (r *Replay) opReady(gen uint64) error {
	if err := r.ready(); err != nil {
		return err
	}
	if gen != r.opGen {
		return nil
	}
	return nil
}

func (r *Replay) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *Replay) retarget() {
	if r.playing {
		r.playAt = r.clock()
		r.playCursor = float64(r.cursor)
	}
}

func (r *Replay) advance(n int, retarget bool, gen uint64) error {
	if err := r.opReady(gen); err != nil {
		return err
	}
	if n <= 0 {
		return nil
	}
	for n > 0 && r.cursor < len(r.base[0]) {
		if err := r.ready(); err != nil {
			return err
		}
		if gen != r.opGen {
			return nil
		}
		if err := r.revealOne(); err != nil {
			return err
		}
		n--
		if err := r.ready(); err != nil {
			return err
		}
		if gen != r.opGen {
			return nil
		}
	}
	if retarget && gen == r.opGen && r.chart.replay == r && !r.closed {
		r.retarget()
	}
	return nil
}

func (r *Replay) revealOne() error {
	idx := r.cursor
	if r.aggregated() {
		if err := r.revealAggregate(idx); err != nil {
			return err
		}
	} else {
		for i := range r.base {
			if err := r.chart.appendBar(r.chart.panes[i].name, r.base[i][idx]); err != nil {
				return err
			}
		}
	}
	r.cursor++
	for i := range r.chart.panes {
		r.chart.reindexMarks(&r.chart.panes[i], r.markerView())
	}
	r.fire(idx)
	return nil
}

func (r *Replay) revealAggregate(idx int) error {
	d, err := r.chart.period.Duration()
	if err != nil {
		return err
	}
	loc := r.chart.bucketZone()
	bt := bucketOpen(r.base[0], idx, d, loc)
	same := false
	if n := len(r.chart.panes[0].candles); n > 0 {
		same = r.chart.panes[0].candles[n-1].Time.Equal(bt)
	}
	for i := range r.base {
		c := aggregateOne(r.base[i], idx, d, loc)
		name := r.chart.panes[i].name
		var err error
		if same {
			err = r.chart.updateLast(name, c)
		} else {
			err = r.chart.appendBar(name, c)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Replay) fire(idx int) {
	if r.hook == nil || len(r.base) == 0 || idx < 0 || idx >= len(r.base[0]) {
		return
	}
	bars := make([]Candle, len(r.base))
	for i := range r.base {
		bars[i] = r.base[i][idx]
	}
	r.hook(ReplayBar{Index: idx, Time: bars[0].Time, Bars: bars})
}

func (r *Replay) aggregated() bool {
	if r.chart == nil {
		return false
	}
	cur, err1 := r.chart.period.Duration()
	base, err2 := r.basePeriod.Duration()
	if err1 != nil || err2 != nil {
		return false
	}
	return cur > base
}

// markerView returns the currently displayed period and the exclusive
// end of the revealed base interval. The next hidden base open is the
// boundary while a suffix exists; a gap in the source therefore cannot
// make a hidden marker visible through the preceding bar's nominal
// period.
func (r *Replay) markerView() markerView {
	view := markerView{period: r.chart.period, bounded: true}
	if r == nil || r.chart == nil || len(r.base) == 0 {
		return view
	}
	if r.cursor <= 0 {
		return view
	}
	// The visible interval depends only on bars that have already been
	// revealed. Reading the next hidden bar's open would make a marker
	// appear or disappear when that unseen bar changed.
	view.from = r.base[0][0].Time
	if step, err := r.basePeriod.Duration(); err == nil && step > 0 {
		view.until = r.base[0][r.cursor-1].Time.Add(step)
	}
	return view
}

// setPeriod aggregates the revealed prefix at p. It does not call the
// data source. A period finer than the base series is ErrReplayPeriod.
// Drawings are dropped, as they are on a normal period change, because
// bar indexes no longer name the same candles. Markers stay and are
// mapped onto the new bars.
func (r *Replay) setPeriod(p Period) error {
	if err := r.ready(); err != nil {
		return err
	}
	if !p.Valid() {
		return ErrInvalidPeriod
	}
	next, err := p.Duration()
	if err != nil {
		return err
	}
	base, err := r.basePeriod.Duration()
	if err != nil {
		return err
	}
	if next < base {
		return fmt.Errorf("%w: %s is finer than %s", ErrReplayPeriod, p, r.basePeriod)
	}
	if p == r.chart.period {
		return nil
	}
	// Only a change that can alter the chart starts a new operation.
	// A callback asking for the current or a refused period is a no-op
	// for the batch that is still unwinding.
	gen := r.nextOp()
	r.chart.period = p
	for i := range r.chart.panes {
		r.chart.panes[i].overlays = Layer{}
	}
	r.chart.cancelGesture()
	r.chart.crossIndex = -1
	r.chart.crossPane = -1
	r.rebuild()
	if gen != r.opGen {
		return nil
	}
	return nil
}

// rebuild replaces every pane with the aggregate of the revealed prefix.
// A view that was on the right edge stays there. A panned view keeps
// its offset, clamped to the new length.
func (r *Replay) rebuild() {
	a := r.chart
	pinned := a.pinnedRight()
	before := a.bars()
	for i := range a.panes {
		s := &a.panes[i]
		s.candles = r.displayed(i)
		s.held = nil
		s.more = false
		a.recalc(s)
		a.reindexMarks(s, r.markerView())
	}
	if a.crossIndex >= a.bars() {
		a.crossIndex = -1
		a.crossPane = -1
	}
	if a.plotW > 1 && (pinned || before == 0) {
		a.vp.FitRight(a.bars(), a.plotW)
		if a.bars() > 0 {
			a.fitted = true
		}
		return
	}
	if a.plotW > 1 {
		a.vp.Clamp(a.bars(), a.plotW)
	}
}

func (r *Replay) displayed(sym int) []Candle {
	if r.cursor <= 0 || sym < 0 || sym >= len(r.base) {
		return nil
	}
	prefix := r.base[sym][:r.cursor]
	if !r.aggregated() {
		return cloneCandles(prefix)
	}
	d, err := r.chart.period.Duration()
	if err != nil {
		return cloneCandles(prefix)
	}
	return aggregate(prefix, d, r.chart.bucketZone())
}

func aggregate(cs []Candle, d time.Duration, loc *time.Location) []Candle {
	if len(cs) == 0 || d <= 0 {
		return nil
	}
	out := make([]Candle, 0, len(cs)/4+1)
	var cur Candle
	have := false
	for i, b := range cs {
		bt := bucketOpen(cs, i, d, loc)
		if !have || !bt.Equal(cur.Time) {
			if have {
				out = append(out, cur)
			}
			cur = Candle{Time: bt, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume}
			have = true
			continue
		}
		if b.High > cur.High {
			cur.High = b.High
		}
		if b.Low < cur.Low {
			cur.Low = b.Low
		}
		cur.Close = b.Close
		cur.Volume += b.Volume
	}
	if have {
		out = append(out, cur)
	}
	return out
}

func aggregateOne(cs []Candle, idx int, d time.Duration, loc *time.Location) Candle {
	bt := bucketOpen(cs, idx, d, loc)
	start := idx
	for start > 0 && bucketOpen(cs, start-1, d, loc).Equal(bt) {
		start--
	}
	b0 := cs[start]
	c := Candle{Time: bt, Open: b0.Open, High: b0.High, Low: b0.Low, Close: b0.Close, Volume: b0.Volume}
	for j := start + 1; j <= idx; j++ {
		b := cs[j]
		if b.High > c.High {
			c.High = b.High
		}
		if b.Low < c.Low {
			c.Low = b.Low
		}
		c.Close = b.Close
		c.Volume += b.Volume
	}
	return c
}

// bucketOpen is the open time of the aggregate bar that holds cs[i].
// Hours and minutes truncate to the epoch, so 09:30 in a 4h period lands
// on 08:00. A day is different: it names the first base bar of the local
// calendar day. A source that serves daily bars opens them at the session
// start, such as 09:30 for an A-share day, so the aggregate has to name
// that same instant; otherwise a marker, a drawing, and the readout would
// not line up between a replay and a plain load of the same period.
func bucketOpen(cs []Candle, i int, d time.Duration, loc *time.Location) time.Time {
	if d%(24*time.Hour) != 0 {
		return bucketFloor(cs[i].Time, d, loc)
	}
	floor := bucketFloor(cs[i].Time, d, loc)
	start := i
	for start > 0 && bucketFloor(cs[start-1].Time, d, loc).Equal(floor) {
		start--
	}
	return cs[start].Time
}

// bucketFloor is the nominal open of the bucket that contains t.
func bucketFloor(t time.Time, d time.Duration, loc *time.Location) time.Time {
	if d <= 0 {
		return t.UTC()
	}
	step := d.Nanoseconds()
	if step <= 0 {
		return t.UTC()
	}
	if d%(24*time.Hour) == 0 {
		local := t.In(bucketLocation(loc))
		y, m, day := local.Date()
		return time.Date(y, m, day, 0, 0, 0, 0, local.Location())
	}
	unix := t.UTC().UnixNano()
	if unix < 0 {
		// Floor, not truncate toward zero, so a negative instant still
		// belongs to the bucket that contains it.
		mod := unix % step
		if mod != 0 {
			unix -= step + mod
		}
		return time.Unix(0, unix).UTC()
	}
	return time.Unix(0, unix-unix%step).UTC()
}

// bucketLocation is the zone a daily bucket starts in. A nil zone is
// UTC, matching the label formatter's default.
func bucketLocation(loc *time.Location) *time.Location {
	if loc == nil {
		return time.UTC
	}
	return loc
}
