package kline_test

import (
	"errors"
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/shenjing023/KLineChart-go/kline"
)

// hostFeed is the smallest DataSource + LiveSource a host has to write.
// History is a slice per symbol. Push delivers one bar to the subscriber.
type hostFeed struct {
	period kline.Period
	hist   map[string][]kline.Candle
	subs   map[string]kline.BarHandler
}

func newHostFeed(period kline.Period, symbols ...string) *hostFeed {
	f := &hostFeed{
		period: period,
		hist:   map[string][]kline.Candle{},
		subs:   map[string]kline.BarHandler{},
	}
	for i, symbol := range symbols {
		f.hist[symbol] = kline.Generate(80, int64(10+i))
	}
	return f
}

func (f *hostFeed) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	if !req.Period.Valid() {
		return nil, false, kline.ErrInvalidPeriod
	}
	all := f.hist[req.Symbol]
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	switch req.Kind {
	case kline.KindInit, "":
		if len(all) <= limit {
			return append([]kline.Candle(nil), all...), false, nil
		}
		return append([]kline.Candle(nil), all[len(all)-limit:]...), true, nil
	case kline.KindForward:
		n := 0
		for n < len(all) && all[n].Time.Before(req.Anchor) {
			n++
		}
		if n == 0 {
			return []kline.Candle{}, false, nil
		}
		if n <= limit {
			return append([]kline.Candle(nil), all[:n]...), false, nil
		}
		return append([]kline.Candle(nil), all[n-limit:n]...), true, nil
	default:
		return nil, false, kline.ErrUnsupportedKind
	}
}

func (f *hostFeed) SubscribeBar(symbol string, period kline.Period, onBar kline.BarHandler) error {
	if onBar == nil {
		return kline.ErrNilBarHandler
	}
	if period != f.period {
		return kline.ErrInvalidPeriod
	}
	f.subs[symbol] = onBar
	return nil
}

func (f *hostFeed) UnsubscribeBar(symbol string, period kline.Period) error {
	delete(f.subs, symbol)
	return nil
}

func (f *hostFeed) Push(symbol string, bar kline.Candle) {
	if h := f.subs[symbol]; h != nil {
		h(bar)
	}
}

// hostLine is a main-pane indicator. DrawMain is empty: the chart strokes
// Series in stack and overlay layouts, and widens the price scale to fit.
type hostLine struct {
	values []float64
	color  ui.Color
}

func (h *hostLine) Name() string     { return "HOST" }
func (h *hostLine) PaneKind() string { return kline.PaneMain }
func (h *hostLine) Calc(cs []kline.Candle) {
	h.values = make([]float64, len(cs))
	for i, c := range cs {
		h.values[i] = c.Close + 30
	}
}
func (h *hostLine) DrawMain(*ui.Painter, ui.Rect, kline.Viewport, chart.Linear) {}
func (h *hostLine) DrawSub(*ui.Painter, ui.Rect, kline.Viewport)                {}
func (h *hostLine) ValueAt(i int) string {
	if i < 0 || i >= len(h.values) || math.IsNaN(h.values[i]) {
		return ""
	}
	return "HOST " + kline.FormatPrice(h.values[i])
}
func (h *hostLine) Series() [][]float64    { return [][]float64{h.values} }
func (h *hostLine) LineColors() []ui.Color { return []ui.Color{h.color} }

// hostOsc is a sub-pane indicator. It fills its band so a test can see
// that the chart opened a pane without a built-in MACD or KDJ name.
type hostOsc struct {
	values []float64
	color  ui.Color
}

