// Command replay opens a native, offline blind-playback example.
// Add -headless to drive the same constructor and view without a window.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

const symbol = "600519"
const seekTimeFormat = "2006-01-02 15:04"

var basePeriod = kline.Period{Span: 1, Unit: kline.UnitHour}
var labelZone = time.FixedZone("Asia/Shanghai", 8*60*60)

type replayExample struct {
	chart               *kline.Chart
	source              kline.DataSource
	full                []kline.Candle
	replay              *kline.Replay
	indexText, timeText string
	err                 error
}

func newReplayExample(source kline.DataSource) *replayExample {
	if source == nil {
		source = &kline.SyntheticSource{Extra: -1}
	}
	a := &replayExample{source: source, indexText: "40"}
	a.chart = kline.NewChart(kline.ChartOptions{
		Source: source, Symbols: []string{symbol}, Period: basePeriod,
		Indicators: []string{"MA", "MACD"}, Location: labelZone,
	})
	if a.err = a.chart.LoadError(); a.err != nil {
		return a
	}
	a.loadScenario()
	if a.err == nil {
		a.restart()
	}
	return a
}

func (a *replayExample) loadScenario() {
	a.full, _, a.err = a.source.GetBars(kline.BarRequest{
		Symbol: symbol, Period: basePeriod, Kind: kline.KindInit, Limit: 120,
	})
	if a.err != nil {
		return
	}
	if len(a.full) == 0 {
		a.err = fmt.Errorf("no replay bars")
		return
	}
	a.timeText = a.full[min(39, len(a.full)-1)].Time.In(labelZone).Format(seekTimeFormat)
	// Chart clips preloaded trades to the revealed interval, including
	// future trades that share the current aggregate bucket.
	marks := []kline.Marker{}
	for i, side := range []kline.MarkerSide{kline.SideBuy, kline.SideSell} {
		if j := 4 + i; j < len(a.full) {
			marks = append(marks, kline.Marker{Time: a.full[j].Time, Side: side})
		}
	}
	a.err = a.chart.SetMarkers(symbol, marks)
}

func (a *replayExample) restart() {
	if len(a.full) == 0 {
		a.loadScenario()
		if a.err != nil {
			return
		}
	}
	// StartReplay takes the current period as its base. Reset it before
	// reattaching hourly bars after a 4h/1d viewing session.
	if a.err = a.chart.SetPeriod(basePeriod); a.err != nil {
		return
	}
	next, err := kline.StartReplay(a.chart, map[string][]kline.Candle{symbol: a.full})
	a.err = err
	if err == nil {
		a.replay = next // StartReplay closes the previous replay on success.
	}
}

func (a *replayExample) returnToSource() {
	// A GetBars failure preserves replay. Once the replacement bars load,
	// SetSource has already closed replay, even if its later subscription
	// returns an error. Keep the UI attached to the chart, not a closed replay.
	old := a.replay
	a.err = a.chart.SetSource(a.source)
	// A closed replay refuses Step(0), while one preserved by a GetBars
	// failure still accepts it.
	if old != nil && errors.Is(old.Step(0), kline.ErrReplayClosed) {
		a.replay = nil
	}
}

func (a *replayExample) seekIndex() {
	i, err := strconv.Atoi(a.indexText)
	if err != nil || i < 0 || i > len(a.full) {
		a.err = fmt.Errorf("index must be a revealed count from 0 to %d", len(a.full))
		return
	}
	a.err = a.replay.SeekIndex(i)
}

func (a *replayExample) seekTime() {
	t, err := time.ParseInLocation(seekTimeFormat, a.timeText, labelZone)
	if err != nil {
		a.err = fmt.Errorf("time must be YYYY-MM-DD HH:MM (Asia/Shanghai)")
		return
	}
	a.err = a.replay.SeekTo(t)
}

