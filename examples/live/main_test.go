package main

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

// The example's feed must satisfy the contract the chart relies on, and
// the chart must accept it. A window cannot open in a test, so this
// drives the same feed and the same chart options without one.
func TestMarketFeedDrivesChart(t *testing.T) {
	feed := newMarketFeed()
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	a := newLiveExample(feed)
	if err := a.attach(nil); err != nil {
		t.Fatal(err)
	}
	c := a.chart
	if err := c.LoadError(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.close)
	if c.Bars() != 240 {
		t.Fatalf("bars %d, want 240", c.Bars())
	}
	// The chart subscribed for the symbol and period.
	feed.mu.Lock()
	subs := len(feed.subs)
	feed.mu.Unlock()
	if subs != 1 {
		t.Fatalf("subscriptions %d, want 1", subs)
	}

	last, ok := c.Bar("600519", c.Bars()-1)
	if !ok {
		t.Fatal("no last bar")
	}
	feed.pushOnce()
	after, _ := c.Bar("600519", c.Bars()-1)
	if after.Time != last.Time {
		t.Fatalf("a revision should keep the open time: %v -> %v", last.Time, after.Time)
	}
	if after.Close == last.Close {
		t.Fatal("the revision did not change the close")
	}
	if c.Bars() != 240 {
		t.Fatalf("a revision changed the bar count: %d", c.Bars())
	}

	// History is ascending, or the chart refuses it.
	feed.mu.Lock()
	hist := append([]kline.Candle(nil), feed.history[hour]...)
	feed.mu.Unlock()
	for i := 1; i < len(hist); i++ {
		if !hist[i].Time.After(hist[i-1].Time) {
			t.Fatalf("history is not ascending at %d", i)
		}
		got := hist[i].Time.In(time.FixedZone("Asia/Shanghai", 8*60*60))
		if got.Weekday() == time.Saturday || got.Weekday() == time.Sunday {
			t.Fatalf("history contains a weekend bar: %s", got)
		}
		switch got.Format("15:04") {
		case "09:30", "10:30", "13:00", "14:00":
		default:
			t.Fatalf("history bar opens outside A-share sessions: %s", got)
		}
	}

	// A period change re-subscribes on the same feed. The feed prepares
	// the new period first, then the chart loads it.
	four := kline.Period{Span: 4, Unit: kline.UnitHour}
	feed.setPeriod(four)
	if err := c.SetPeriod(four); err != nil {
		t.Fatal(err)
	}
	if c.Bars() == 0 {
		t.Fatal("the 4h window is empty")
	}

	a.close()
	// After Close a push must not reach the chart.
	bars := c.Bars()
	feed.pushOnce()
	if c.Bars() != bars {
		t.Fatal("a push after Close changed the chart")
	}
}

func TestLiveViewDispatchesAndFrames(t *testing.T) {
	feed := newMarketFeed()
	var invalidations atomic.Int32
	a := newLiveExample(feed)
	a.request = func() {
		invalidations.Add(1)
	}
	if err := a.attach(a.request); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.close)
	tt := ui.NewTester(a.view, 960, 640)
	tt.Frame()
	before, _ := a.chart.Bar("600519", a.chart.Bars()-1)
	if !tt.HasText("1h · 240 bars") {
		t.Fatalf("live view missing: %q", tt.Texts())
	}
	feed.pushOnce()
	if invalidations.Load() != 1 {
		t.Fatalf("push requested %d UI updates, want 1", invalidations.Load())
	}
	a.mu.Lock()
	queued := a.queue.Len()
	a.mu.Unlock()
	if queued != 1 {
		t.Fatalf("the live callback was not queued before the frame request: %d", queued)
	}
	a.applyPending()
	a.mu.Lock()
	queued = a.queue.Len()
	a.mu.Unlock()
	if queued != 0 {
		t.Fatalf("applying the frame left %d callbacks queued", queued)
	}
	tt.Frame()
	after, _ := a.chart.Bar("600519", a.chart.Bars()-1)
	if after.Time != before.Time || after.Close == before.Close {
		t.Fatalf("dispatch did not apply the bar: %.4f -> %.4f", before.Close, after.Close)
	}
	if err := tt.Click("4h"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := a.chart.LoadError(); err != nil {
		t.Fatalf("view period toolbar: %v", err)
	}
	if a.chart.Period() != (kline.Period{Span: 4, Unit: kline.UnitHour}) {
		t.Fatalf("view period toolbar: %s", a.chart.Period())
	}
	a.close()
	feed.pushOnce()
	tt.Frame()
	if invalidations.Load() != 1 {
		t.Fatal("a push after close reached the dispatcher")
	}
}

func TestLiveBackgroundLoopReachesView(t *testing.T) {
	feed := newMarketFeed()
	var invalidations atomic.Int32
	a := newLiveExample(feed)
	a.request = func() {
		invalidations.Add(1)
	}
	if err := a.attach(a.request); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.view, 960, 640)
	tt.Frame()
	before, _ := a.chart.Bar("600519", a.chart.Bars()-1)
	feed.start()
	deadline := time.Now().Add(3 * time.Second)
	for invalidations.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if invalidations.Load() == 0 {
		a.close()
		t.Fatal("background loop did not queue a push")
	}
	a.mu.Lock()
	queued := a.queue.Len()
	a.mu.Unlock()
	if queued == 0 {
		a.close()
		t.Fatal("frame request arrived before the live callback was queued")
	}
	// Stop the loop before the next frame applies the pending callback.
	feed.close()
	tt.Frame()
	after, _ := a.chart.Bar("600519", a.chart.Bars()-1)
	if after.Time != before.Time || after.Close == before.Close {
		t.Fatalf("background push was not applied as a revision: %+v -> %+v", before, after)
	}
	a.close()
	afterClose := invalidations.Load()
	time.Sleep(1100 * time.Millisecond)
	if invalidations.Load() != afterClose {
		t.Fatal("background loop requested a frame after close")
	}
}

