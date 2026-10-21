package kline

import (
	"errors"
	"testing"
	"time"
)

func TestSyntheticHourlyInitUsesAshareSessions(t *testing.T) {
	src := &SyntheticSource{}
	period := Period{Span: 1, Unit: UnitHour}
	bars, more, err := src.GetBars(BarRequest{Symbol: "600519", Period: period, Kind: KindInit, Limit: 600})
	if err != nil {
		t.Fatal(err)
	}
	if !more {
		t.Fatal("hourly init should leave older bars")
	}
	if len(bars) != 600 {
		t.Fatalf("600519 init len %d", len(bars))
	}
	beta, _, err := src.GetBars(BarRequest{Symbol: "000001", Period: period, Kind: KindInit, Limit: 600})
	if err != nil {
		t.Fatal(err)
	}
	if bars[10].Close == beta[10].Close {
		t.Fatal("600519 and 000001 produced the same close")
	}
	if first := bars[0].Time.In(ashareLocation); first.Location() != ashareLocation {
		t.Fatalf("hourly time zone %v", first.Location())
	}
	for i := 1; i < len(bars); i++ {
		if !bars[i].Time.After(bars[i-1].Time) {
			t.Fatalf("bar %d is not after %d", i, i-1)
		}
		got := bars[i].Time.In(ashareLocation)
		if got.Weekday() == time.Saturday || got.Weekday() == time.Sunday {
			t.Fatalf("weekend bar %s", got)
		}
		clock := got.Format("15:04")
		switch clock {
		case "09:30", "10:30", "13:00", "14:00":
		default:
			t.Fatalf("bar %d opens at %s, want an A-share session bucket", i, clock)
		}
		prev := bars[i-1].Time.In(ashareLocation)
		if prev.Format("15:04") == "10:30" && got.Format("15:04") != "13:00" {
			t.Fatalf("lunch break joined without a session gap: %s -> %s", prev, got)
		}
	}
}

func TestSyntheticSourceUsesAshareDayAndCoarseBuckets(t *testing.T) {
	src := &SyntheticSource{}
	for _, tc := range []struct {
		period Period
		n      int
		want   string
	}{
		{Period{Span: 15, Unit: UnitMinute}, 600, "14:45"},
		{Period{Span: 1, Unit: UnitHour}, 600, "14:00"},
		{Period{Span: 4, Unit: UnitHour}, 600, "09:30"},
		{Period{Span: 1, Unit: UnitDay}, 600, "09:30"},
	} {
		bars, _, err := src.GetBars(BarRequest{Symbol: "600519", Period: tc.period, Kind: KindInit, Limit: tc.n})
		if err != nil {
			t.Fatal(err)
		}
		if len(bars) == 0 {
			t.Fatalf("%s returned no bars", tc.period)
		}
		if got := bars[len(bars)-1].Time.In(ashareLocation).Format("15:04"); got != tc.want {
			t.Fatalf("%s newest open %s, want %s", tc.period, got, tc.want)
		}
		for i := 1; i < len(bars); i++ {
			if !bars[i].Time.After(bars[i-1].Time) {
				t.Fatalf("%s bar %d is not after %d", tc.period, i, i-1)
			}
			if bars[i].Time.Sub(bars[i-1].Time) < time.Minute {
				t.Fatalf("%s duplicate/near-zero step at %d", tc.period, i)
			}
		}
	}
}

func TestSyntheticForwardBeforeAnchorNoOverlap(t *testing.T) {
	src := &SyntheticSource{}
	period := Period{Span: 4, Unit: UnitHour}
	init, _, err := src.GetBars(BarRequest{Symbol: "000001", Period: period, Kind: KindInit, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	anchor := init[0].Time
	fwd, more, err := src.GetBars(BarRequest{
		Symbol: "000001", Period: period, Kind: KindForward, Anchor: anchor, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fwd) != 20 || !more {
		t.Fatalf("forward len %d more %v", len(fwd), more)
	}
	if !fwd[len(fwd)-1].Time.Before(anchor) {
		t.Fatalf("last forward %s is not before %s", fwd[len(fwd)-1].Time, anchor)
	}
	seen := map[time.Time]bool{}
	for _, b := range init {
		seen[b.Time] = true
	}
	for i, b := range fwd {
		if seen[b.Time] {
			t.Fatalf("overlap at %s", b.Time)
		}
		if i > 0 {
			prev := fwd[i-1].Time.In(ashareLocation)
			cur := b.Time.In(ashareLocation)
			if cur.Before(prev) || (cur.Day() == prev.Day() && cur.Sub(prev) < 4*time.Hour && cur.Format("15:04") == "09:30") {
				t.Fatalf("forward order %s -> %s", prev, cur)
			}
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
		_, _, err := src.GetBars(BarRequest{Symbol: "600519", Period: p, Kind: KindInit, Limit: 10})
		if !errors.Is(err, ErrInvalidPeriod) {
			t.Fatalf("period %+v err %v", p, err)
		}
	}
	_, _, err := src.GetBars(BarRequest{
		Symbol: "600519", Period: Period{Span: 1, Unit: UnitHour}, Kind: KindBackward, Limit: 10,
	})
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("backward err %v", err)
	}
}

func TestSyntheticForwardExhaustsHistory(t *testing.T) {
	src := &SyntheticSource{Extra: 30}
	period := Period{Span: 1, Unit: UnitHour}
	bars, more, err := src.GetBars(BarRequest{Symbol: "600519", Period: period, Kind: KindInit, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 20 || !more {
		t.Fatalf("init len %d more %v", len(bars), more)
	}
	total := len(bars)
	for more {
		older, next, err := src.GetBars(BarRequest{
			Symbol: "600519", Period: period, Kind: KindForward, Anchor: bars[0].Time, Limit: 20,
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