func (a *replayExample) status() string {
	if a.replay == nil {
		return fmt.Sprintf("Source | %s | %d bars", a.chart.Period(), a.chart.Bars())
	}
	state := "Paused"
	if a.replay.Playing() {
		state = "Playing 2 bars/s"
	}
	return fmt.Sprintf("%s | %d/%d revealed | %s | %d plotted", state,
		a.replay.Cursor(), len(a.full), a.chart.Period(), a.chart.Bars())
}

func (a *replayExample) view(c *ui.Context) {
	ui.Column(c).Fill().Background(c.Theme().Background).Children(func() {
		ui.Row(c).Padding(8, 12, 4, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			active := a.replay != nil
			if ui.Button(c, "Back").Disabled(!active).Clicked() {
				a.err = a.replay.StepBack(1)
			}
			if ui.Button(c, "Step").Disabled(!active).Clicked() {
				a.err = a.replay.Step(1)
			}
			if ui.Button(c, "Play").Disabled(!active).Clicked() {
				a.err = nil
				a.replay.Play(2)
			}
			if ui.Button(c, "Pause").Disabled(!active).Clicked() {
				a.replay.Pause()
			}
			if ui.Button(c, "Restart").Clicked() {
				a.restart()
			}
			if ui.Button(c, "Return to source").Clicked() {
				a.returnToSource()
			}
			for _, period := range []kline.Period{basePeriod, {Span: 4, Unit: kline.UnitHour}, {Span: 1, Unit: kline.UnitDay}} {
				if ui.Button(c, period.String()).Clicked() {
					a.err = a.chart.SetPeriod(period)
				}
			}
		})
		ui.Row(c).Padding(4, 12, 4, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.indexText).Label("Revealed count").Width(70).Disabled(a.replay == nil)
			if ui.Button(c, "Seek index").Disabled(a.replay == nil).Clicked() {
				a.seekIndex()
			}
			ui.TextInput(c, &a.timeText).Label("Seek time (Shanghai)").Width(190).Disabled(a.replay == nil)
			if ui.Button(c, "Seek time").Disabled(a.replay == nil).Clicked() {
				a.seekTime()
			}
			ui.Text(c, "YYYY-MM-DD HH:MM | Asia/Shanghai").FontSize(12)
		})
		if a.err != nil {
			ui.Textf(c, "Error: %v", a.err).Padding(0, 12, 4, 12)
		}
		ui.Column(c).Grow(1).WidthPercent(100).Children(func() { a.chart.View(c) })
		// Chart.View ticks on the UI thread; status reads the same frame.
		ui.Text(c, a.status()).Padding(4, 12, 8, 12).FontSize(12)
	})
}

func (a *replayExample) smoke(w io.Writer) error {
	if a.err != nil {
		return a.err
	}
	tester := ui.NewTester(a.view, 1100, 720)
	fmt.Fprintln(w, "start:", a.status())
	for _, button := range []string{"Seek index", "Step", "Back", "4h", "Seek time", "1d"} {
		if err := tester.Click(button); err != nil {
			return err
		}
		if a.err != nil {
			return a.err
		}
	}
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	a.replay.SetClock(func() time.Time { return now })
	if err := tester.Click("Play"); err != nil {
		return err
	}
	now = now.Add(1500 * time.Millisecond)
	tester.Frame()
	if err := tester.Click("Pause"); err != nil {
		return err
	}
	fmt.Fprintln(w, "playback:", a.status())
	for _, button := range []string{"Restart", "Return to source"} {
		if err := tester.Click(button); err != nil {
			return err
		}
		fmt.Fprintln(w, button+":", a.status())
	}
	return a.err
}

func main() {
	headless := flag.Bool("headless", false, "run the shared view smoke without a window")
	flag.Parse()
	if *headless {
		a := newReplayExample(nil)
		defer a.chart.Close()
		if err := a.smoke(os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	mygo.App.WhenReady(func() {
		a := newReplayExample(nil)
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: "kline blind replay", Width: 1100, Height: 720,
			MinWidth: 1000, MinHeight: 520, Content: ui.View(a.view),
		})
		window.OnClosed(func() { _ = a.chart.Close() })
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
