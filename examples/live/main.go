// Command live is a runnable example of embedding kline with a pushing
// feed. It is the smallest program that shows the whole host contract:
// a DataSource for history, a LiveSource for pushes, ChartOptions.Marshal
// to hop a pushed bar onto the UI thread, and Close to release the feed.
//
//	go run ./examples/live
//
// The feed in this file generates bars in memory. A real host replaces
// marketFeed with its own client and keeps the rest.
package main

import (
	"container/list"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func main() {
	mygo.App.WhenReady(func() {
		app := newLiveExample(newMarketFeed())
		window := mygo.NewWindow(mygo.WindowOptions{
			Title:     "kline live example",
			Width:     960,
			Height:    640,
			MinWidth:  640,
			MinHeight: 420,
			Content:   ui.View(app.view),
		})
		// The view closes over app, so its first frame may arrive before
		// attach installs the chart. attach then installs the Marshal hook.
		if err := app.attach(func() { window.Update(app.applyPending) }); err != nil {
			log.Fatal(err)
		}
		window.OnClosed(app.close)
		app.feed.start()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

type liveExample struct {
	feed  *marketFeed
	chart *kline.Chart
	err   error
	// request asks for a frame. Production posts applyPending through
	// window.Update; the headless test records the same request.
	request func()
	mu      sync.Mutex
	queue   list.List
	frame   bool
	closed  bool
}

func newLiveExample(source *marketFeed) *liveExample {
	return &liveExample{feed: source}
}

func (a *liveExample) attach(request func()) error {
	a.request = request
	dispatch := a.dispatch
	if request == nil {
		dispatch = nil
	}
	a.chart = newLiveChart(a.feed, dispatch)
	a.err = a.chart.LoadError()
	return a.err
}

func newLiveChart(feed kline.LiveSource, dispatch func(func())) *kline.Chart {
	return kline.NewChart(kline.ChartOptions{
		Source:     feed,
		Symbols:    []string{"600519"},
		Period:     kline.Period{Span: 1, Unit: kline.UnitHour},
		Indicators: []string{"MA"},
		Marshal:    dispatch,
	})
}

func (a *liveExample) close() {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.closed = true
	a.queue.Init()
	a.frame = false
	feed := a.feed
	chart := a.chart
	a.mu.Unlock()

	if feed != nil {
		feed.close()
	}
	if chart != nil {
		_ = chart.Close()
	}
}

// dispatch is ChartOptions.Marshal. A feed may push several bars before the
// UI drains one frame, so keep every callback in order instead of replacing
// one slot. Dropping the first new open makes later ones look out of order.
func (a *liveExample) dispatch(fn func()) {
	if fn == nil {
		return
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return
	}
	a.queue.PushBack(fn)
	request := a.request
	if request == nil || a.frame {
		a.mu.Unlock()
		return
	}
	a.frame = true
	a.mu.Unlock()
	request()
}

func (a *liveExample) applyPending() {
	a.mu.Lock()
	if a.closed {
		a.frame = false
		a.mu.Unlock()
		return
	}
	a.frame = false
	a.err = nil
	next := a.queue.Front()
	if next != nil {
		a.queue.Remove(next)
	}
	request := a.request
	if a.queue.Len() > 0 && request != nil && !a.closed {
		a.frame = true
	}
	a.mu.Unlock()
	if next == nil {
		return
	}
	next.Value.(func())()
	if request != nil {
		a.mu.Lock()
		more := !a.closed && a.queue.Len() > 0
		if more {
			a.frame = true
		}
		a.mu.Unlock()
		if more {
			request()
		}
	}
}

func (a *liveExample) view(c *ui.Context) {
	if a.chart == nil {
		return
	}
	a.applyPending()
	chart := a.chart
	ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
		ui.Row(c).Padding(8, 12, 6, 12).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "kline live A-share").Bold().FontSize(15)
			ui.Textf(c, "%s · %d bars", chart.Period(), chart.Bars()).
				FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Box(c).Grow(1)
			for _, p := range []kline.Period{
				{Span: 1, Unit: kline.UnitHour},
				{Span: 4, Unit: kline.UnitHour},
				{Span: 1, Unit: kline.UnitDay},
			} {
				label := p.String()
				var button ui.Element
				if chart.Period() == p {
					button = ui.PrimaryButton(c, label)
				} else {
					button = ui.Button(c, label)
				}
				period := p
				if button.Clicked() {
					a.feed.setPeriod(period)
					if err := chart.SetPeriod(period); err != nil {
						a.err = err
					}
				}
			}
		})
		if a.err != nil {
			ui.Text(c, "error: "+a.err.Error()).Padding(0, 12, 6, 12)
		}
		ui.Column(c).Grow(1).WidthPercent(100).Children(func() {
			chart.View(c)
		})
	})
}

// marketFeed is the host side: history through GetBars, pushes through
// the optional LiveSource methods. It generates bars instead of calling a
// server, so the example runs with no network.
type marketFeed struct {
	mu      sync.Mutex
	period  kline.Period
	history map[kline.Period][]kline.Candle
	subs    map[string]kline.BarHandler
	ticks   int
	stop    chan struct{}
	once    sync.Once
}

