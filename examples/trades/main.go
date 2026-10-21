// Command trades demonstrates stacked trades and style controls on a fixed,
// offline fixture. Its 4h bars aggregate the same 1h OHLCV data.
//
// go run ./examples/trades
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

// This is an illustrative continuous hourly fixture, not an exchange calendar.
type tradeFeed struct{ hourly []kline.Candle }

func newTradeFeed() *tradeFeed {
	bars := make([]kline.Candle, 640)
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	prev := 100.0
	for i := range bars {
		close := 100 + 3*math.Sin(float64(i)/6)
		switch i {
		case len(bars) - 25:
			close = 100
		case len(bars) - 24:
			close = 110 // Exactly +10% from the previous close.
		case len(bars) - 23:
			close = 99 // Exactly -10% from 110.
		}
		bars[i] = kline.Candle{Time: start.Add(time.Duration(i) * time.Hour),
			Open: prev, Close: close, High: math.Max(prev, close) + 0.8,
			Low: math.Min(prev, close) - 0.8, Volume: 1000 + float64(i%7)*100}
		prev = close
	}
	return &tradeFeed{hourly: bars}
}

func (f *tradeFeed) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	if req.Symbol != "600519" {
		return nil, false, kline.ErrUnknownSymbol
	}
	if req.Period.Unit != kline.UnitHour || (req.Period.Span != 1 && req.Period.Span != 4) {
		return nil, false, kline.ErrInvalidPeriod
	}
	bars := f.hourly
	if req.Period.Span == 4 {
		bars = nil
		for _, c := range f.hourly {
			open := c.Time.Truncate(4 * time.Hour)
			if len(bars) == 0 || !bars[len(bars)-1].Time.Equal(open) {
				c.Time = open
				bars = append(bars, c)
				continue
			}
			b := &bars[len(bars)-1]
			b.High, b.Low = math.Max(b.High, c.High), math.Min(b.Low, c.Low)
			b.Close, b.Volume = c.Close, b.Volume+c.Volume
		}
	}
	end := len(bars)
	switch req.Kind {
	case kline.KindInit:
	case kline.KindForward:
		if req.Anchor.IsZero() {
			return nil, false, fmt.Errorf("fixture: missing forward anchor")
		}
		end = sort.Search(len(bars), func(i int) bool { return !bars[i].Time.Before(req.Anchor) })
	default:
		return nil, false, kline.ErrUnsupportedKind
	}
	limit := req.Limit
	if limit <= 0 {
		limit = kline.DefaultLimit
	}
	first := max(end-limit, 0)
	return append([]kline.Candle(nil), bars[first:end]...), first > 0, nil
}

type tradesDemo struct {
	chart     *kline.Chart
	saved     []kline.Marker
	alternate bool
	err       error
}

func newTradesDemo() (*tradesDemo, error) {
	chart := kline.NewChart(kline.ChartOptions{Source: newTradeFeed(),
		Symbols: []string{"600519"}, Indicators: []string{}})
	if err := chart.LoadError(); err != nil {
		return nil, err
	}
	d := &tradesDemo{chart: chart}
	buy, ok := chart.Bar("600519", chart.Bars()-16)
	if !ok {
		return nil, fmt.Errorf("fixture buy bar missing")
	}
	for i := range 3 {
		// Distinct fills within ONE bar produce a three-arrow buy stack.
		d.saved = append(d.saved, kline.Marker{Time: buy.Time.Add(time.Duration(i+1) * 10 * time.Minute),
			Side: kline.SideBuy, Price: buy.Low + 0.5, Qty: 100, Label: fmt.Sprintf("B%d", i+1)})
	}
	sell, ok := chart.Bar("600519", chart.Bars()-8)
	if !ok {
		return nil, fmt.Errorf("fixture sell bar missing")
	}
	d.saved = append(d.saved, kline.Marker{Time: sell.Time.Add(30 * time.Minute),
		Side: kline.SideSell, Price: sell.High - 0.5, Qty: 300, Label: "S"})
	if err := chart.SetMarkers("600519", d.saved); err != nil {
		return nil, err
	}
	chart.SetLimitPct(0.10)
	return d, nil
}

func tradesView(d *tradesDemo) func(*ui.Context) {
	chart := d.chart
	return func(c *ui.Context) {
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
			ui.Row(c).Padding(10, 16, 4, 16).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "TRADE MARKERS").Bold().FontSize(15)
				ui.Box(c).Grow(1)
				for _, p := range []kline.Period{{Span: 1, Unit: kline.UnitHour}, {Span: 4, Unit: kline.UnitHour}} {
					if ui.Button(c, p.String()).Clicked() {
						d.err = chart.SetPeriod(p)
						c.Invalidate()
					}
				}
				if ui.Button(c, "Clear").Clicked() {
					d.err = chart.ClearMarkers("600519")
					c.Invalidate()
				}
				if ui.Button(c, "Restore").Clicked() {
					d.err = chart.SetMarkers("600519", d.saved)
					c.Invalidate()
				}
			})
			ui.Row(c).Padding(0, 16, 4, 16).Gap(10).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "Palette").Clicked() {
					d.alternate = !d.alternate
					chart.EditStyle(c.Theme(), func(s *kline.Style) {
						if d.alternate {
							s.Up, s.Down = ui.RGB(205, 52, 72), ui.RGB(0, 122, 85)
							s.Buy, s.Sell = s.Up, s.Down
							s.LimitUp, s.LimitDown = ui.RGB(255, 140, 0), ui.RGB(0, 220, 210)
						}
					})
					c.Invalidate()
				}
				if ui.Button(c, "Hollow").Clicked() {
					chart.SetHollow(!chart.Hollow())
					c.Invalidate()
				}
				if ui.Button(c, "Limit colors").Clicked() {
					pct := 0.10
					if chart.LimitPct() > 0 {
						pct = 0
					}
					chart.SetLimitPct(pct)
					c.Invalidate()
				}
				ui.Text(c, fmt.Sprintf("%s | hollow %v | limit %.0f%%", chart.Period(), chart.Hollow(), chart.LimitPct()*100)).FontSize(12)
			})
			message := "1h fixture: B1/B2/B3 share a bar; +10% / -10% candles near the right edge."
			if d.err != nil {
				message = "error: " + d.err.Error()
			}
			ui.Text(c, message).Padding(0, 16, 8, 16).FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Column(c).Grow(1).WidthPercent(100).Children(func() { chart.View(c) })
		})
	}
}

func main() {
	d, err := newTradesDemo()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := d.chart.Close(); err != nil {
			log.Print(err)
		}
	}()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "kline trade markers", Width: 960, Height: 700,
			MinWidth: 640, MinHeight: 460, Content: ui.View(tradesView(d))})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
