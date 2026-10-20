package kline

import (
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"
)

func TestForwardSkipsWhenOnePaneHasNoMore(t *testing.T) {
	a := newApp()
	a.fitted = true
	a.vp.Offset = 2
	a.crossIndex = 20
	a.panes[1].more = false
	const watch = 20
	off := a.vp.Offset
	want := float64(watch) - off
	t0 := a.panes[0].candles[watch].Time
	t1 := a.panes[1].candles[watch].Time
	n0, n1 := len(a.panes[0].candles), len(a.panes[1].candles)
	calls := 0
	inner := a.src
	a.src = sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
		if req.Kind == KindForward {
			calls++
		}
		return inner.GetBars(req)
	})
	a.ensureForward()
	a.ensureForward()
	if calls != 0 {
		t.Fatalf("forward calls %d, want 0", calls)
	}
	if len(a.panes[0].candles) != n0 || len(a.panes[1].candles) != n1 {
		t.Fatalf("lengths %d %d, want %d %d", len(a.panes[0].candles), len(a.panes[1].candles), n0, n1)
	}
	if a.vp.Offset != off || a.crossIndex != 20 {
		t.Fatalf("offset %v cross %d", a.vp.Offset, a.crossIndex)
	}
	assertScreen(t, a.panes[0], t0, a.vp.Offset, want)
	assertScreen(t, a.panes[1], t1, a.vp.Offset, want)
	if a.panes[1].more {
		t.Fatal("BETA more changed")
	}
	if !a.panes[0].more {
		t.Fatal("ALPHA more changed")
	}
}

func TestForwardEmptyBatchDoesNotShift(t *testing.T) {
	a := newApp()
	a.fitted = true
	a.vp.Offset = 2
	a.crossIndex = 12
	const watch = 12
	off := a.vp.Offset
	want := float64(watch) - off
	t0 := a.panes[0].candles[watch].Time
	t1 := a.panes[1].candles[watch].Time
	n0, n1 := len(a.panes[0].candles), len(a.panes[1].candles)
	inner := a.src
	var forwards int
	a.src = sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
		if req.Kind != KindForward {
			return inner.GetBars(req)
		}
		forwards++
		// The anchor itself is not strictly earlier. more stays as reported.
		return []Candle{{Time: req.Anchor, Close: 1}}, true, nil
	})
	a.ensureForward()
	if a.vp.Offset != off || a.crossIndex != 12 {
		t.Fatalf("offset %v cross %d", a.vp.Offset, a.crossIndex)
	}
	if len(a.panes[0].candles) != n0 || len(a.panes[1].candles) != n1 {
		t.Fatalf("lengths %d %d", len(a.panes[0].candles), len(a.panes[1].candles))
	}
	assertScreen(t, a.panes[0], t0, a.vp.Offset, want)
	assertScreen(t, a.panes[1], t1, a.vp.Offset, want)
	if !a.panes[0].more || !a.panes[1].more {
		t.Fatalf("more forced false: %v %v", a.panes[0].more, a.panes[1].more)
	}
	got := forwards
	a.ensureForward()
	if forwards != got {
		t.Fatalf("empty batch retried immediately: %d then %d", got, forwards)
	}
}

func TestForwardOneEmptyBatchDoesNotShift(t *testing.T) {
	a := newApp()
	a.fitted = true
	a.vp.Offset = 2
	a.crossIndex = 12
	const watch = 12
	off := a.vp.Offset
	want := float64(watch) - off
	t0 := a.panes[0].candles[watch].Time
	t1 := a.panes[1].candles[watch].Time
	n0, n1 := len(a.panes[0].candles), len(a.panes[1].candles)
	inner := a.src
	a.src = sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
		if req.Kind == KindForward && req.Symbol == "BETA" {
			return []Candle{}, false, nil
		}
		return inner.GetBars(req)
	})
	a.ensureForward()
	if a.vp.Offset != off || a.crossIndex != 12 {
		t.Fatalf("offset %v cross %d", a.vp.Offset, a.crossIndex)
	}
	if len(a.panes[0].candles) != n0 || len(a.panes[1].candles) != n1 {
		t.Fatalf("lengths %d %d, want %d %d", len(a.panes[0].candles), len(a.panes[1].candles), n0, n1)
	}
	assertScreen(t, a.panes[0], t0, a.vp.Offset, want)
	assertScreen(t, a.panes[1], t1, a.vp.Offset, want)
	if a.panes[1].more {
		t.Fatal("BETA more should stay the source's false")
	}
	if !a.panes[0].more {
		t.Fatal("ALPHA more should stay the source's true")
	}
}

