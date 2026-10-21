package kline

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The visible markers must depend only on the revealed prefix. A change
// to a hidden bar's open time cannot make a marker visible or invisible.
func TestReplayMarkerWindowIgnoresHiddenSuffix(t *testing.T) {
	first := time.Date(2024, 3, 1, 8, 0, 0, 0, time.UTC)
	base := func(hiddenHour int) map[string][]Candle {
		cs := sampleWalk(4, 1, first, time.Hour)
		cs[2].Time = time.Date(2024, 3, 1, hiddenHour, 0, 0, 0, time.UTC)
		cs[3].Time = cs[2].Time.Add(time.Hour)
		return map[string][]Candle{"600519": cs}
	}
	for _, hiddenHour := range []int{10, 11} {
		c, _ := replayChart(t, []string{"600519"}, base(hiddenHour), LayoutStack)
		r, err := StartReplay(c, base(hiddenHour))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.SetMarkers("600519", []Marker{{
			Time: first.Add(150 * time.Minute), Side: SideBuy, Label: "FUTURE_1030",
		}}); err != nil {
			t.Fatal(err)
		}
		if err := r.SeekIndex(2); err != nil {
			t.Fatal(err)
		}
		if err := c.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
			t.Fatal(err)
		}
		if got := c.panes[0].marks[0].bar; got != -1 {
			t.Fatalf("hidden hour %d: marker mapped to %d", hiddenHour, got)
		}
		if got := c.Readout("600519"); containsText(got, "FUTURE_1030") {
			t.Fatalf("hidden hour %d: readout leaked %q", hiddenHour, got)
		}
	}
}

