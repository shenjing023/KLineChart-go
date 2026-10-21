package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func TestHostViewUsesPublicReadout(t *testing.T) {
	chart := newHostChart()
	if err := chart.LoadError(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = chart.Close() })
	tt := ui.NewTester(hostView(chart), 960, 720)
	tt.Frame()
	if !tt.HasText("HOST EMBED") {
		t.Fatalf("host chrome missing, texts %q", tt.Texts())
	}
	box, ok := tt.Find("600519 chart")
	if !ok {
		t.Fatal("chart missing")
	}
	tt.Move(box.X+box.W*0.80, box.Y+box.H/3)
	tt.Frame()
	status := readHostStatus(chart)
	if !strings.Contains(status, "Hover") {
		t.Fatalf("host status %q", status)
	}
	if _, _, ok := chart.Crosshair(); !ok {
		t.Fatal("crosshair did not update")
	}
}

func TestHostStatusFollowsConfiguredFormat(t *testing.T) {
	const layout = "2006-01-02 15:04"
	instant := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	cs := make([]kline.Candle, 10)
	for i := range cs {
		cs[i] = kline.Candle{
			Time: instant.Add(time.Duration(i) * time.Hour),
			Open: 10.25, High: 10.75, Low: 9.75, Close: 10.5, Volume: 100,
		}
	}
	src := hostFixture(cs)
	digits := 4
	zone := time.FixedZone("UTC+8", 8*60*60)
	chart := kline.NewChart(kline.ChartOptions{
		Source: src, Symbols: []string{"600519"}, Indicators: []string{},
		PriceDecimals: &digits, Location: zone,
	})
	t.Cleanup(func() { _ = chart.Close() })
	if err := chart.LoadError(); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(hostView(chart), 960, 720)
	tt.Frame()
	status := readHostStatus(chart)
	if !strings.Contains(status, instant.Add(9*time.Hour).In(zone).Format(layout)) ||
		!strings.Contains(status, "O 10.2500") || !strings.Contains(status, "C 10.5000") {
		t.Fatalf("host status ignores precision/location: %q", status)
	}
	tt.Move(0, 0)
	tt.Frame()
	if got := readHostStatus(chart); !strings.Contains(got, "Last") {
		t.Fatalf("status outside the chart should read the last bar: %q", got)
	}
}

type hostFixture []kline.Candle

func (f hostFixture) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	if req.Symbol != "600519" || req.Kind != kline.KindInit {
		return nil, false, fmt.Errorf("unexpected request: %+v", req)
	}
	return append([]kline.Candle(nil), f...), false, nil
}
