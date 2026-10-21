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
		// The window comes first: its Update is the hook that moves a
		// pushed bar onto the UI thread, and ChartOptions needs it.
		var window *mygo.Window
		var chart *kline.Chart
		feed := newMarketFeed()

		window = mygo.NewWindow(mygo.WindowOptions{
			Title:     "kline live example",
			Width:     960,
			Height:    640,
			MinWidth:  640,
			MinHeight: 420,
			Content: ui.View(func(c *ui.Context) {
				if chart == nil {
					return
				}
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
								// The feed has to be able to answer for the
								// new period before the chart asks, so the
								// history is prepared first.
								feed.setPeriod(period)
								// SetPeriod subscribes the new period on the
								// same source. A failed load keeps the bars.
								if err := chart.SetPeriod(period); err != nil {
									log.Println("period:", err)
								}
							}
						}
					})
					ui.Column(c).Grow(1).WidthPercent(100).Children(func() {
						chart.View(c)
					})
				})
			}),
		})

		chart = kline.NewChart(kline.ChartOptions{
			Source:  feed,
			Symbols: []string{"600519"},
			Period:  kline.Period{Span: 1, Unit: kline.UnitHour},
			// One indicator keeps the first push cheap; every Calc walks
			// the series, and a live append only pays for the tail.
			Indicators: []string{"MA"},
			// Update runs the append on the UI thread and redraws. Without
			// it the feed's goroutine would write the series while View
			// paints it.
			Marshal: window.Update,
		})
		if err := chart.LoadError(); err != nil {
			log.Fatal(err)
		}

		window.OnClosed(func() {
			feed.close()
			// Unsubscribe and fence any handler that is still in flight.
			_ = chart.Close()
		})
		feed.start()
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// marketFeed is the host side: history through GetBars, pushes through
// the optional LiveSource methods. It generates bars instead of calling a
// server, so the example runs with no network.
type marketFeed struct {
	mu      sync.Mutex
	period  kline.Period
	history map[kline.Period][]kline.Candle
	subs    map[string]kline.BarHandler
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

// start pushes a revised last bar every 900ms. The open time does not
// change, so the chart replaces the newest bar in place and updates the
// indicator tail instead of recalculating the series.
func (f *marketFeed) start() {
	go func() {
		ticker := time.NewTicker(900 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-f.stop:
				return
			case <-ticker.C:
				f.pushOnce()
			}
		}
	}()
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

	// The chart's Marshal hook routes this to the UI thread. This
	// goroutine never touches the chart.
	onBar(last)
}

func (f *marketFeed) close() {
	f.once.Do(func() { close(f.stop) })
}
