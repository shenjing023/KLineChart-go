package main

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func historyTester(t *testing.T) (*historyDemo, *ui.Tester) {
	t.Helper()
	d, err := newHistoryDemo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.chart.Close(); err != nil {
			t.Error(err)
		}
	})
	return d, ui.NewTester(historyView(d), 960, 700)
}

func TestHistoryVisibleFailureRetryAndRefresh(t *testing.T) {
	d, tt := historyTester(t)
	if d.chart.Bars() != 0 || d.chart.LoadError() == nil || d.feed.gets != 1 || !tt.HasText("not ready yet") {
		t.Fatalf("initial error was hidden: bars=%d requests=%d texts=%q", d.chart.Bars(), d.feed.gets, tt.Texts())
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	click("Retry")
	if d.err != nil || d.chart.LoadError() != nil || d.chart.Bars() != kline.DefaultLimit || tt.HasText("not ready yet") {
		t.Fatalf("retry: %v / %v, bars=%d", d.err, d.chart.LoadError(), d.chart.Bars())
	}
	first, ok := d.chart.Bar("600519", 0)
	if !ok {
		t.Fatal("first bar missing")
	}
	click("Fail refresh")
	unchanged, ok := d.chart.Bar("600519", 0)
	if !ok || unchanged != first || d.chart.Bars() != kline.DefaultLimit {
		t.Fatal("failed refresh replaced cached chart data")
	}
	// SetSource returns this error without replacing LoadError.
	if d.chart.LoadError() != nil || d.err == nil || !tt.HasText("not ready yet") {
		t.Fatalf("refresh error not displayed: %v; %q", d.err, tt.Texts())
	}
	click("Retry")
	click("Reload")
	if d.err != nil || tt.HasText("not ready yet") || d.feed.gets != 5 {
		t.Fatalf("refresh/retry: %v, requests=%d", d.err, d.feed.gets)
	}
}

func TestHistoryLoadsEveryPageAndStops(t *testing.T) {
	d, tt := historyTester(t)
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	box, ok := tt.Find("600519 chart")
	if !ok {
		t.Fatal("chart missing")
	}
	drag := func() {
		x, y := box.X+2, box.Y+box.H/3
		tt.Press(x, y)
		tt.Move(x+600, y)
		tt.Release(x+600, y)
		tt.Frame()
	}
	for range 60 {
		drag()
	}
	if d.chart.Bars() != len(d.feed.all) || d.chart.Bars() <= kline.DefaultLimit {
		t.Fatalf("pagination stopped at %d of %d", d.chart.Bars(), len(d.feed.all))
	}
	for i, want := range d.feed.all {
		if got, ok := d.chart.Bar("600519", i); !ok || got != want {
			t.Fatalf("bar %d changed across pages: %v / %v", i, got, want)
		}
	}
	gets := d.feed.gets
	for range 3 {
		drag()
	}
	if d.feed.gets != gets {
		t.Fatal("exhausted source was queried again")
	}
}

func TestCacheContract(t *testing.T) {
	p := kline.Period{Span: 1, Unit: kline.UnitHour}
	f, err := newCacheFeed(p)
	if err != nil {
		t.Fatal(err)
	}
	f.failInit = false
	req := kline.BarRequest{Symbol: "600519", Period: p, Kind: kline.KindInit}
	latest, more, err := f.GetBars(req)
	if err != nil || !more || len(latest) != 600 {
		t.Fatalf("init: len=%d more=%v err=%v", len(latest), more, err)
	}
	baseline := append([]kline.Candle(nil), latest...)
	latest[0].Close = -1
	again, _, err := f.GetBars(req)
	if err != nil || !reflect.DeepEqual(again, baseline) {
		t.Fatal("init did not return an independent stable copy")
	}
	// An anchor between opens still returns only strictly older bars.
	req.Kind, req.Anchor, req.Limit = kline.KindForward, f.all[801].Time.Add(-time.Second), 17
	older, more, err := f.GetBars(req)
	if err != nil || !more || !reflect.DeepEqual(older, f.all[784:801]) {
		t.Fatalf("forward window: len=%d more=%v err=%v", len(older), more, err)
	}
	older[0].Close = -1
	if f.all[784].Close == -1 {
		t.Fatal("forward response aliases the cache")
	}
	req.Anchor, req.Limit = f.all[12].Time, 50
	older, more, err = f.GetBars(req)
	if err != nil || more || !reflect.DeepEqual(older, f.all[:12]) {
		t.Fatal("oldest partial page contract failed")
	}
	req.Anchor = f.all[0].Time
	older, more, err = f.GetBars(req)
	if err != nil || more || len(older) != 0 {
		t.Fatal("exhausted cache must return an empty page without more")
	}
	for _, tc := range []struct {
		name string
		edit func(*kline.BarRequest)
		want error
	}{
		{"symbol", func(r *kline.BarRequest) { r.Symbol = "unknown" }, kline.ErrUnknownSymbol},
		{"period", func(r *kline.BarRequest) { r.Period.Span = 4 }, kline.ErrInvalidPeriod},
		{"kind", func(r *kline.BarRequest) { r.Kind = kline.KindBackward }, kline.ErrUnsupportedKind},
		{"anchor", func(r *kline.BarRequest) { r.Anchor = time.Time{} }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := req
			tc.edit(&r)
			_, _, err := f.GetBars(r)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("invalid request returned %v, want %v", err, tc.want)
			}
		})
	}
}