func (h *hostOsc) Name() string     { return "HOSTOSC" }
func (h *hostOsc) PaneKind() string { return kline.PaneSub }
func (h *hostOsc) Calc(cs []kline.Candle) {
	h.values = make([]float64, len(cs))
	for i, c := range cs {
		h.values[i] = c.Close - c.Open
	}
}
func (h *hostOsc) DrawMain(*ui.Painter, ui.Rect, kline.Viewport, chart.Linear) {}
func (h *hostOsc) DrawSub(p *ui.Painter, rect ui.Rect, _ kline.Viewport) {
	if p == nil || rect.W < 2 || rect.H < 2 {
		return
	}
	p.Fill(rect, h.color, 0)
}
func (h *hostOsc) ValueAt(i int) string {
	if i < 0 || i >= len(h.values) || math.IsNaN(h.values[i]) {
		return ""
	}
	return "OSC " + kline.FormatPrice(h.values[i])
}
func (h *hostOsc) Scale(first, last int) chart.Linear {
	lo, hi := -1.0, 1.0
	if first < 0 {
		first = 0
	}
	if last >= len(h.values) {
		last = len(h.values) - 1
	}
	for i := first; i <= last && i < len(h.values); i++ {
		v := h.values[i]
		if math.IsNaN(v) {
			continue
		}
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	if hi <= lo {
		hi = lo + 1
	}
	return chart.NewLinear(lo, hi)
}

func registerHostSpecs(t *testing.T) {
	t.Helper()
	specs := []kline.Spec{
		{Name: "HOST", Kind: kline.PaneMain, New: func() kline.Indicator { return &hostLine{color: ui.RGB(255, 140, 0)} }},
		{Name: "HOSTOSC", Kind: kline.PaneSub, New: func() kline.Indicator { return &hostOsc{color: ui.RGB(20, 200, 40)} }},
	}
	for _, spec := range specs {
		err := kline.Register(spec)
		if err != nil && !errors.Is(err, kline.ErrDuplicateIndicator) {
			t.Fatal(err)
		}
	}
	if err := kline.Register(kline.Spec{
		Name: "HOST",
		Kind: kline.PaneMain,
		New:  func() kline.Indicator { return &hostLine{} },
	}); !errors.Is(err, kline.ErrDuplicateIndicator) {
		t.Fatalf("duplicate register: %v", err)
	}
}

func TestHostEmbedWithoutToolbar(t *testing.T) {
	period := kline.Period{Span: 1, Unit: kline.UnitHour}
	feed := newHostFeed(period, "ALPHA", "BETA")
	c := kline.NewChart(kline.ChartOptions{
		Source:     feed,
		Symbols:    []string{"ALPHA", "BETA"},
		Period:     period,
		Layout:     kline.LayoutStack,
		Indicators: []string{"MA", "MACD"},
		HideHeader: false,
	})
	tt := ui.NewTester(c.View, 960, 720)
	if !tt.HasText("ALPHA") || !tt.HasText("Last") {
		t.Fatalf("built-in header missing, texts %q", tt.Texts())
	}
	if _, ok := tt.Find("K-line"); ok {
		t.Fatal("View should not paint the demo toolbar title")
	}
	box, ok := tt.Find("ALPHA chart")
	if !ok {
		t.Fatal("stack plot missing")
	}
	x, y := box.X+box.W/2, box.Y+40
	tt.Press(x, y)
	tt.Move(x+80, y)
	tt.Release(x+80, y)
	beforeFirst, beforeLast := c.Visible()
	tt.Scroll(x, y, 0, -400)
	afterFirst, afterLast := c.Visible()
	if afterLast-afterFirst >= beforeLast-beforeFirst {
		t.Fatalf("wheel zoom in should show fewer bars: %d..%d -> %d..%d", beforeFirst, beforeLast, afterFirst, afterLast)
	}
	tt.Move(x, y)
	if !tt.HasText("Hover") {
		t.Fatalf("crosshair readout missing, texts %q", tt.Texts())
	}
	symbol, idx, ok := c.Crosshair()
	if !ok || symbol != "ALPHA" || idx < 0 {
		t.Fatalf("crosshair symbol %q index %d ok %v", symbol, idx, ok)
	}
	bar, ok := c.Bar("ALPHA", idx)
	if !ok || bar.Time.IsZero() {
		t.Fatal("Bar should return the hovered candle")
	}
	first, last := c.Visible()
	if first < 0 || last < first || idx < first || idx > last {
		t.Fatalf("visible %d..%d does not contain %d", first, last, idx)
	}
	if c.Readout("ALPHA") == "" || c.Readout("BETA") == "" {
		t.Fatal("host readout should be available without scraping the header")
	}
	if _, ok := c.Bar("NOPE", 0); ok {
		t.Fatal("unknown symbol should miss")
	}
}

func TestHideHeaderLeavesPlotAndReadout(t *testing.T) {
	c := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"ALPHA"},
		Indicators: []string{"MA"},
		HideHeader: true,
	})
	tt := ui.NewTester(c.View, 960, 720)
	if tt.HasText("Last") || tt.HasText("Hover") {
		t.Fatalf("hidden header still drew a readout, texts %q", tt.Texts())
	}
	box, ok := tt.Find("ALPHA chart")
	if !ok {
		t.Fatal("plot missing when the header is hidden")
	}
	tt.Move(box.X+box.W/2, box.Y+30)
	if tt.HasText("Hover") {
		t.Fatal("hidden header should stay hidden while the crosshair moves")
	}
	if _, _, ok := c.Crosshair(); !ok {
		t.Fatal("crosshair state should still update for the host readout")
	}
	if c.Readout("ALPHA") == "" {
		t.Fatal("Readout replaces the hidden header")
	}
}