// The lower edge of the revealed interval is the first revealed base
// open, not the floor of the first aggregate bucket.
func TestReplayMarkerWindowHasLowerBound(t *testing.T) {
	first := time.Date(2024, 3, 1, 9, 30, 0, 0, time.UTC)
	base := sampleWalk(4, 2, first, time.Hour)
	all := map[string][]Candle{"600519": base}
	c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
	r, err := StartReplay(c, all)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMarkers("600519", []Marker{{
		Time: first.Add(-time.Hour), Side: SideBuy, Label: "BEFORE",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := r.SeekIndex(1); err != nil {
		t.Fatal(err)
	}
	if err := c.SetPeriod(Period{Span: 4, Unit: UnitHour}); err != nil {
		t.Fatal(err)
	}
	if got := c.panes[0].marks[0].bar; got != -1 {
		t.Fatalf("marker before the first revealed bar mapped to %d", got)
	}
}

// A partial rollback must fence handlers before it starts unsubscribing.
// A feed may invoke an old handler from UnsubscribeBar.
func TestSubscribeRollbackFencesOldHandlers(t *testing.T) {
	hour := Period{Span: 1, Unit: UnitHour}
	later := Generate(20, 3)[19]
	later.Close = 999
	src := &rollbackFeed{failAt: "C"}
	calls := 0
	src.onUnsubscribe = func() {
		calls++
		src.retired(later)
	}
	c := NewChart(ChartOptions{
		Source:     src,
		Symbols:    []string{"A", "B", "C"},
		Period:     hour,
		Indicators: []string{},
	})
	if c.LoadError() == nil {
		t.Fatal("the feed should report the third subscription failure")
	}
	if calls != 1 {
		t.Fatalf("retired handler called %d times, want 1", calls)
	}
	if got := c.panes[0].candles[len(c.panes[0].candles)-1].Close; math.Abs(got-999) < 1e-9 {
		t.Fatalf("a retired A handler wrote through the rollback: %v", got)
	}
}

type rollbackFeed struct {
	hist          map[string][]Candle
	handlers      map[string]BarHandler
	failAt        string
	onUnsubscribe func()
	retired       BarHandler
}

func (f *rollbackFeed) GetBars(req BarRequest) ([]Candle, bool, error) {
	if f.hist == nil {
		f.hist = map[string][]Candle{}
	}
	if _, ok := f.hist[req.Symbol]; !ok {
		f.hist[req.Symbol] = Generate(20, int64(len(req.Symbol)+int(req.Symbol[0])))
	}
	return cloneCandles(f.hist[req.Symbol]), false, nil
}

func (f *rollbackFeed) SubscribeBar(symbol string, period Period, onBar BarHandler) error {
	if symbol == f.failAt {
		return errors.New("third subscription failed")
	}
	if f.handlers == nil {
		f.handlers = map[string]BarHandler{}
	}
	f.handlers[symbol] = onBar
	return nil
}

func (f *rollbackFeed) UnsubscribeBar(symbol string, period Period) error {
	if symbol == "A" {
		f.retired = f.handlers[symbol]
	}
	delete(f.handlers, symbol)
	if symbol == "B" && f.onUnsubscribe != nil {
		fn := f.onUnsubscribe
		f.onUnsubscribe = nil
		fn()
	}
	return nil
}

// An OnBar callback may ask for the period already loaded. That is a
// no-op and must not cancel the remaining reveals in the batch.
func TestReplayNoOpPeriodDoesNotCancelBatch(t *testing.T) {
	all := map[string][]Candle{"600519": Generate(20, 4)}
	c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
	r, err := StartReplay(c, all)
	if err != nil {
		t.Fatal(err)
	}
	callbacks := 0
	r.OnBar(func(ReplayBar) {
		callbacks++
		if err := c.SetPeriod(c.Period()); err != nil {
			t.Errorf("same-period SetPeriod: %v", err)
		}
	})
	if err := r.Step(5); err != nil {
		t.Fatal(err)
	}
	if callbacks != 5 || r.Cursor() != 5 || c.Bars() != 5 {
		t.Fatalf("callbacks %d cursor %d bars %d, want 5, 5, 5", callbacks, r.Cursor(), c.Bars())
	}
}

// A rejected period request is also a no-op for the current batch.
func TestReplayRejectedPeriodDoesNotCancelBatch(t *testing.T) {
	for _, p := range []Period{
		{},
		{Span: 15, Unit: UnitMinute},
	} {
		all := map[string][]Candle{"600519": Generate(20, 5)}
		c, _ := replayChart(t, []string{"600519"}, all, LayoutStack)
		r, err := StartReplay(c, all)
		if err != nil {
			t.Fatal(err)
		}
		callbacks := 0
		r.OnBar(func(ReplayBar) {
			callbacks++
			_ = c.SetPeriod(p)
		})
		if err := r.Step(4); err != nil {
			t.Fatal(err)
		}
		if callbacks != 4 || r.Cursor() != 4 {
			t.Fatalf("%s: callbacks %d cursor %d, want 4 and 4", p, callbacks, r.Cursor())
		}
	}
}

func TestOverlayIndexBaseRequiresFiniteClose(t *testing.T) {
	primary := Generate(20, 6)
	primary[0].Close = math.Inf(1)
	compare := Generate(20, 7)
	vp := Viewport{BarWidth: 8}
	plotW := 160.0
	index, raw, base := CompareScales(primary, compare, nil, vp, plotW)
	if base < 1 {
		t.Fatalf("infinite primary base accepted: base %d", base)
	}
	if math.IsInf(index.Hi, 0) || math.IsInf(index.Lo, 0) || math.IsInf(raw.Hi, 0) || math.IsInf(raw.Lo, 0) {
		t.Fatalf("infinite scale: index %+v raw %+v", index, raw)
	}
	compare[1].Close = math.Inf(1)
	if _, _, base = CompareScales(primary, compare, nil, vp, plotW); base < 2 {
		t.Fatalf("infinite compare base accepted: base %d", base)
	}
	if got := NormalizeClose(primary, 0); got != nil {
		t.Fatalf("NormalizeClose accepted an infinite base: %v", got[:3])
	}
	if got := NormalizeToIndex(primary, 0); got != nil {
		t.Fatalf("NormalizeToIndex accepted an infinite base")
	}
}

func TestCrosshairIgnoresPriceAxis(t *testing.T) {
	style := DarkDefault()
	style.Cross = ui.RGB(0, 255, 255)
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Indicators: []string{},
		Style:      style,
	})
	tt := ui.NewTester(c.View, 900, 500)
	tt.Frame()
	c.vp.Offset = 50
	tt.Frame()
	first, last := c.Visible()
	if first < 1 {
		t.Fatalf("setup should have panned-away bars, visible %d..%d", first, last)
	}
	price := c.painted[0].Price
	for _, tc := range []struct {
		name string
		x    float32
		ok   bool
	}{
		{"left boundary", price.X, true},
		{"right plot boundary", price.X + price.W, true},
		{"inside plot", price.X + price.W*0.25, true},
		{"price axis", price.X + price.W + 1, false},
		{"price axis far", price.X + price.W + price.W/10, false},
	} {
		tt.Move(tc.x, price.Y+price.H/2)
		_, idx, found := c.Crosshair()
		if found != tc.ok {
			t.Fatalf("%s: found=%v, want %v", tc.name, found, tc.ok)
		}
		if found && (idx < first || idx > last) {
			t.Fatalf("%s: bar %d outside visible %d..%d", tc.name, idx, first, last)
		}
		if !tc.ok {
			if got := countNear(tt.Image(), price, style.Cross); got != 0 {
				t.Fatalf("%s: %d crosshair pixels in the price plot, want none", tc.name, got)
			}
		}
	}
}

func TestOverlaySecondaryMarkersUsePlottedClose(t *testing.T) {
	primary := Generate(40, 8)
	compare := Generate(40, 9)
	for i := range compare {
		compare[i].Open = 10
		compare[i].High = 12
		compare[i].Low = 8
		compare[i].Close = 10
	}
	src := &fixedPairSource{primary: primary, compare: compare}
	c := NewChart(ChartOptions{
		Source:     src,
		Symbols:    []string{"A", "B"},
		Layout:     LayoutOverlay,
		Indicators: []string{},
	})
	if err := c.LoadError(); err != nil {
		t.Fatal(err)
	}
	style := DarkDefault()
	style.Buy = ui.RGB(0, 255, 0)
	style.Sell = ui.RGB(255, 0, 255)
	c.SetStyle(style)
	tt := ui.NewTester(c.View, 900, 700)
	tt.Frame()
	first, last := c.Visible()
	when := c.panes[1].candles[(first+last)/2].Time
	if err := c.SetMarkers("B", []Marker{
		{Time: when, Side: SideBuy, Label: "B"},
		{Time: when, Side: SideSell, Label: "S"},
	}); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	price := c.painted[0].Price
	if got := countNear(tt.Image(), price, style.Buy); got == 0 {
		t.Fatalf("secondary overlay buy marker painted no pixels")
	}
	if got := countNear(tt.Image(), price, style.Sell); got == 0 {
		t.Fatalf("secondary overlay sell marker painted no pixels")
	}
	if got := countNear(tt.Image(), c.painted[0].Volume, style.Buy) + countNear(tt.Image(), c.painted[0].Volume, style.Sell); got != 0 {
		t.Fatalf("secondary overlay markers painted %d pixels outside the price plot", got)
	}
}

func TestPaintComparePaintsPrimaryTail(t *testing.T) {
	primary := Generate(10, 10)
	for i := range primary {
		primary[i].Close = 100
		primary[i].Open = 100
		primary[i].High = 101
		primary[i].Low = 99
	}
	primary[9].Close = 200
	primary[9].Open = 199
	primary[9].Low = 198
	primary[9].High = 201
	compare := Generate(3, 11)
	panes := Split(ui.Rect{W: 600, H: 400})
	vp := Viewport{BarWidth: 50}
	style := DarkDefault()
	_, raw, _ := CompareScales(primary, compare, nil, vp, float64(panes.Price.W))
	if raw.Hi < primary[9].High {
		t.Fatalf("raw scale %+v excludes primary tail high %v", raw, primary[9].High)
	}
	tt := ui.NewTester(func(ctx *ui.Context) {
		ui.Box(ctx).Fill().Draw(func(p *ui.Painter, _ ui.Rect) {
			PaintCompare(p, panes, primary, compare, nil, vp, Pointer{}, -1, style)
		})
	}, 600, 400)
	tt.Frame()
	x := panes.Price.X + float32(vp.BarLeft(9)+vp.BarWidth/2)
	if got := countNear(tt.Image(), ui.Rect{X: x - 4, Y: panes.Price.Y, W: 8, H: panes.Price.H}, style.Up); got == 0 {
		t.Fatal("the primary tail was clipped by a scale computed over the compare length")
	}
}

func TestTimeAxisUsesChartLocation(t *testing.T) {
	instant := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	cs := sampleWalk(12, 12, instant, time.Hour)
	vp := Viewport{BarWidth: 100}
	const plotW = 600
	shanghai := labelFmt{digits: 2, loc: ashareLocation}
	ticks := timeTicks(cs, vp, plotW, shanghai)
	if len(ticks) == 0 {
		t.Fatal("no time ticks")
	}
	if ticks[0].Label != "08:00" {
		t.Fatalf("first tick %q, want Shanghai 08:00", ticks[0].Label)
	}
	local := cloneCandles(cs)
	for i := range local {
		local[i].Time = local[i].Time.In(ashareLocation)
	}
	if !reflect.DeepEqual(ticks, timeTicks(local, vp, plotW, shanghai)) {
		t.Fatal("equivalent instants generated different ticks")
	}
	for _, tick := range ticks {
		at := time.UnixMilli(int64(tick.Value)).In(ashareLocation)
		if tick.Label == "" {
			t.Fatalf("tick %v has no label", at)
		}
	}
	utc := timeTicks(cs, vp, plotW, labelFmt{digits: 2, loc: time.UTC})
	changed := false
	for i := range utc {
		if i < len(ticks) && utc[i].Label != ticks[i].Label {
			changed = true
		}
	}
	if !changed {
		t.Fatal("UTC and Asia/Shanghai produced identical tick labels")
	}
}

func TestCollapsedLayoutsStayInsideHostRect(t *testing.T) {
	for _, rect := range []ui.Rect{
		{W: 200, H: 24},
		{W: 200, H: 32},
		{W: 200, H: 80},
	} {
		for _, panes := range []Panes{
			SplitStack(rect, true, true, true),
			splitMany(rect, true, 5),
		} {
			for _, r := range append([]ui.Rect{panes.Price, panes.Volume, panes.MACD, panes.KDJ, panes.Time, panes.PriceAxis, panes.VolumeAxis, panes.MACDAxis, panes.KDJAxis}, append(panes.Extra, panes.ExtraAxis...)...) {
				if r.W <= 0 || r.H <= 0 {
					continue
				}
				if r.X < rect.X || r.Y < rect.Y || r.X+r.W > rect.X+rect.W+0.01 || r.Y+r.H > rect.Y+rect.H+0.01 {
					t.Fatalf("%+v escaped host %+v: %+v", rect, rect, r)
				}
			}
		}
	}
}

func containsText(s, want string) bool {
	if want == "" {
		return true
	}
	for i := 0; i+len(want) <= len(s); i++ {
		if s[i:i+len(want)] == want {
			return true
		}
	}
	return false
}

func TestTimeAxisRenderingIgnoresSourceZone(t *testing.T) {
	cs := sampleWalk(30, 12, time.Date(2024, 3, 1, 1, 30, 0, 0, time.UTC), 15*time.Minute)
	local := cloneCandles(cs)
	for i := range local {
		local[i].Time = local[i].Time.In(ashareLocation)
	}
	for _, compare := range []bool{false, true} {
		render := func(bars []Candle, loc *time.Location) []byte {
			panes := Split(ui.Rect{W: 672, H: 300})
			vp := Viewport{BarWidth: 30}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Fill().Draw(func(p *ui.Painter, _ ui.Rect) {
					labels := labelFmt{digits: 2, loc: loc}
					if compare {
						drawCompare(p, panes, bars, bars, nil, vp, Pointer{}, -1, DarkDefault(), labels)
					} else {
						drawPrice(p, panes, bars, nil, vp, Pointer{}, -1, DarkDefault(), labels)
					}
				})
			}, 672, 300)
			return append([]byte(nil), tt.Image().Pix...)
		}
		shanghai := render(cs, ashareLocation)
		if !bytes.Equal(shanghai, render(local, ashareLocation)) {
			t.Fatalf("compare=%v: source timestamp zone changed rendered axis", compare)
		}
		if bytes.Equal(shanghai, render(cs, time.UTC)) {
			t.Fatalf("compare=%v: explicit UTC did not change the axis", compare)
		}
		if !bytes.Equal(render(local, nil), render(cs, time.UTC)) {
			t.Fatalf("compare=%v: default low-level labels must use UTC", compare)
		}
	}
}
