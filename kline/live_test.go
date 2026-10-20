package kline

import (
	"errors"
	"testing"
	"time"
)

func TestSyntheticSourceIsNotLive(t *testing.T) {
	var src DataSource = &SyntheticSource{}
	if _, ok := src.(LiveSource); ok {
		t.Fatal("SyntheticSource implements LiveSource; it must stay historical")
	}
}

func TestStubLivePushReachesHandlerUntilUnsubscribe(t *testing.T) {
	src := &StubLiveSource{}
	period := Period{Span: 1, Unit: UnitHour}
	bar := Candle{
		Time:   time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC),
		Open:   10,
		High:   12,
		Low:    9,
		Close:  11,
		Volume: 100,
	}
	var got []Candle
	if err := src.SubscribeBar("ALPHA", period, func(c Candle) {
		got = append(got, c)
	}); err != nil {
		t.Fatal(err)
	}
	src.Push("ALPHA", period, bar)
	if len(got) != 1 || got[0] != bar {
		t.Fatalf("handler got %+v, want one copy of the pushed bar", got)
	}
	if err := src.UnsubscribeBar("ALPHA", period); err != nil {
		t.Fatal(err)
	}
	src.Push("ALPHA", period, bar)
	if len(got) != 1 {
		t.Fatalf("handler calls after unsubscribe: %d", len(got))
	}
}

func TestStubLivePushWithoutSubscriber(t *testing.T) {
	src := &StubLiveSource{}
	period := Period{Span: 1, Unit: UnitHour}
	src.Push("ALPHA", period, Candle{Close: 1})
	bars, more, err := src.GetBars(BarRequest{Symbol: "ALPHA", Period: period, Kind: KindInit, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if more || len(bars) != 1 || bars[0].Close != 1 {
		t.Fatalf("stored %+v more %v", bars, more)
	}
}

func TestStubLiveDeliversOnlyMatchingSubscription(t *testing.T) {
	src := &StubLiveSource{}
	hour := Period{Span: 1, Unit: UnitHour}
	day := Period{Span: 1, Unit: UnitDay}
	var alpha, beta, daily int
	if err := src.SubscribeBar("ALPHA", hour, func(Candle) { alpha++ }); err != nil {
		t.Fatal(err)
	}
	if err := src.SubscribeBar("BETA", hour, func(Candle) { beta++ }); err != nil {
		t.Fatal(err)
	}
	src.Push("ALPHA", hour, Candle{Close: 1})
	src.Push("ALPHA", day, Candle{Close: 2})
	src.Push("BETA", hour, Candle{Close: 3})
	if alpha != 1 || beta != 1 || daily != 0 {
		t.Fatalf("alpha %d beta %d daily %d", alpha, beta, daily)
	}
}

func TestStubLiveSubscribeReplacesHandler(t *testing.T) {
	src := &StubLiveSource{}
	period := Period{Span: 1, Unit: UnitHour}
	var first, second int
	if err := src.SubscribeBar("ALPHA", period, func(Candle) { first++ }); err != nil {
		t.Fatal(err)
	}
	if err := src.SubscribeBar("ALPHA", period, func(Candle) { second++ }); err != nil {
		t.Fatal(err)
	}
	src.Push("ALPHA", period, Candle{Close: 1})
	if first != 0 || second != 1 {
		t.Fatalf("first %d second %d", first, second)
	}
}

func TestStubLiveSubscribeRejectsBadArgs(t *testing.T) {
	src := &StubLiveSource{}
	period := Period{Span: 1, Unit: UnitHour}
	if err := src.SubscribeBar("ALPHA", period, nil); !errors.Is(err, ErrNilBarHandler) {
		t.Fatalf("nil handler err %v", err)
	}
	if err := src.SubscribeBar("ALPHA", Period{Span: 0, Unit: UnitHour}, func(Candle) {}); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("invalid period err %v", err)
	}
	src.Push("ALPHA", period, Candle{Close: 4})
	if _, _, err := src.GetBars(BarRequest{Symbol: "ALPHA", Period: Period{Unit: "week"}, Kind: KindInit}); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("get bars err %v", err)
	}
}

