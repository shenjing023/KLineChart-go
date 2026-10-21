// Command replay is a runnable example of blind playback: the chart
// holds only the bars the cursor has revealed, so nothing on screen can
// depend on a bar the viewer has not seen.
//
//	go run ./examples/replay
//
// It plays headlessly here so the example doubles as a smoke test; a
// window would pass chart.View to ui.View.
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func main() {
	const symbol = "600519"
	src := &kline.SyntheticSource{Extra: -1}
	full, _, err := src.GetBars(kline.BarRequest{
		Symbol: symbol, Period: kline.Period{Span: 1, Unit: kline.UnitHour},
		Kind: kline.KindInit, Limit: 120,
	})
	if err != nil {
		log.Fatal(err)
	}

	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{symbol},
		Indicators: []string{"MA", "MACD"},
	})
	if err := chart.LoadError(); err != nil {
		log.Fatal(err)
	}

	// StartReplay copies the series and shows none of it. The chart's
	// panes are empty until the first Step or Seek.
	replay, err := kline.StartReplay(chart, map[string][]kline.Candle{symbol: full})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("bars after StartReplay:", chart.Bars())

	// OnBar runs once per base bar that becomes visible, on the caller.
	// This is where a host records a trade, with no future bar on screen.
	trades := 0
	replay.OnBar(func(ev kline.ReplayBar) {
		trades++
	})

	if err := replay.SeekIndex(40); err != nil {
		log.Fatal(err)
	}
	fmt.Println("bars after SeekIndex(40):", chart.Bars())
	fmt.Println("bars reported:", trades)

	// A live bar cannot slip in ahead of the cursor.
	err = chart.AppendBar(symbol, full[60])
	fmt.Println("AppendBar during replay:", err)

	// A coarser period aggregates only the revealed prefix, and does not
	// ask the source for more.
	if err := chart.SetPeriod(kline.Period{Span: 4, Unit: kline.UnitHour}); err != nil {
		log.Fatal(err)
	}
	fmt.Println("4h bars over the 40 revealed:", chart.Bars())
	// A finer period than the series would need bars the cursor has not
	// reached, so it is refused.
	err = chart.SetPeriod(kline.Period{Span: 15, Unit: kline.UnitMinute})
	fmt.Println("finer period:", err)

	// Playback is frame-driven: no goroutine. A clock makes it
	// deterministic.
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	replay.SetClock(func() time.Time { return now })
	replay.Play(2) // two base bars per second
	now = now.Add(1500 * time.Millisecond)
	replay.Tick()
	fmt.Println("bars after 1.5s at 2/s:", chart.Bars())
	fmt.Println("still playing:", replay.Playing())

	// Paint one frame, as a window would every frame.
	tester := ui.NewTester(chart.View, 900, 600)
	tester.Frame()
	fmt.Println("painted:", tester.HasText("600519"))

	replay.Pause()
	// Close keeps the revealed bars and detaches the replay.
	replay.Close()
	fmt.Println("bars after Close:", chart.Bars())
	fmt.Println("step after Close:", replay.Step(1))
}