func TestLiveDispatchPreservesBurstOpenOrder(t *testing.T) {
	feed := newMarketFeed()
	a := newLiveExample(feed)
	frames := make(chan struct{}, 8)
	var frameRequests atomic.Int32
	a.request = func() {
		frameRequests.Add(1)
		select {
		case frames <- struct{}{}:
		default:
		}
	}
	if err := a.attach(a.request); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.close)
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	before := a.chart.Bars()
	for i := range 3 {
		feed.pushOnce()
		feed.pushOpen()
		if i == 0 {
			// No frame runs before all seven callbacks are queued.
			a.mu.Lock()
			queued := a.queue.Len()
			a.mu.Unlock()
			if queued != 2 {
				t.Fatalf("burst queued %d callbacks, want 2", queued)
			}
		}
	}
	a.mu.Lock()
	queued := a.queue.Len()
	frameRequested := a.frame
	a.mu.Unlock()
	if queued != 6 || !frameRequested {
		t.Fatalf("burst lost callbacks: queue=%d frame=%v", queued, frameRequested)
	}
	// Drain the frame requests in FIFO order, as window.Update would.
	for {
		<-frames
		a.applyPending()
		a.mu.Lock()
		idle := a.queue.Len() == 0
		a.mu.Unlock()
		if idle {
			break
		}
	}
	if a.err != nil {
		t.Fatal(a.err)
	}
	if got, want := a.chart.Bars(), before+3; got != want {
		t.Fatalf("burst chart has %d bars, want %d", got, want)
	}
	feed.mu.Lock()
	want := append([]kline.Candle(nil), feed.history[hour]...)
	feed.mu.Unlock()
	if len(want) != a.chart.Bars() {
		t.Fatalf("feed has %d bars, chart has %d", len(want), a.chart.Bars())
	}
	for i := range want {
		got, ok := a.chart.Bar("600519", i)
		if !ok || got != want[i] {
			t.Fatalf("bar %d differs after burst: %v / %v", i, got, want[i])
		}
	}
}

func TestLiveDispatchFencesCallbacksAfterClose(t *testing.T) {
	feed := newMarketFeed()
	a := newLiveExample(feed)
	var requests atomic.Int32
	if err := a.attach(func() { requests.Add(1) }); err != nil {
		t.Fatal(err)
	}
	feed.pushOnce()
	a.close()
	queued := func() int {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.queue.Len()
	}
	if got := queued(); got != 0 {
		t.Fatalf("close retained %d callbacks", got)
	}
	requestsBefore := requests.Load()
	a.dispatch(func() { t.Error("closed dispatcher ran a callback") })
	if got := queued(); got != 0 || requests.Load() != requestsBefore {
		t.Fatalf("closed dispatcher queued or requested work: queue=%d requests=%d", got, requests.Load())
	}
}

func TestLiveDispatchSlowUIKeepsExactRevisionOrder(t *testing.T) {
	feed := newMarketFeed()
	a := newLiveExample(feed)
	var requests atomic.Int32
	a.request = func() { requests.Add(1) }
	if err := a.attach(a.request); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.close)
	feed.pushOnce()
	feed.pushOpen()
	feed.pushOnce()
	feed.pushOpen()
	before := a.chart.Bars()
	a.applyPending()
	if got := a.chart.Bars(); got != before {
		t.Fatalf("one UI pass applied %d bars", got-before)
	}
	for requests.Load() > 0 {
		requests.Add(-1)
		a.applyPending()
	}
	if got := a.chart.Bars(); got != before+2 {
		t.Fatalf("final chart has %d bars, want %d", got, before+2)
	}
}

// The feed reports a forward window properly, because the example's
// status bar and the chart's paging depend on it.
func TestMarketFeedForward(t *testing.T) {
	feed := newMarketFeed()
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	init, more, err := feed.GetBars(kline.BarRequest{Symbol: "600519", Period: hour, Kind: kline.KindInit, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(init) != 100 || !more {
		t.Fatalf("init %d bars more %v", len(init), more)
	}
	older, moreOlder, err := feed.GetBars(kline.BarRequest{
		Symbol: "600519", Period: hour, Kind: kline.KindForward, Anchor: init[0].Time, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(older) != 50 || !moreOlder {
		t.Fatalf("forward %d bars more %v", len(older), moreOlder)
	}
	if !older[len(older)-1].Time.Before(init[0].Time) {
		t.Fatal("forward bars are not strictly before the anchor")
	}
	if _, _, err := feed.GetBars(kline.BarRequest{Symbol: "600519", Period: hour, Kind: kline.KindBackward}); err == nil {
		t.Fatal("KindBackward should be rejected")
	}
	if _, _, err := feed.GetBars(kline.BarRequest{Symbol: "600519", Period: kline.Period{}}); err == nil {
		t.Fatal("an invalid period should be rejected")
	}
}

// stop is idempotent, so closing the window twice does not panic.
func TestMarketFeedCloseIsIdempotent(t *testing.T) {
	feed := newMarketFeed()
	feed.close()
	feed.close()
	select {
	case <-feed.stop:
	case <-time.After(time.Second):
		t.Fatal("stop was not closed")
	}
}
