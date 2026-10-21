package main

import (
	"errors"
	"testing"
	"time"

	"github.com/shenjing023/KLineChart-go/kline"
)

// A replay chart holds only the revealed prefix, and the public append
// pair is fenced while the replay owns the series.
func TestReplayExampleContract(t *testing.T) {
	const symbol = "600519"
	full, _, err := (&kline.SyntheticSource{Extra: -1}).GetBars(kline.BarRequest{
		Symbol: symbol, Period: kline.Period{Span: 1, Unit: kline.UnitHour},
		Kind: kline.KindInit, Limit: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	chart := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{symbol},
		Indicators: []string{"MA", "MACD"},
	})
	replay, err := kline.StartReplay(chart, map[string][]kline.Candle{symbol: full})
	if err != nil {
		t.Fatal(err)
	}
	if chart.Bars() != 0 {
		t.Fatalf("StartReplay should reveal nothing, got %d bars", chart.Bars())
	}

	seen := 0
	replay.OnBar(func(kline.ReplayBar) { seen++ })
	if err := replay.SeekIndex(40); err != nil {
		t.Fatal(err)
	}
	if chart.Bars() != 40 || seen != 40 {
		t.Fatalf("bars %d reported %d, want 40 and 40", chart.Bars(), seen)
	}

	// No lookahead: a bar past the cursor is refused.
	if err := chart.AppendBar(symbol, full[60]); !errors.Is(err, kline.ErrReplayActive) {
		t.Fatalf("AppendBar during replay: %v", err)
	}
	if chart.Bars() != 40 {
		t.Fatalf("a refused append changed the bars: %d", chart.Bars())
	}

	// A coarser period aggregates the revealed prefix only.
	if err := chart.SetPeriod(kline.Period{Span: 4, Unit: kline.UnitHour}); err != nil {
		t.Fatal(err)
	}
	if got := chart.Bars(); got != 20 {
		t.Fatalf("4h bars %d, want 20 natural four-hour buckets", got)
	}
	// A finer period would need hidden bars.
	if err := chart.SetPeriod(kline.Period{Span: 15, Unit: kline.UnitMinute}); !errors.Is(err, kline.ErrReplayPeriod) {
		t.Fatalf("finer period: %v", err)
	}

	// Playback is clock-driven and reveals base bars, not 4h bars.
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	replay.SetClock(func() time.Time { return now })
	before := replay.Cursor()
	replay.Play(2)
	now = now.Add(1500 * time.Millisecond)
	replay.Tick()
	if got := replay.Cursor(); got != before+3 {
		t.Fatalf("cursor %d after 1.5s at 2/s, want %d", got, before+3)
	}
	if !replay.Playing() {
		t.Fatal("playback should still be running")
	}

	replay.Pause()
	replay.Close()
	if chart.Bars() == 0 {
		t.Fatal("Close should keep the revealed bars")
	}
	if err := replay.Step(1); !errors.Is(err, kline.ErrReplayClosed) {
		t.Fatalf("step after close: %v", err)
	}
}