func TestStubLiveUnsubscribeIsIdempotent(t *testing.T) {
	src := &StubLiveSource{}
	period := Period{Span: 1, Unit: UnitHour}
	if err := src.UnsubscribeBar("ALPHA", period); err != nil {
		t.Fatal(err)
	}
	if err := src.SubscribeBar("ALPHA", period, func(Candle) {}); err != nil {
		t.Fatal(err)
	}
	if err := src.UnsubscribeBar("ALPHA", period); err != nil {
		t.Fatal(err)
	}
	if err := src.UnsubscribeBar("ALPHA", period); err != nil {
		t.Fatal(err)
	}
}

func TestStubLiveHistoryInitAndForward(t *testing.T) {
	src := &StubLiveSource{}
	period := Period{Span: 1, Unit: UnitHour}
	start := time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC)
	bars := make([]Candle, 5)
	for i := range bars {
		bars[i] = Candle{Time: start.Add(time.Duration(i) * time.Hour), Close: float64(i + 1)}
	}
	if err := src.SetBars("ALPHA", period, bars); err != nil {
		t.Fatal(err)
	}
	bars[0].Close = -1
	got, more, err := src.GetBars(BarRequest{Symbol: "ALPHA", Period: period, Kind: KindInit, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !more || len(got) != 3 || got[0].Close != 3 || got[2].Close != 5 {
		t.Fatalf("init %+v more %v", got, more)
	}
	fwd, more, err := src.GetBars(BarRequest{
		Symbol: "ALPHA", Period: period, Kind: KindForward, Anchor: got[0].Time, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if more || len(fwd) != 2 || fwd[0].Close != 1 || fwd[1].Close != 2 {
		t.Fatalf("forward %+v more %v", fwd, more)
	}
	if !fwd[1].Time.Before(got[0].Time) {
		t.Fatal("forward overlapped the init window")
	}
	one, more, err := src.GetBars(BarRequest{
		Symbol: "ALPHA", Period: period, Kind: KindForward, Anchor: got[0].Time, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !more || len(one) != 1 || one[0].Close != 2 {
		t.Fatalf("short forward %+v more %v", one, more)
	}
	_, _, err = src.GetBars(BarRequest{
		Symbol: "ALPHA", Period: period, Kind: KindForward, Limit: 1,
	})
	if err == nil {
		t.Fatal("forward without an anchor succeeded")
	}
	_, _, err = src.GetBars(BarRequest{
		Symbol: "ALPHA", Period: period, Kind: KindBackward, Limit: 1,
	})
	if !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("backward err %v", err)
	}
}

func TestStubLivePushRevisesLastBarInHistory(t *testing.T) {
	src := &StubLiveSource{}
	period := Period{Span: 1, Unit: UnitHour}
	t0 := time.Date(2024, 1, 2, 9, 0, 0, 0, time.UTC)
	if err := src.SetBars("ALPHA", period, []Candle{{Time: t0, Close: 1}}); err != nil {
		t.Fatal(err)
	}
	var got []Candle
	if err := src.SubscribeBar("ALPHA", period, func(c Candle) { got = append(got, c) }); err != nil {
		t.Fatal(err)
	}
	src.Push("ALPHA", period, Candle{Time: t0, Close: 7})
	src.Push("ALPHA", period, Candle{Time: t0.Add(time.Hour), Close: 8})
	src.Push("ALPHA", period, Candle{Time: t0.Add(-time.Hour), Close: 9})
	bars, _, err := src.GetBars(BarRequest{Symbol: "ALPHA", Period: period, Kind: KindInit, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 || bars[0].Close != 7 || bars[1].Close != 8 {
		t.Fatalf("history %+v", bars)
	}
	if len(got) != 3 || got[2].Close != 9 {
		t.Fatalf("handler %+v", got)
	}
}
