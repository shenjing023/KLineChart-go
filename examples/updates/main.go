// Command updates demonstrates host-driven AppendBar, UpdateLast, and
// shared-index alignment for two symbols. It uses an offline fixture; the
// controls are the host's application logic, not a market-data client.
//
//	go run ./examples/updates
package main

import (
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

var updatePeriod = kline.Period{Span: 1, Unit: kline.UnitHour}

type updateFeed struct {
	lastUpdate time.Time
}

func newUpdateFeed() *updateFeed {
	return &updateFeed{}
}

func (f *updateFeed) series(symbol string) ([]kline.Candle, bool) {
	history := map[string][]kline.Candle{
		"600519": kline.Generate(32, 17),
		"000001": kline.Generate(32, 29),
	}
	bars, ok := history[symbol]
	if !ok {
		return nil, false
	}
	if !f.lastUpdate.IsZero() {
		for i := range bars {
			bars[i].Close += 1
			bars[i].High += 1
		}
	}
	return bars, true
}

func (f *updateFeed) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	all, ok := f.series(req.Symbol)
	if !ok {
		return nil, false, fmt.Errorf("%w: %s", kline.ErrUnknownSymbol, req.Symbol)
	}
	if req.Period != updatePeriod {
		return nil, false, kline.ErrInvalidPeriod
	}
	if req.Kind != kline.KindInit && req.Kind != kline.KindForward {
		return nil, false, kline.ErrUnsupportedKind
	}
	if req.Kind == kline.KindForward && req.Anchor.IsZero() {
		return nil, false, fmt.Errorf("updates fixture: forward requires an anchor")
	}
	end := len(all)
	if req.Kind == kline.KindForward {
		end = sort.Search(len(all), func(i int) bool { return !all[i].Time.Before(req.Anchor) })
	}
	limit := req.Limit
	if limit <= 0 {
		limit = kline.DefaultLimit
	}
	start := max(end-limit, 0)
	return append([]kline.Candle(nil), all[start:end]...), start > 0, nil
}

type updatesDemo struct {
	chart  *kline.Chart
	source *updateFeed
	// pending is the latest submitted bar per symbol. The chart keeps it
	// held until both symbols reach that open time; Bar exposes only the
	// visible prefix, so the host keeps its own copy for revisions.
	pending map[string]kline.Candle
	state   map[string]string
	toolbar ui.ScrollState
	err     error
	note    string
}

func newUpdatesDemo() (*updatesDemo, error) {
	source := newUpdateFeed()
	chart := kline.NewChart(kline.ChartOptions{
		Source:     source,
		Symbols:    []string{"600519", "000001"},
		Period:     updatePeriod,
		Indicators: []string{"MA"},
		Location:   time.UTC,
	})
	if err := chart.LoadError(); err != nil {
		return nil, err
	}
	return &updatesDemo{chart: chart, source: source,
		pending: map[string]kline.Candle{},
		state:   map[string]string{},
		note:    "append A and B with the same open time"}, nil
}

func (d *updatesDemo) nextBar(symbol string, hours int) (kline.Candle, error) {
	last, ok := d.chart.Bar(symbol, d.chart.Bars()-1)
	if !ok {
		return kline.Candle{}, fmt.Errorf("no last bar for %s", symbol)
	}
	if pending, ok := d.pending[symbol]; ok && !pending.Time.Before(last.Time) {
		last = pending
	}
	next := last
	next.Time = last.Time.Add(time.Duration(hours) * time.Hour)
	next.Open = last.Close
	delta := 0.8
	if symbol == "000001" {
		delta = -0.6
	}
	next.Close = next.Open + delta
	next.High = math.Max(next.Open, next.Close) + 0.4
	next.Low = math.Min(next.Open, next.Close) - 0.4
	next.Volume = 1200 + float64(d.chart.Bars()%5)*90
	return next, nil
}

func (d *updatesDemo) append(symbol string, hours int) {
	bar, err := d.nextBar(symbol, hours)
	if err != nil {
		d.err = err
		return
	}
	d.err = d.chart.AppendBar(symbol, bar)
	if d.err == nil {
		d.pending[symbol] = bar
	}
	d.reconcile()
	state := d.state[symbol]
	note := fmt.Sprintf("AppendBar(%s, +%dh): %s", symbol, hours, state)
	d.note = note
}