func newMarketFeed() *marketFeed {
	f := &marketFeed{
		period:  kline.Period{Span: 1, Unit: kline.UnitHour},
		history: map[kline.Period][]kline.Candle{},
		subs:    map[string]kline.BarHandler{},
		stop:    make(chan struct{}),
	}
	f.history[f.period] = ashareBars(240, f.period)
	return f
}

// GetBars serves history. Bars must be in strictly ascending open time;
// the chart refuses a batch that is not, with kline.ErrBarOrder.
func (f *marketFeed) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	if !req.Period.Valid() {
		return nil, false, kline.ErrInvalidPeriod
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	all := f.history[req.Period]
	switch req.Kind {
	case kline.KindInit:
		limit := req.Limit
		if limit <= 0 || limit > len(all) {
			limit = len(all)
		}
		return append([]kline.Candle(nil), all[len(all)-limit:]...), len(all) > limit, nil
	case kline.KindForward:
		end := 0
		for end < len(all) && all[end].Time.Before(req.Anchor) {
			end++
		}
		start := end - req.Limit
		if start < 0 {
			start = 0
		}
		return append([]kline.Candle(nil), all[start:end]...), start > 0, nil
	default:
		return nil, false, fmt.Errorf("%w: %q", kline.ErrUnsupportedKind, req.Kind)
	}
}

// SubscribeBar stores the handler. The chart calls it once per symbol and
// period, and calls UnsubscribeBar before it subscribes again.
func (f *marketFeed) SubscribeBar(symbol string, period kline.Period, onBar kline.BarHandler) error {
	if onBar == nil {
		return kline.ErrNilBarHandler
	}
	if !period.Valid() {
		return kline.ErrInvalidPeriod
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs[subKey(symbol, period)] = onBar
	return nil
}

func (f *marketFeed) UnsubscribeBar(symbol string, period kline.Period) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.subs, subKey(symbol, period))
	return nil
}

func subKey(symbol string, period kline.Period) string {
	return symbol + "|" + period.String()
}

// ashareBars builds a visible 09:30-11:30 / 13:00-15:00 Asia/Shanghai
// walk for the demo. Real feed code should replace this with its own
// cached data source.
func ashareBars(n int, period kline.Period) []kline.Candle {
	req := kline.BarRequest{
		Symbol: "600519", Period: period, Kind: kline.KindInit, Limit: n,
	}
	src := &kline.SyntheticSource{Extra: -1}
	bars, _, err := src.GetBars(req)
	if err == nil && len(bars) == n {
		return bars
	}
	// SyntheticSource normally satisfies the request; return no demo
	// history rather than silently mixing in a continuous 24-hour walk.
	return nil
}

func (f *marketFeed) setPeriod(p kline.Period) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.period = p
	if _, ok := f.history[p]; !ok {
		f.history[p] = ashareBars(240, p)
	}
}

// start pushes a revised last bar every 900ms. Every fourth tick also
// opens a new bar. The short burst after it can finish before the UI drains
// a frame, which exercises the host queue rather than hiding it.
func (f *marketFeed) start() {
	go func() {
		ticker := time.NewTicker(900 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-f.stop:
				return
			case <-ticker.C:
				f.pushTick()
			}
		}
	}()
}

func (f *marketFeed) pushTick() {
	f.mu.Lock()
	f.ticks++
	tick := f.ticks
	f.mu.Unlock()
	f.pushOnce()
	if tick%4 == 0 {
		f.pushOpen()
	}
}

func (f *marketFeed) pushOnce() {
	f.mu.Lock()
	period := f.period
	onBar := f.subs[subKey("600519", period)]
	series := f.history[period]
	if onBar == nil || len(series) == 0 {
		f.mu.Unlock()
		return
	}
	last := series[len(series)-1]
	last.Close += 0.4
	if last.Close > last.High {
		last.High = last.Close
	}
	if last.Close < last.Low {
		last.Low = last.Close
	}
	last.Volume += 120
	series[len(series)-1] = last
	f.history[period] = series
	f.mu.Unlock()

	// The chart calls the configured Marshal hook, which queues the
	// callback for the UI thread. This goroutine never touches the chart.
	onBar(last)
}

// pushOpen advances the cached series before handing the new open to the
// chart. Pushes use chart order, not delivery order: the UI queue applies
// the new open after the revision that opened it.
func (f *marketFeed) pushOpen() {
	f.mu.Lock()
	period := f.period
	onBar := f.subs[subKey("600519", period)]
	series := f.history[period]
	if onBar == nil || len(series) == 0 {
		f.mu.Unlock()
		return
	}
	last := series[len(series)-1]
	duration, err := period.Duration()
	if err != nil {
		f.mu.Unlock()
		return
	}
	next := last
	next.Time = last.Time.Add(duration)
	next.Open = last.Close
	next.Close = last.Close + 0.2
	next.High = next.Close + 0.3
	next.Low = next.Open - 0.3
	next.Volume = 900
	f.history[period] = append(series, next)
	f.mu.Unlock()

	onBar(next)
}

func (f *marketFeed) close() {
	f.once.Do(func() { close(f.stop) })
}
