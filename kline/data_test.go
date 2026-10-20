package kline

import (
	"errors"
	"testing"
	"time"
)

func TestSyntheticHourlyInitMatchesGenerate(t *testing.T) {
	src := &SyntheticSource{}
	period := Period{Span: 1, Unit: UnitHour}
	bars, more, err := src.GetBars(BarRequest{Symbol: "ALPHA", Period: period, Kind: KindInit, Limit: 600})
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("hourly init should leave older bars")
	}
	want := Generate(600, 42)
	if len(bars) != len(want) || bars[0] != want[0] || bars[len(bars)-1] != want[len(bars)-1] {
		t.Fatalf("ALPHA init drifted from Generate(600, 42)")
	}
	beta, _, err := src.GetBars(BarRequest{Symbol: "BETA", Period: period, Kind: KindInit, Limit: 600})
	if err != nil {
		t.Fatal(err)
	}
	other := Generate(600, 99)
	if beta[0] != other[0] || beta[599] != other[599] {
		t.Fatal("BETA init drifted from Generate(600, 99)")
	}
	if bars[10].Close == beta[10].Close {
		t.Fatal("ALPHA and BETA produced the same close")
	}
	for i := 1; i < len(bars); i++ {
		if bars[i].Time.Sub(bars[i-1].Time) != time.Hour {
			t.Fatalf("step %s", bars[i].Time.Sub(bars[i-1].Time))
		}
	}
}

func TestSyntheticForwardBeforeAnchorNoOverlap(t *testing.T) {
	src := &SyntheticSource{}
	period := Period{Span: 4, Unit: UnitHour}
	init, _, err := src.GetBars(BarRequest{Symbol: "BETA", Period: period, Kind: KindInit, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	anchor := init[0].Time
	fwd, more, err := src.GetBars(BarRequest{
		Symbol: "BETA", Period: period, Kind: KindForward, Anchor: anchor, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fwd) != 20 || !more {
		t.Fatalf("forward len %d more %v", len(fwd), more)
	}
	step := 4 * time.Hour
	if !fwd[len(fwd)-1].Time.Before(anchor) {
		t.Fatalf("last forward %s is not before %s", fwd[len(fwd)-1].Time, anchor)
	}
	if !fwd[len(fwd)-1].Time.Add(step).Equal(anchor) {
		t.Fatalf("forward does not meet anchor: last %s anchor %s", fwd[len(fwd)-1].Time, anchor)
	}
	seen := map[time.Time]bool{}
	for _, b := range init {
		seen[b.Time] = true
	}
	for i, b := range fwd {
		if seen[b.Time] {
			t.Fatalf("overlap at %s", b.Time)
		}
		if i > 0 && b.Time.Sub(fwd[i-1].Time) != step {
			t.Fatalf("forward step %s", b.Time.Sub(fwd[i-1].Time))
		}
	}
}

func TestSyntheticInvalidPeriod(t *testing.T) {
	src := &SyntheticSource{}
	cases := []Period{
		{Span: 0, Unit: UnitHour},
		{Span: -1, Unit: UnitDay},
		{Span: 1, Unit: "week"},
		{Span: 1, Unit: ""},
	}
	for _, p := range cases {
		_, _, err := src.GetBars(BarRequest{Symbol: "ALPHA", Period: p, Kind: KindInit, Limit: 10})
		if !errors.Is(err, ErrInvalidPeriod) {
			t.Fatalf("period %+v err %v", p, err)
		}
	}
	_, _, err := src.GetBars(BarRequest{
		Symbol: "ALPHA", Period: Period{Span: 1, Unit: UnitHour}, Kind: KindBackward, Limit: 10,
	})
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("backward err %v", err)
	}
}

func TestSyntheticForwardExhaustsHistory(t *testing.T) {
	src := &SyntheticSource{Extra: 30}
	period := Period{Span: 1, Unit: UnitHour}
	bars, more, err := src.GetBars(BarRequest{Symbol: "ALPHA", Period: period, Kind: KindInit, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 20 || !more {
		t.Fatalf("init len %d more %v", len(bars), more)
	}
	total := len(bars)
	for more {
		older, next, err := src.GetBars(BarRequest{
			Symbol: "ALPHA", Period: period, Kind: KindForward, Anchor: bars[0].Time, Limit: 20,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(older) == 0 {
			t.Fatal("more was true but forward returned nothing")
		}
		if !older[len(older)-1].Time.Before(bars[0].Time) {
			t.Fatal("forward overlapped")
		}
		bars = append(older, bars...)
		total += len(older)
		more = next
	}
	if total != 50 {
		t.Fatalf("walked %d bars, want 50", total)
	}
}