func TestForwardErrorDoesNotSpin(t *testing.T) {
	a := newApp()
	a.fitted = true
	a.vp.Offset = 1
	a.crossIndex = 8
	n0, n1 := len(a.panes[0].candles), len(a.panes[1].candles)
	more0, more1 := a.panes[0].more, a.panes[1].more
	off := a.vp.Offset
	first0 := a.panes[0].candles[0].Time
	var calls int
	a.src = sourceFunc(func(req BarRequest) ([]Candle, bool, error) {
		if req.Kind == KindForward {
			calls++
			return nil, true, errors.New("feed down")
		}
		return nil, false, fmt.Errorf("unexpected %s", req.Kind)
	})
	a.ensureForward()
	a.ensureForward()
	a.ensureForward()
	if calls != 1 {
		t.Fatalf("forward calls %d, want 1", calls)
	}
	if len(a.panes[0].candles) != n0 || len(a.panes[1].candles) != n1 {
		t.Fatalf("lengths %d %d", len(a.panes[0].candles), len(a.panes[1].candles))
	}
	if a.panes[0].more != more0 || a.panes[1].more != more1 {
		t.Fatalf("more changed %v %v", a.panes[0].more, a.panes[1].more)
	}
	if a.vp.Offset != off || a.crossIndex != 8 {
		t.Fatalf("offset %v cross %d", a.vp.Offset, a.crossIndex)
	}
	if !a.panes[0].candles[0].Time.Equal(first0) {
		t.Fatal("pane mutated on forward error")
	}
}