func (d *updatesDemo) revise(symbol string) {
	last, ok := d.chart.Bar(symbol, d.chart.Bars()-1)
	if !ok {
		d.err = fmt.Errorf("no last bar for %s", symbol)
		return
	}
	if d.hasHeldTail(symbol) {
		d.err = fmt.Errorf("revise shown %s: another symbol has a held edge", symbol)
		d.note = "UpdateLast shown " + symbol + " refused: system has a held edge"
		return
	}
	revised := last
	revised.Close = last.Close + 1
	revised.High = math.Max(last.High, revised.Close)
	d.err = d.chart.UpdateLast(symbol, revised)
	if d.err == nil {
		d.pending[symbol] = revised
		d.state[symbol] = "shown"
	}
	d.note = fmt.Sprintf("UpdateLast(%s): close %.2f -> %.2f", symbol, last.Close, revised.Close)
}

func (d *updatesDemo) revisePending(symbol string) {
	// UpdateLast also revises a held bar, but Bar does not expose held
	// bars. A real feed received that price from its own push, so keep
	// the last submitted bar in the demo.
	next, ok := d.pending[symbol]
	last, visible := d.chart.Bar(symbol, d.chart.Bars()-1)
	if !ok || !visible || !next.Time.After(last.Time) {
		// A successful skew can leave a newer held bar on only one symbol.
		// The other symbol's submitted snapshot is the visible edge until
		// it also advances.
		d.err = fmt.Errorf("no pending bar for %s", symbol)
		d.note = "UpdateLast(held " + symbol + ") refused: no pending bar"
		return
	}
	// A held bar remains provisional after an earlier alignment error. Pending
	// state answers the latest submission; AlignError is a historical report.
	d.state[symbol] = "held"
	revised := next
	revised.Close += 1
	revised.High = math.Max(next.High, revised.Close)
	wasHeld := next.Time.After(last.Time)
	d.err = d.chart.UpdateLast(symbol, revised)
	if d.err == nil {
		d.pending[symbol] = revised
		if wasHeld {
			d.state[symbol] = "held"
		}
	}
	d.note = fmt.Sprintf("UpdateLast(%s, held): close %.2f -> %.2f", symbol, next.Close, revised.Close)
}

func (d *updatesDemo) skew() {
	for _, item := range []struct {
		symbol string
		hours  int
	}{{"600519", 2}, {"000001", 1}} {
		bar, err := d.nextBar(item.symbol, item.hours)
		if err != nil {
			d.err = err
			d.note = "skew stopped at " + item.symbol + "; " + err.Error()
			return
		}
		if err := d.chart.AppendBar(item.symbol, bar); err != nil {
			d.err = err
			d.reconcile()
			d.note = "skew stopped at " + item.symbol + "; " + err.Error()
			return
		}
		d.pending[item.symbol] = bar
	}
	d.err = nil
	d.reconcile()
	d.note = "skewed feeds: A +2h, B +1h; AlignError should report the gap"
}

func (d *updatesDemo) reset() {
	d.source.lastUpdate = time.Date(2024, 6, 29, 0, 0, 0, 0, time.UTC)
	d.err = d.chart.SetSource(d.source)
	d.pending = map[string]kline.Candle{}
	d.state = map[string]string{}
	if d.err == nil {
		for _, symbol := range d.chart.Symbols() {
			d.state[symbol] = "shown"
		}
	}
	d.note = fmt.Sprintf("reload revised fixture at %s; held and dropped bars are gone",
		d.source.lastUpdate.Format(time.RFC3339))
}

func (d *updatesDemo) status() string {
	line := fmt.Sprintf("bars %d | %s | A %s | B %s | %s", d.chart.Bars(), d.chart.Period(),
		d.stateText("600519"), d.stateText("000001"), d.note)
	if err := d.chart.AlignError(); err != nil {
		line += " | align: " + err.Error()
	}
	if err := d.chart.LoadError(); err != nil {
		line += " | load: " + err.Error()
	}
	return line
}

