// Command history is a plain cached DataSource with more than one page.
// It demonstrates KindForward, a refresh through SetSource, and an
// initial-load failure that the host can retry.
//
//	go run ./examples/history
package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

// cacheFeed is the smallest file-backed-style source: history is held
// in memory and every request is sliced without touching the network.
// failInit makes the first load fail so the retry path is visible.
type cacheFeed struct {
	period   kline.Period
	all      []kline.Candle
	failInit bool
	gets     int
}

func newCacheFeed(period kline.Period) (*cacheFeed, error) {
	req := kline.BarRequest{
		Symbol: "600519", Period: period, Kind: kline.KindInit, Limit: 1400,
	}
	src := &kline.SyntheticSource{Extra: -1}
	bars, _, err := src.GetBars(req)
	if err != nil {
		return nil, err
	}
	return &cacheFeed{period: period, all: bars, failInit: true}, nil
}

func (f *cacheFeed) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	if req.Symbol != "600519" {
		return nil, false, kline.ErrUnknownSymbol
	}
	if req.Period != f.period {
		return nil, false, kline.ErrInvalidPeriod
	}
	if req.Kind != kline.KindInit && req.Kind != kline.KindForward {
		return nil, false, kline.ErrUnsupportedKind
	}
	if req.Kind == kline.KindForward && req.Anchor.IsZero() {
		return nil, false, fmt.Errorf("cached history: forward requires an anchor")
	}
	f.gets++
	if req.Kind == kline.KindInit && f.failInit {
		f.failInit = false
		return nil, false, fmt.Errorf("cached history is not ready yet")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = kline.DefaultLimit
	}
	switch req.Kind {
	case kline.KindInit:
		start := max(len(f.all)-limit, 0)
		return append([]kline.Candle(nil), f.all[start:]...), start > 0, nil
	case kline.KindForward:
		end := sort.Search(len(f.all), func(i int) bool { return !f.all[i].Time.Before(req.Anchor) })
		start := max(end-limit, 0)
		return append([]kline.Candle(nil), f.all[start:end]...), start > 0, nil
	default:
		return nil, false, kline.ErrUnsupportedKind
	}
}

type historyDemo struct {
	chart *kline.Chart
	feed  *cacheFeed
	err   error // SetSource failures do not replace Chart.LoadError.
}

func newHistoryDemo() (*historyDemo, error) {
	period := kline.Period{Span: 1, Unit: kline.UnitHour}
	feed, err := newCacheFeed(period)
	if err != nil {
		return nil, err
	}
	chart := kline.NewChart(kline.ChartOptions{
		Source: feed, Symbols: []string{"600519"}, Period: period,
		Indicators: []string{"MA", "MACD"},
	})
	// Keep this deliberate initial error visible until the user clicks Retry.
	return &historyDemo{chart: chart, feed: feed, err: chart.LoadError()}, nil
}

func historyView(d *historyDemo) func(*ui.Context) {
	chart, feed := d.chart, d.feed
	return func(c *ui.Context) {
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
			ui.Row(c).Padding(10, 16, 8, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "CACHED HISTORY").Bold().FontSize(15)
				ui.Box(c).Grow(1)
				if ui.Button(c, "Retry").Clicked() {
					if chart.Bars() == 0 {
						d.err = chart.SetPeriod(feed.period)
					} else {
						d.err = chart.SetSource(feed)
					}
					c.Invalidate()
				}
				if ui.Button(c, "Reload").Clicked() {
					d.err = chart.SetSource(feed)
					c.Invalidate()
				}
				if ui.Button(c, "Fail refresh").Clicked() {
					feed.failInit = true
					d.err = chart.SetSource(feed)
					c.Invalidate()
				}
			})
			ui.Text(c, statusLine(d)).Padding(0, 16, 8, 16).FontSize(12).
				TextColor(c.Theme().TextMuted)
			ui.Column(c).Grow(1).WidthPercent(100).Children(func() {
				chart.View(c)
			})
		})
	}
}

func statusLine(d *historyDemo) string {
	chart := d.chart
	first, last := chart.Visible()
	parts := []string{
		fmt.Sprintf("bars %d", chart.Bars()),
		fmt.Sprintf("visible %d..%d", first, last),
		fmt.Sprintf("requests %d; drag right for older bars", d.feed.gets),
	}
	if d.err != nil {
		parts = append([]string{"Retry: " + d.err.Error()}, parts...)
	} else if err := chart.LoadError(); err != nil {
		parts = append([]string{"load error: " + err.Error()}, parts...)
	} else if err := chart.AlignError(); err != nil {
		parts = append(parts, "align: "+err.Error())
	}
	return strings.Join(parts, "  ")
}

func main() {
	d, err := newHistoryDemo()
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
			Title:     "kline cached history",
			Width:     960,
			Height:    700,
			MinWidth:  640,
			MinHeight: 460,
			Content:   ui.View(historyView(d)),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