func TestPartialStyleKeepsPalette(t *testing.T) {
	up := ui.RGB(1, 2, 3)
	c := kline.NewChart(kline.ChartOptions{Style: kline.Style{Up: up}})
	got := c.Style(nil)
	base := kline.DarkDefault()
	if got.Up != up {
		t.Fatalf("up %+v", got.Up)
	}
	if got.Plot != base.Plot || got.Grid != base.Grid || got.Down != base.Down {
		t.Fatalf("partial style cleared the palette: plot %+v grid %+v down %+v", got.Plot, got.Grid, got.Down)
	}
	c.EditStyle(ui.LightTheme(), func(s *kline.Style) {
		s.Down = ui.RGB(9, 8, 7)
	})
	got = c.Style(nil)
	light := kline.StyleFrom(ui.LightTheme())
	if got.Down != ui.RGB(9, 8, 7) || got.Plot != light.Plot {
		t.Fatalf("EditStyle plot %+v want %+v down %+v", got.Plot, light.Plot, got.Down)
	}
	if got.Up == up {
		t.Fatal("EditStyle should start from the theme passed in, not keep the previous override")
	}
}

func TestSetIndicatorsAndPeriodFootguns(t *testing.T) {
	c := kline.NewChart(kline.ChartOptions{Indicators: []string{"MA", "MACD"}})
	if err := c.SetIndicators([]string{"NOPE"}); err == nil || !errors.Is(err, kline.ErrUnknownIndicator) {
		t.Fatalf("unknown indicator: %v", err)
	}
	if !c.IndicatorOn("MA") || !c.IndicatorOn("MACD") {
		t.Fatal("rejected SetIndicators should leave the previous set")
	}
	if err := c.SetIndicators([]string{"BOLL"}); err != nil {
		t.Fatal(err)
	}
	if c.IndicatorOn("MA") || c.IndicatorOn("MACD") || !c.IndicatorOn("BOLL") || c.IndicatorOn("KDJ") {
		t.Fatalf("MA %v MACD %v BOLL %v KDJ %v", c.IndicatorOn("MA"), c.IndicatorOn("MACD"), c.IndicatorOn("BOLL"), c.IndicatorOn("KDJ"))
	}
	if err := c.SetIndicators(nil); err != nil {
		t.Fatal(err)
	}
	if c.IndicatorOn("BOLL") {
		t.Fatal("nil set should turn every indicator off")
	}
	bars := c.Bars()
	if err := c.SetPeriod(kline.Period{}); err == nil {
		t.Fatal("invalid period should fail")
	}
	if c.Bars() != bars || c.Period().String() != "1h" {
		t.Fatalf("failed period changed bars %d period %s", c.Bars(), c.Period())
	}
	if err := c.SetSource(nil); err != kline.ErrNilSource {
		t.Fatalf("nil source %v", err)
	}
	before := c.Bars()
	err := c.SetSource(failSource{})
	if err == nil {
		t.Fatal("a source that fails GetBars should not replace the chart")
	}
	if c.Bars() != before {
		t.Fatalf("failed SetSource changed bars %d -> %d", before, c.Bars())
	}
	if _, ok := c.Bar("ALPHA", 0); !ok {
		t.Fatal("failed SetSource dropped the previous series")
	}
}

// failSource is a DataSource whose reload always fails.
type failSource struct{}

func (failSource) GetBars(kline.BarRequest) ([]kline.Candle, bool, error) {
	return nil, false, errors.New("host feed down")
}

