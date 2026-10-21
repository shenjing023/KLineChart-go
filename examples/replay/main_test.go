package main

import (
	"bytes"
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

// A replay chart holds only the revealed prefix, and the public append
// pair is fenced while the replay owns the series.
func TestReplayExampleContract(t *testing.T) {
	a := newReplayExample(nil)
	t.Cleanup(func() { _ = a.chart.Close() })
	if a.err != nil {
		t.Fatal(a.err)
	}
	chart, replay, full := a.chart, a.replay, a.full
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

func TestNativeReplayControls(t *testing.T) {
	src := &testSource{}
	a := newReplayExample(src)
	t.Cleanup(func() { _ = a.chart.Close() })
	if a.err != nil {
		t.Fatal(a.err)
	}
	tt := ui.NewTester(a.view, 1100, 720)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		if a.err != nil {
			t.Fatal(a.err)
		}
	}
	if a.chart.Bars() != 0 || !tt.HasText("0/120 revealed") {
		t.Fatal("new replay must start empty")
	}
	seen := 0
	a.replay.OnBar(func(kline.ReplayBar) { seen++ })
	click("Step")
	if a.replay.Cursor() != 1 || seen != 1 {
		t.Fatal("Step did not reveal/report one bar")
	}
	click("Back")
	if a.chart.Bars() != 0 || seen != 1 {
		t.Fatal("Back did not hide the bar, or fired OnBar")
	}
	enter(t, tt, "Revealed count", "40")
	click("Seek index")
	if a.chart.Bars() != 40 {
		t.Fatalf("index input did not seek: %d", a.chart.Bars())
	}
	calls := src.calls
	click("4h")
	if a.chart.Bars() != 20 {
		t.Fatalf("4h: got %d", a.chart.Bars())
	}
	click("1d")
	if a.chart.Bars() != 10 {
		t.Fatalf("1d: got %d", a.chart.Bars())
	}
	click("1h")
	if a.chart.Bars() != 40 {
		t.Fatal("1h did not restore the revealed prefix")
	}
	enter(t, tt, "Seek time (Shanghai)", a.full[6].Time.In(labelZone).Format(seekTimeFormat))
	click("Seek time")
	if a.replay.Cursor() != 7 || a.chart.Bars() != 7 {
		t.Fatal("time seek must include the matching bar")
	}
	enter(t, tt, "Seek time (Shanghai)", a.full[0].Time.Add(-time.Hour).In(labelZone).Format(seekTimeFormat))
	click("Seek time")
	if a.replay.Cursor() != 0 || a.chart.Bars() != 0 {
		t.Fatal("time before the series must reveal nothing")
	}
	enter(t, tt, "Seek time (Shanghai)", a.full[len(a.full)-1].Time.Add(time.Hour).In(labelZone).Format(seekTimeFormat))
	click("Seek time")
	if a.replay.Cursor() != len(a.full) {
		t.Fatal("time after the series must reveal every base bar")
	}
	enter(t, tt, "Revealed count", "7")
	click("Seek index")
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	a.replay.SetClock(func() time.Time { return now })
	click("Play")
	now = now.Add(1500 * time.Millisecond)
	tt.Frame() // Chart.View, not the test, calls Tick.
	if a.replay.Cursor() != 10 || !a.replay.Playing() || !tt.HasText("10/120 revealed") {
		t.Fatalf("view did not tick playback: %s", a.status())
	}
	click("Pause")
	now = now.Add(time.Hour)
	tt.Frame()
	if a.replay.Cursor() != 10 || a.replay.Playing() {
		t.Fatal("Pause did not stop frame playback")
	}
	if src.calls != calls {
		t.Fatal("replay controls fetched source bars")
	}
	click("1d")
	oldReplay := a.replay
	click("Restart")
	if a.chart.Period() != basePeriod || a.replay.Cursor() != 0 || a.chart.Bars() != 0 {
		t.Fatal("Restart must reset both base period and prefix")
	}
	if err := oldReplay.Step(1); !errors.Is(err, kline.ErrReplayClosed) {
		t.Fatalf("Restart left the old replay open: %v", err)
	}
	click("Step")
	first, ok := a.chart.Bar(symbol, 0)
	if !ok || first != a.full[0] {
		t.Fatal("Restart did not reattach the hourly base series")
	}
	oldReplay = a.replay
	click("Return to source")
	if a.replay != nil || a.chart.Bars() != kline.DefaultLimit || !tt.HasText("Source | 1h") {
		t.Fatal("source window was not restored")
	}
	last, ok := a.chart.Bar(symbol, a.chart.Bars()-1)
	if !ok || last != a.full[len(a.full)-1] {
		t.Fatal("source restoration omitted the unrevealed suffix")
	}
	if err := oldReplay.Step(1); !errors.Is(err, kline.ErrReplayClosed) {
		t.Fatalf("SetSource did not close replay: %v", err)
	}
	click("Restart")
	if a.replay == nil || a.chart.Bars() != 0 {
		t.Fatal("cannot restart after returning to source")
	}
	enter(t, tt, "Revealed count", "119")
	click("Seek index")
	a.replay.SetClock(func() time.Time { return now })
	click("Play")
	now = now.Add(time.Second)
	tt.Frame()
	if a.replay.Cursor() != 120 || a.replay.Playing() {
		t.Fatal("playback did not stop at the end")
	}
}

func TestFutureMarkersClippedInSharedView(t *testing.T) {
	for _, period := range []string{"1h", "4h", "1d"} {
		t.Run(period, func(t *testing.T) {
			a := newReplayExample(nil)
			t.Cleanup(func() { _ = a.chart.Close() })
			if a.err != nil {
				t.Fatal(a.err)
			}
			tt := ui.NewTester(a.view, 1100, 720)
			marks, err := a.chart.Markers(symbol)
			if err != nil || len(marks) != 2 {
				t.Fatalf("scenario markers: %v %v", marks, err)
			}
			if err := tt.Click(period); err != nil {
				t.Fatal(err)
			}
			// Reveals marker 0, but marker 1 is still hidden. On 4h/1d
			// both trades fall into the last visible aggregate bucket.
			if err := a.replay.SeekIndex(5); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			box, ok := tt.Find("600519 chart")
			if !ok {
				t.Fatal("missing chart")
			}
			plot := image.Rect(int(box.X), int(box.Y), int(box.X+box.W), int(box.Y+box.H))
			before := copyRegion(tt.Image(), plot)
			if err := a.chart.SetMarkers(symbol, marks[:1]); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if !bytes.Equal(before, copyRegion(tt.Image(), plot)) {
				t.Fatal("future marker changed actual chart ink")
			}
			if err := a.chart.SetMarkers(symbol, nil); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if bytes.Equal(before, copyRegion(tt.Image(), plot)) {
				t.Fatal("revealed marker produced no ink")
			}
			if err := a.chart.SetMarkers(symbol, marks); err != nil {
				t.Fatal(err)
			}
			if err := a.replay.Step(1); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			revealed := copyRegion(tt.Image(), plot)
			if err := a.chart.SetMarkers(symbol, marks[:1]); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if bytes.Equal(revealed, copyRegion(tt.Image(), plot)) {
				t.Fatal("second marker did not appear when revealed")
			}
			if err := a.chart.SetMarkers(symbol, marks); err != nil {
				t.Fatal(err)
			}
			if err := a.replay.StepBack(1); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if !bytes.Equal(before, copyRegion(tt.Image(), plot)) {
				t.Fatal("Back did not hide the future marker again")
			}
		})
	}
}

func TestReplayErrorsRecoverInView(t *testing.T) {
	src := &testSource{}
	a := newReplayExample(src)
	t.Cleanup(func() { _ = a.chart.Close() })
	tt := ui.NewTester(a.view, 1100, 720)
	for _, bad := range []string{"not a count", "-1", "121"} {
		enter(t, tt, "Revealed count", bad)
		if err := tt.Click("Seek index"); err != nil {
			t.Fatal(err)
		}
		if a.err == nil || a.replay.Cursor() != 0 || !tt.HasText("Error:") {
			t.Fatal("invalid index must show an error without moving replay")
		}
	}
	enter(t, tt, "Seek time (Shanghai)", "bad time")
	if err := tt.Click("Seek time"); err != nil {
		t.Fatal(err)
	}
	if a.err == nil || !tt.HasText("time must be") {
		t.Fatal("invalid time did not show an error")
	}
	enter(t, tt, "Revealed count", "5")
	if err := tt.Click("Seek index"); err != nil {
		t.Fatal(err)
	}
	if a.err != nil || tt.HasText("Error:") || a.chart.Bars() != 5 {
		t.Fatal("valid seek did not clear the error")
	}
	src.fail = true
	old := a.replay
	if err := tt.Click("Return to source"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(a.err, errSource) || !tt.HasText("source unavailable") || a.replay != old || a.chart.Bars() != 5 {
		t.Fatal("failed restore did not preserve the replay and expose the error")
	}
	src.fail = false
	if err := tt.Click("Return to source"); err != nil {
		t.Fatal(err)
	}
	if a.err != nil || a.replay != nil || a.chart.Bars() != kline.DefaultLimit {
		t.Fatal("retry did not restore source")
	}
}

func TestReturnToSourceAfterSubscriptionFailureDetachesClosedReplay(t *testing.T) {
	src := &liveTestSource{testSource: &testSource{}}
	a := newReplayExample(src)
	t.Cleanup(func() { _ = a.chart.Close() })
	if a.err != nil {
		t.Fatal(a.err)
	}
	tt := ui.NewTester(a.view, 1100, 720)
	if err := a.replay.SeekIndex(5); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	old := a.replay
	src.failSubscribe = true
	if err := tt.Click("Return to source"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !errors.Is(a.err, errSubscribe) {
		t.Fatalf("subscription error was hidden: %v", a.err)
	}
	if a.replay != nil {
		t.Fatal("replay pointer still refers to a replay SetSource closed")
	}
	if a.chart.Bars() != kline.DefaultLimit || !tt.HasText("Source | 1h") {
		t.Fatalf("loaded source bars or status missing: %d %q", a.chart.Bars(), tt.Texts())
	}
	if err := old.Step(1); !errors.Is(err, kline.ErrReplayClosed) {
		t.Fatalf("old replay was not closed: %v", err)
	}
}

func TestFailedConstructionCanRestart(t *testing.T) {
	src := &testSource{fail: true}
	a := newReplayExample(src)
	t.Cleanup(func() { _ = a.chart.Close() })
	tt := ui.NewTester(a.view, 1100, 720)
	if !tt.HasText("Error:") || a.replay != nil {
		t.Fatal("initial load error was not shown")
	}
	src.fail = false
	if err := tt.Click("Restart"); err != nil {
		t.Fatal(err)
	}
	if a.err != nil || a.replay == nil || a.chart.Bars() != 0 || len(a.full) != 120 {
		t.Fatalf("restart did not retry initial scenario: %v", a.err)
	}
}

func TestHeadlessSmokeUsesNativeView(t *testing.T) {
	a := newReplayExample(nil)
	t.Cleanup(func() { _ = a.chart.Close() })
	var out bytes.Buffer
	if err := a.smoke(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "43/120 revealed") || a.replay != nil || a.chart.Bars() != kline.DefaultLimit {
		t.Fatalf("smoke: %s", out.String())
	}
}

var errSource = errors.New("source unavailable")
var errSubscribe = errors.New("subscription unavailable")

type liveTestSource struct {
	*testSource
	failSubscribe bool
}

func (s *liveTestSource) SubscribeBar(string, kline.Period, kline.BarHandler) error {
	if s.failSubscribe {
		return errSubscribe
	}
	return nil
}

func (s *liveTestSource) UnsubscribeBar(string, kline.Period) error { return nil }

type testSource struct {
	fail  bool
	calls int
}

func (s *testSource) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	s.calls++
	if s.fail {
		return nil, false, errSource
	}
	return (&kline.SyntheticSource{Extra: -1}).GetBars(req)
}

func enter(t *testing.T, tt *ui.Tester, label, value string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(value)
}

func copyRegion(img *image.RGBA, r image.Rectangle) []byte {
	out := make([]byte, 0, r.Dx()*r.Dy()*4)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		i := img.PixOffset(r.Min.X, y)
		out = append(out, img.Pix[i:i+r.Dx()*4]...)
	}
	return out
}
