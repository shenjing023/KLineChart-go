package kline

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Paint and PaintCompare are the entry points a host calls when it draws
// its own chrome. They take no labelFmt, so they must stay at two
// decimals and UTC even when the chart was configured otherwise.
func TestPaintUsesDefaultLabels(t *testing.T) {
	digits := 4
	zone := time.FixedZone("UTC+8", 8*3600)
	labels := defaultLabels()
	if labels.digits != 2 {
		t.Fatalf("default digits %d, want 2", labels.digits)
	}
	if labels.loc != nil {
		t.Fatalf("default location %v, want nil (UTC)", labels.loc)
	}
	if got := labels.price(1.23456); got != "1.23" {
		t.Fatalf("default price %q, want 1.23", got)
	}
	bar := Candle{Time: time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)}
	if got := labels.when(bar.Time, "15:04"); got != "09:30" {
		t.Fatalf("default time %q, want 09:30 UTC", got)
	}

	// The chart's configured labels are not the defaults.
	c := NewChart(ChartOptions{
		Symbols:       []string{"600519", "000001"},
		Indicators:    []string{"MA"},
		PriceDecimals: &digits,
		Location:      zone,
	})
	if got := c.labels(); got.digits != 4 || got.loc != zone {
		t.Fatalf("chart labels %d %v, want 4 and %v", got.digits, got.loc, zone)
	}
	if got := c.labels().when(bar.Time, "15:04"); got != "17:30" {
		t.Fatalf("chart time %q, want 17:30 in UTC+8", got)
	}
	// Paint still formats with the defaults, not the chart's.
	if got := defaultLabels().when(bar.Time, "15:04"); got != "09:30" {
		t.Fatalf("default time changed with the chart: %q", got)
	}
	if got := FormatBar(bar, math.NaN(), math.NaN()); got[0:16] != bar.Time.UTC().Format("2006-01-02 15:04") {
		t.Fatalf("FormatBar uses a non-UTC time: %q", got)
	}
}

func TestPaintRendersPanes(t *testing.T) {
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519", "000001"},
		Indicators: []string{"MA", "MACD"},
	})
	primary, compare := c.overlayPair()
	inds := c.active(&c.panes[0])
	panes := SplitNoTime(ui.Rect{X: 0, Y: 0, W: 900, H: 600})
	ptr := Pointer{On: true, X: 400, Y: 200}
	for _, tc := range []struct {
		name string
		draw func(p *ui.Painter, r ui.Rect)
	}{
		{"Paint", func(p *ui.Painter, r ui.Rect) {
			Paint(p, panes, primary, inds, c.vp, ptr, 100, DarkDefault())
		}},
		{"PaintCompare", func(p *ui.Painter, r ui.Rect) {
			if !PaintCompare(p, panes, primary, compare, inds, c.vp, ptr, 100, DarkDefault()) {
				t.Error("PaintCompare returned false on a positive series")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tt := ui.NewTester(func(ctx *ui.Context) {
				ui.Column(ctx).Fill().Children(func() {
					ui.Box(ctx).Grow(1).Draw(tc.draw)
				})
			}, 960, 720)
			tt.Frame()
			if !frameHasInk(tt.Image()) {
				t.Fatalf("%s painted nothing", tc.name)
			}
		})
	}

	// A tiny rect draws nothing instead of panicking.
	small := ui.NewTester(func(ctx *ui.Context) {
		ui.Column(ctx).Fill().Children(func() {
			ui.Box(ctx).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
				Paint(p, Panes{Price: ui.Rect{W: 1, H: 1}}, primary, inds, c.vp, ptr, 100, DarkDefault())
				PaintCompare(p, Panes{Price: ui.Rect{W: 1, H: 1}}, primary, compare, inds, c.vp, ptr, 100, DarkDefault())
			})
		})
	}, 40, 40)
	small.Frame()
}

func TestHollowEnabledBinding(t *testing.T) {
	c := NewChart(ChartOptions{Indicators: []string{}})
	flag := c.HollowEnabled()
	if flag == nil {
		t.Fatal("HollowEnabled returned nil")
	}
	if *flag {
		t.Fatal("hollow starts off")
	}
	*flag = true // a ui.Toggle writes through the pointer
	if !c.Hollow() {
		t.Fatal("writing the flag did not flip the chart")
	}
	c.SetHollow(false)
	if *flag {
		t.Fatal("SetHollow did not reach the same bool")
	}
}

func TestReplaySpeedAndLayerClear(t *testing.T) {
	c, _ := replayChart(t, []string{"600519"}, map[string][]Candle{"600519": Generate(20, 1)}, LayoutStack)
	replay, err := StartReplay(c, map[string][]Candle{"600519": Generate(20, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if got := replay.Speed(); got != 0 {
		t.Fatalf("speed before Play %v", got)
	}
	replay.Play(4)
	if got := replay.Speed(); got != 4 {
		t.Fatalf("speed %v, want 4", got)
	}
	var nilReplay *Replay
	if got := nilReplay.Speed(); got != 0 {
		t.Fatalf("nil replay speed %v", got)
	}

	var layer Layer
	layer.Add(OverlayHLine, Point{Price: 1})
	layer.Add(OverlayHLine, Point{Price: 2})
	if len(layer.Items()) != 2 {
		t.Fatalf("items %d", len(layer.Items()))
	}
	layer.Clear()
	if len(layer.Items()) != 0 || layer.Selected() != nil {
		t.Fatalf("Clear left items %d selected %v", len(layer.Items()), layer.Selected())
	}
	// Add after Clear still works and keeps its id sequence.
	o := layer.Add(OverlayHLine, Point{Price: 3})
	if o.ID == "" || len(layer.Items()) != 1 {
		t.Fatalf("add after clear: %+v", o)
	}
	var nilLayer *Layer
	nilLayer.Clear() // must not panic
}