// reconcile ties the host's submitted snapshot to what the chart shows
// after an arbitrary button order. A matching visible edge adopts the
// chart's copy; a newer submitted bar is held; an older or ambiguous
// one is no longer revivable.
func (d *updatesDemo) reconcile() {
	for _, symbol := range d.chart.Symbols() {
		pending, ok := d.pending[symbol]
		if !ok {
			continue
		}
		last, visible := d.chart.Bar(symbol, d.chart.Bars()-1)
		switch {
		case visible && pending.Time.Equal(last.Time):
			d.pending[symbol] = last
			d.state[symbol] = "shown"
		case visible && pending.Time.After(last.Time):
			d.state[symbol] = "held"
		default:
			delete(d.pending, symbol)
			d.state[symbol] = "stale"
		}
	}
}

func (d *updatesDemo) stateText(symbol string) string {
	state := d.state[symbol]
	if state == "" {
		state = "shown"
	}
	pending, ok := d.pending[symbol]
	if !ok {
		if last, visible := d.chart.Bar(symbol, d.chart.Bars()-1); visible {
			return fmt.Sprintf("%s %s", state, last.Time.Format("15:04"))
		}
		return state
	}
	return fmt.Sprintf("%s %s", state, pending.Time.Format("15:04"))
}

func (d *updatesDemo) revisePendingEnabled(symbol string) bool {
	return d.hasHeldTail(symbol)
}

func (d *updatesDemo) hasHeldTail(symbol string) bool {
	next, ok := d.pending[symbol]
	if !ok {
		return false
	}
	last, visible := d.chart.Bar(symbol, d.chart.Bars()-1)
	return visible && next.Time.After(last.Time)
}

func (d *updatesDemo) reviseEnabled(symbol string) bool {
	_, visible := d.chart.Bar(symbol, d.chart.Bars()-1)
	for _, name := range d.chart.Symbols() {
		if d.hasHeldTail(name) {
			return false
		}
	}
	return visible
}

func updatesView(d *updatesDemo) func(*ui.Context) {
	chart := d.chart
	return func(c *ui.Context) {
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
			ui.ScrollHorizontal(c).TrackScroll(&d.toolbar).Children(func() {
				ui.Row(c).Padding(10, 16, 4, 16).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "APPEND / UPDATE").Bold().FontSize(15)
					if ui.Button(c, "Append A").Clicked() {
						d.append("600519", 1)
						c.Invalidate()
					}
					if ui.Button(c, "Append B").Clicked() {
						d.append("000001", 1)
						c.Invalidate()
					}
					if ui.Button(c, "Revise A").Disabled(!d.reviseEnabled("600519")).Clicked() {
						d.revise("600519")
						c.Invalidate()
					}
					if ui.Button(c, "Revise held A").Disabled(!d.revisePendingEnabled("600519")).Clicked() {
						d.revisePending("600519")
						c.Invalidate()
					}
					if ui.Button(c, "Skew feeds").Clicked() {
						d.skew()
						c.Invalidate()
					}
					if ui.Button(c, "Reset").Clicked() {
						d.reset()
						c.Invalidate()
					}
				})
			})
			if d.err != nil {
				ui.Text(c, "error: "+d.err.Error()).Padding(0, 16, 4, 16).TextColor(c.Theme().TextMuted)
			}
			ui.ScrollHorizontal(c).Children(func() {
				ui.Text(c, "Append A and B at one open time; while an edge is held, only the held revision is available.").
					Padding(0, 16, 4, 16).FontSize(12).NoWrap().TextColor(c.Theme().TextMuted)
			})
			ui.ScrollHorizontal(c).Children(func() {
				ui.Text(c, d.status()).Padding(0, 16, 8, 16).Font("monospace").FontSize(12).NoWrap().
					TextColor(c.Theme().TextMuted)
			})
			ui.Column(c).Grow(1).WidthPercent(100).Children(func() { chart.View(c) })
		})
	}
}

func main() {
	d, err := newUpdatesDemo()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := d.chart.Close(); err != nil {
			log.Print(err)
		}
	}()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title: "kline append updates", Width: 960, Height: 700,
			MinWidth: 640, MinHeight: 460, Content: ui.View(updatesView(d)),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