func TestLiveHandlerAfterUnsubscribeDoesNotAppend(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	four := Period{Span: 4, Unit: UnitHour}
	stub := &keepHandlers{StubLiveSource: &StubLiveSource{}}
	alpha := Generate(30, 42)
	beta := Generate(30, 99)
	if err := stub.SetBars("ALPHA", hour, alpha); err != nil {
		t.Fatal(err)
	}
	if err := stub.SetBars("BETA", hour, beta); err != nil {
		t.Fatal(err)
	}
	fourA := sampleWalk(30, 42, otherOrigin, 4*time.Hour)
	fourB := sampleWalk(30, 99, otherOrigin, 4*time.Hour)
	if err := stub.SetBars("ALPHA", four, fourA); err != nil {
		t.Fatal(err)
	}
	if err := stub.SetBars("BETA", four, fourB); err != nil {
		t.Fatal(err)
	}
	a := NewChart(ChartOptions{
		Source:     stub,
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     hour,
		Indicators: []string{},
	})
	if len(stub.handlers) != 2 {
		t.Fatalf("handlers %d", len(stub.handlers))
	}
	stale := stub.handlers[0]
	if err := a.SetPeriod(four); err != nil {
		t.Fatal(err)
	}
	before := len(a.panes[0].candles)
	stale(Candle{Time: fourA[len(fourA)-1].Time.Add(time.Hour), Close: 99})
	if len(a.panes[0].candles) != before {
		t.Fatalf("stale handler appended, len %d", len(a.panes[0].candles))
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if len(stub.handlers) < 3 {
		t.Fatal("4h subscribe did not run")
	}
	closed := stub.handlers[len(stub.handlers)-1]
	closed(Candle{Time: fourA[len(fourA)-1].Time.Add(4 * time.Hour), Close: 7})
	if len(a.panes[0].candles) != before {
		t.Fatalf("handler after Close appended, len %d", len(a.panes[0].candles))
	}
	stub.Push("ALPHA", four, Candle{Time: fourA[len(fourA)-1].Time.Add(8 * time.Hour), Close: 8})
	if len(a.panes[0].candles) != before {
		t.Fatal("Push after Close appended")
	}
}

func TestSubscribeBarErrorRollsBack(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	alpha := Generate(20, 42)
	beta := Generate(20, 99)
	if err := stub.SetBars("ALPHA", hour, alpha); err != nil {
		t.Fatal(err)
	}
	if err := stub.SetBars("BETA", hour, beta); err != nil {
		t.Fatal(err)
	}
	bad := &subscribeFail{StubLiveSource: stub, fail: "BETA"}
	a := NewChart(ChartOptions{
		Source:     bad,
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     hour,
		Indicators: []string{},
	})
	if err := a.SetPeriod(hour); err == nil {
		t.Fatal("SetPeriod swallowed SubscribeBar error")
	}
	n := len(a.panes[0].candles)
	if len(bad.handlers) == 0 {
		t.Fatal("no handler captured")
	}
	bad.handlers[0](Candle{Time: alpha[len(alpha)-1].Time.Add(time.Hour), Close: 3})
	stub.Push("ALPHA", hour, Candle{Time: alpha[len(alpha)-1].Time.Add(2 * time.Hour), Close: 4})
	if len(a.panes[0].candles) != n {
		t.Fatalf("partial subscribe still applied, len %d", len(a.panes[0].candles))
	}

	good := &StubLiveSource{}
	if err := good.SetBars("ALPHA", hour, alpha); err != nil {
		t.Fatal(err)
	}
	if err := good.SetBars("BETA", hour, beta); err != nil {
		t.Fatal(err)
	}
	base := NewChart(ChartOptions{
		Source:     good,
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     hour,
		Indicators: []string{},
	})
	next := &subscribeFail{StubLiveSource: stub, fail: "BETA"}
	if err := base.SetSource(next); err == nil {
		t.Fatal("SetSource swallowed SubscribeBar error")
	}
	n = len(base.panes[0].candles)
	if len(next.handlers) == 0 {
		t.Fatal("SetSource captured no handler")
	}
	next.handlers[0](Candle{Time: alpha[len(alpha)-1].Time.Add(3 * time.Hour), Close: 5})
	stub.Push("ALPHA", hour, Candle{Time: alpha[len(alpha)-1].Time.Add(4 * time.Hour), Close: 6})
	if len(base.panes[0].candles) != n {
		t.Fatalf("SetSource partial subscribe applied, len %d", len(base.panes[0].candles))
	}
}

func TestLiveMarshalHopsApply(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &StubLiveSource{}
	alpha := Generate(16, 42)
	beta := Generate(16, 99)
	if err := stub.SetBars("ALPHA", hour, alpha); err != nil {
		t.Fatal(err)
	}
	if err := stub.SetBars("BETA", hour, beta); err != nil {
		t.Fatal(err)
	}
	var chart *Chart
	hopped := 0
	chart = NewChart(ChartOptions{
		Source:     stub,
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     hour,
		Indicators: []string{},
		Marshal: func(fn func()) {
			before := len(chart.panes[0].candles)
			fn()
			if len(chart.panes[0].candles) != before+1 {
				t.Fatalf("hook ran without appending: %d -> %d", before, len(chart.panes[0].candles))
			}
			hopped++
		},
	})
	n := len(chart.panes[0].candles)
	last := chart.panes[0].candles[n-1]
	next := last
	next.Time = last.Time.Add(time.Hour)
	next.Close = last.Close + 1
	stub.Push("ALPHA", hour, next)
	if hopped != 1 {
		t.Fatalf("marshal calls %d", hopped)
	}
	if len(chart.panes[0].candles) != n+1 || chart.panes[0].candles[n].Close != next.Close {
		t.Fatal("live bar was not applied inside Marshal")
	}
}

func TestLiveOffThreadPushIsFenced(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	stub := &keepHandlers{StubLiveSource: &StubLiveSource{}}
	alpha := Generate(16, 42)
	beta := Generate(16, 99)
	if err := stub.SetBars("ALPHA", hour, alpha); err != nil {
		t.Fatal(err)
	}
	if err := stub.SetBars("BETA", hour, beta); err != nil {
		t.Fatal(err)
	}
	hop := make(chan func(), 4)
	var chart *Chart
	chart = NewChart(ChartOptions{
		Source:     stub,
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     hour,
		Indicators: []string{},
		Marshal:    func(fn func()) { hop <- fn },
	})
	n := len(chart.panes[0].candles)
	last := chart.panes[0].candles[n-1]
	next := last
	next.Time = last.Time.Add(time.Hour)
	next.Close = last.Close + 1
	go stub.Push("ALPHA", hour, next)
	var fn func()
	select {
	case fn = <-hop:
	case <-time.After(2 * time.Second):
		t.Fatal("off-thread Push did not hop through Marshal")
	}
	for i := 0; i < 50; i++ {
		_ = len(chart.panes[0].candles)
		_ = chart.vp.Offset
	}
	fn()
	if len(chart.panes[0].candles) != n+1 {
		t.Fatalf("fenced push len %d", len(chart.panes[0].candles))
	}
	if err := chart.Close(); err != nil {
		t.Fatal(err)
	}
	if len(stub.handlers) == 0 {
		t.Fatal("no handler")
	}
	stale := stub.handlers[0]
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		bar := Candle{Time: next.Time.Add(time.Hour), Close: 1}
		for {
			select {
			case <-stop:
				return
			default:
				stale(bar)
			}
		}
	}()
	for i := 0; i < 100; i++ {
		_ = len(chart.panes[0].candles)
		_ = chart.vp.Offset
	}
	close(stop)
	wg.Wait()
	if len(chart.panes[0].candles) != n+1 {
		t.Fatalf("stale off-thread handler appended, len %d", len(chart.panes[0].candles))
	}
}

func assertScreen(t *testing.T, s series, at time.Time, offset, want float64) {
	t.Helper()
	for i, c := range s.candles {
		if c.Time.Equal(at) {
			if math.Abs(float64(i)-offset-want) > 1e-9 {
				t.Fatalf("screen %v, want %v", float64(i)-offset, want)
			}
			return
		}
	}
	t.Fatalf("missing %s", at)
}

type keepHandlers struct {
	*StubLiveSource
	mu       sync.Mutex
	handlers []BarHandler
}

func (k *keepHandlers) SubscribeBar(symbol string, period Period, onBar BarHandler) error {
	k.mu.Lock()
	k.handlers = append(k.handlers, onBar)
	k.mu.Unlock()
	return k.StubLiveSource.SubscribeBar(symbol, period, onBar)
}

type subscribeFail struct {
	*StubLiveSource
	fail     string
	handlers []BarHandler
}

func (s *subscribeFail) SubscribeBar(symbol string, period Period, onBar BarHandler) error {
	if symbol == s.fail {
		return errors.New("subscribe failed")
	}
	s.handlers = append(s.handlers, onBar)
	return s.StubLiveSource.SubscribeBar(symbol, period, onBar)
}