func TestHostIndicatorAndSubPane(t *testing.T) {
	registerHostSpecs(t)
	c := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"ALPHA"},
		Indicators: []string{"HOST", "HOSTOSC"},
	})
	if !c.IndicatorOn("HOST") || !c.IndicatorOn("HOSTOSC") || c.IndicatorOn("MA") {
		t.Fatalf("HOST %v OSC %v MA %v", c.IndicatorOn("HOST"), c.IndicatorOn("HOSTOSC"), c.IndicatorOn("MA"))
	}
	tt := ui.NewTester(c.View, 960, 720)
	if !tt.HasText("HOST ") || !tt.HasText("OSC ") {
		t.Fatalf("readout missing host values, texts %q", tt.Texts())
	}
	line := pixelsNear(tt.Image(), ui.RGB(255, 140, 0))
	if line < 30 {
		t.Fatalf("host line pixels %d", line)
	}
	c.SetLayout(kline.LayoutOverlay)
	tt.Frame()
	if pixelsNear(tt.Image(), ui.RGB(255, 140, 0)) < 30 {
		t.Fatal("overlay layout dropped the host line")
	}
	osc := pixelsNear(tt.Image(), ui.RGB(20, 200, 40))
	if osc < 80 {
		t.Fatalf("host sub-pane pixels %d", osc)
	}
	if err := c.SetIndicator("HOST", false); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("HOST ") {
		t.Fatal("turned-off host indicator stayed in the readout")
	}
}

func TestSyncIndicatorsPicksUpLateRegistration(t *testing.T) {
	c := kline.NewChart(kline.ChartOptions{Symbols: []string{"ALPHA"}, Indicators: []string{}})
	had := false
	for _, name := range c.IndicatorNames() {
		if name == "HOSTLATE" {
			had = true
		}
	}
	err := kline.Register(kline.Spec{
		Name: "HOSTLATE",
		Kind: kline.PaneMain,
		New:  func() kline.Indicator { return &hostLine{color: ui.RGB(255, 140, 0)} },
	})
	if err != nil && !errors.Is(err, kline.ErrDuplicateIndicator) {
		t.Fatal(err)
	}
	c.SyncIndicators()
	found := false
	for _, name := range c.IndicatorNames() {
		if name == "HOSTLATE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("names after sync %v", c.IndicatorNames())
	}
	if !had && c.IndicatorOn("HOSTLATE") {
		t.Fatal("a synced indicator should start off")
	}
	if err := c.SetIndicator("HOSTLATE", true); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(c.View, 960, 720)
	if !tt.HasText("HOST ") {
		t.Fatalf("synced indicator did not paint, texts %q", tt.Texts())
	}
}

func TestHostOverlayTool(t *testing.T) {
	err := kline.RegisterOverlay(kline.OverlaySpec{
		Kind:    "hostmark",
		Anchors: 1,
		Draw: func(p *ui.Painter, plot ui.Rect, o kline.Overlay, vp kline.Viewport, scale chart.Linear, cs []kline.Candle, _ kline.Style) {
			if len(o.Points) == 0 {
				return
			}
			x := plot.X + float32(kline.AnchorX(o.Points[0], vp, cs))
			y := plot.Y + float32(kline.PriceToLocalY(float64(plot.H), o.Points[0].Price, scale))
			p.Fill(ui.Rect{X: x - 5, Y: y - 5, W: 10, H: 10}, ui.RGB(0, 220, 255), 1)
		},
		Hit: func(o kline.Overlay, x, y, _, plotH float64, vp kline.Viewport, scale chart.Linear, cs []kline.Candle) (int, bool) {
			if len(o.Points) == 0 {
				return 0, false
			}
			ax := kline.AnchorX(o.Points[0], vp, cs)
			ay := kline.PriceToLocalY(plotH, o.Points[0].Price, scale)
			if math.Hypot(x-ax, y-ay) <= kline.HitRadius+4 {
				return kline.HitBody, true
			}
			return 0, false
		},
	})
	if err != nil && !errors.Is(err, kline.ErrDuplicateOverlay) {
		t.Fatal(err)
	}
	if err := kline.RegisterOverlay(kline.OverlaySpec{
		Kind:    kline.OverlayHLine,
		Anchors: 1,
		Draw:    func(*ui.Painter, ui.Rect, kline.Overlay, kline.Viewport, chart.Linear, []kline.Candle, kline.Style) {},
		Hit: func(kline.Overlay, float64, float64, float64, float64, kline.Viewport, chart.Linear, []kline.Candle) (int, bool) {
			return 0, false
		},
	}); err == nil {
		t.Fatal("hline is reserved")
	}
	c := kline.NewChart(kline.ChartOptions{Symbols: []string{"ALPHA"}, Indicators: []string{}})
	if err := c.SetTool("hostmark"); err != nil {
		t.Fatal(err)
	}
	if c.Tool() != "hostmark" {
		t.Fatalf("tool %s", c.Tool())
	}
	if err := c.SetTool("not-a-tool"); err == nil {
		t.Fatal("unknown tool should fail")
	}
	if c.Tool() != "hostmark" {
		t.Fatal("failed SetTool changed the tool")
	}
	tt := ui.NewTester(c.View, 960, 720)
	box, ok := tt.Find("ALPHA chart")
	if !ok {
		t.Fatal("plot missing")
	}
	x, y := box.X+140, box.Y+box.H*0.4
	tt.ClickAt(x, y)
	if pixelsNear(tt.Image(), ui.RGB(0, 220, 255)) < 8 {
		t.Fatal("host overlay did not draw")
	}
	tt.RightClickAt(x, y)
	tt.Frame()
	if pixelsNear(tt.Image(), ui.RGB(0, 220, 255)) > 4 {
		t.Fatal("right-click should remove the host overlay under the pointer")
	}
}

