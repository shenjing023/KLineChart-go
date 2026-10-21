package main

import (
	"testing"
	"time"

	"github.com/shenjing023/KLineChart-go/kline"
)

// The example's feed must satisfy the contract the chart relies on, and
// the chart must accept it. A window cannot open in a test, so this
// drives the same feed and the same chart options without one.
func TestMarketFeedDrivesChart(t *testing.T) {
	feed := newMarketFeed()
	hour := kline.Period{Span: 1, Unit: kline.UnitHour}
	c := kline.NewChart(kline.ChartOptions{
		Source:     feed,
		Symbols:    []string{"600519"},
		Period:     hour,
		Indicators: []string{"MA"},
		// Nil Marshal applies the bar on the caller, which is how a test
		// stands in for window.Update.
		Marshal: nil,
	})
	if err := c.LoadError(); err != nil {
		t.Fatal(err)
	}
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

	feed.close()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	// After Close a push must not reach the chart.
	bars := c.Bars()
	feed.pushOnce()
	if c.Bars() != bars {
		t.Fatal("a push after Close changed the chart")
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
