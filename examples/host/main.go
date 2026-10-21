// Command host is the smallest embeddable chart with the host's own
// header. It keeps ChartOptions.HideHeader on and reads Crosshair, Bar,
// and Readout on every frame, which is the path a host uses when it
// already has a toolbar and status bar.
//
//	go run ./examples/host
package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

// hostView is shared by main and its headless test.
func hostView(chart *kline.Chart) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
			ui.Row(c).Padding(10, 16, 2, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "HOST EMBED").Bold().FontSize(15)
				ui.Text(c, "600519").FontSize(12).TextColor(c.Theme().TextMuted)
			})
			ui.Text(c, readHostStatus(chart)).Padding(0, 16, 8, 16).
				Font("monospace").FontSize(12).NoWrap().TextColor(c.Theme().TextMuted)
			ui.Column(c).Grow(1).WidthPercent(100).Children(func() {
				chart.View(c)
			})
		})
	}
}

// readHostStatus is the host's own replacement for Chart's header. It
// uses the public pointer and readout, not internal fields. Readout
// already starts with the bar time, so the host adds no duplicate.
func readHostStatus(chart *kline.Chart) string {
	symbol, index, ok := chart.Crosshair()
	if !ok {
		symbol = "600519"
		index = chart.Bars() - 1
	}
	_, ok = chart.Bar(symbol, index)
	if !ok {
		return "No bars"
	}
	return chart.Readout(symbol)
}

func newHostChart() *kline.Chart {
	digits := 2
	zone, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		zone = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return kline.NewChart(kline.ChartOptions{
		Symbols:       []string{"600519"},
		Indicators:    []string{"MA", "MACD"},
		HideHeader:    true,
		PriceDecimals: &digits,
		Location:      zone,
	})
}

func main() {
	chart := newHostChart()
	if err := chart.LoadError(); err != nil {
		log.Fatal(err)
	}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "kline host embed",
			Width:     960,
			Height:    720,
			MinWidth:  640,
			MinHeight: 480,
			Content:   ui.View(hostView(chart)),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