func TestHostLiveSourceAppendsBar(t *testing.T) {
	period := kline.Period{Span: 1, Unit: kline.UnitHour}
	feed := newHostFeed(period, "ALPHA")
	c := kline.NewChart(kline.ChartOptions{
		Source:     feed,
		Symbols:    []string{"ALPHA"},
		Period:     period,
		Indicators: []string{},
	})
	tt := ui.NewTester(c.View, 960, 720)
	before := c.Bars()
	last, ok := c.Bar("ALPHA", before-1)
	if !ok {
		t.Fatal("missing last bar")
	}
	feed.Push("ALPHA", kline.Candle{
		Time: last.Time.Add(time.Hour), Open: last.Close, High: last.Close + 1, Low: last.Close - 1, Close: last.Close + 0.5, Volume: 10,
	})
	tt.Frame()
	if c.Bars() != before+1 {
		t.Fatalf("bars %d, was %d", c.Bars(), before)
	}
	got, ok := c.Bar("ALPHA", c.Bars()-1)
	if !ok || !got.Time.Equal(last.Time.Add(time.Hour)) {
		t.Fatalf("last bar %+v", got)
	}
}

func TestBuiltInToolsStillPanAndDraw(t *testing.T) {
	c := kline.NewChart(kline.ChartOptions{
		Symbols:    []string{"ALPHA", "BETA"},
		Indicators: []string{"MA", "MACD"},
	})
	tt := ui.NewTester(c.View, 960, 720)
	box, ok := tt.Find("ALPHA chart")
	if !ok {
		t.Fatal("plot missing")
	}
	_, idx0, ok := func() (string, int, bool) {
		tt.Move(box.X+80, box.Y+30)
		return c.Crosshair()
	}()
	if !ok {
		t.Fatal("no crosshair")
	}
	first, _ := c.Visible()
	tt.Press(box.X+80, box.Y+30)
	tt.Move(box.X+180, box.Y+30)
	tt.Release(box.X+180, box.Y+30)
	first2, _ := c.Visible()
	if first2 >= first {
		t.Fatalf("pan should reveal earlier bars: %d -> %d", first, first2)
	}
	if err := c.SetTool(kline.ToolHLine); err != nil {
		t.Fatal(err)
	}
	tt.ClickAt(box.X+120, box.Y+50)
	if err := c.SetTool(kline.ToolPan); err != nil {
		t.Fatal(err)
	}
	tt.Move(box.X+120, box.Y+40)
	_, idx1, ok := c.Crosshair()
	if !ok || idx1 < 0 {
		t.Fatal("crosshair lost after tool switch")
	}
	_ = idx0
	c.SetLayout(kline.LayoutOverlay)
	tt.Frame()
	if c.Layout() != kline.LayoutOverlay {
		t.Fatalf("layout %s", c.Layout())
	}
	if _, ok := tt.Find("Compare chart"); !ok {
		t.Fatal("overlay plot missing")
	}
	if err := c.SetIndicator("MACD", false); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if c.IndicatorOn("MACD") {
		t.Fatal("MACD stayed on")
	}
}

func pixelsNear(img *image.RGBA, target ui.Color) int {
	if img == nil {
		return 0
	}
	b := img.Bounds()
	n := 0
	tr, tg, tb := int(target.R), int(target.G), int(target.B)
	for y := b.Min.Y; y < b.Max.Y; y += 1 {
		for x := b.Min.X; x < b.Max.X; x += 1 {
			c := img.RGBAAt(x, y)
			dr := int(c.R) - tr
			dg := int(c.G) - tg
			db := int(c.B) - tb
			if dr*dr+dg*dg+db*db <= 28*28 {
				n++
			}
		}
	}
	return n
}
