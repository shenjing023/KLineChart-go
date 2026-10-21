package kline

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func TestLayoutPanesMatchBuiltinSplit(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 800, H: 500}
	cases := []struct {
		inds      []string
		macd, kdj bool
	}{
		{[]string{}, false, false},
		{[]string{"MACD"}, true, false},
		{[]string{"KDJ"}, false, true},
		{[]string{"MACD", "KDJ"}, true, true},
		{[]string{"MA", "MACD", "BOLL", "KDJ"}, true, true},
	}
	for _, tc := range cases {
		c := NewChart(ChartOptions{Indicators: tc.inds, Symbols: []string{"600519"}})
		got := c.layoutPanes(r, true)
		want := SplitStack(r, true, tc.macd, tc.kdj)
		if got.Price != want.Price || got.Volume != want.Volume || got.MACD != want.MACD || got.KDJ != want.KDJ || got.Time != want.Time || got.PriceAxis != want.PriceAxis || got.VolumeAxis != want.VolumeAxis || got.MACDAxis != want.MACDAxis || got.KDJAxis != want.KDJAxis {
			t.Fatalf("inds %v\ngot %+v\nwant %+v", tc.inds, got, want)
		}
		if len(got.Extra) != 0 || len(got.ExtraAxis) != 0 {
			t.Fatalf("inds %v leaked extra %+v", tc.inds, got.Extra)
		}
	}
}

func TestSplitManyStacksSubPanes(t *testing.T) {
	r := ui.Rect{X: 10, Y: 20, W: 800, H: 640}
	panes := splitMany(r, true, 3)
	if panes.Price.H <= 1 || panes.Volume.H <= 1 || panes.Time.H <= 0 || len(panes.Extra) != 3 {
		t.Fatalf("bands %+v", panes)
	}
	if panes.Volume.Y < panes.Price.Y+panes.Price.H {
		t.Fatal("volume overlaps price")
	}
	prev := panes.Volume
	for i, band := range panes.Extra {
		if band.H <= 1 || band.Y < prev.Y+prev.H || band.W != panes.Price.W {
			t.Fatalf("sub %d %+v prev %+v", i, band, prev)
		}
		if panes.ExtraAxis[i].X != band.X+band.W || panes.ExtraAxis[i].H != band.H {
			t.Fatalf("axis %d %+v", i, panes.ExtraAxis[i])
		}
		prev = band
	}
	if panes.Time.Y < prev.Y+prev.H-0.5 {
		t.Fatalf("time %+v last %+v", panes.Time, prev)
	}
}

type countedSub struct {
	name string
	draw int
}

func (s *countedSub) Name() string                                        { return s.name }
func (*countedSub) PaneKind() string                                      { return PaneSub }
func (s *countedSub) Calc([]Candle)                                       {}
func (*countedSub) DrawMain(*ui.Painter, ui.Rect, Viewport, chart.Linear) {}
func (s *countedSub) DrawSub(*ui.Painter, ui.Rect, Viewport)              { s.draw++ }
func (*countedSub) ValueAt(int) string                                    { return "" }

func TestSecondSameKindSubPaneGetsOwnBand(t *testing.T) {
	const name = "TEST_FASTMACD"
	if err := Register(Spec{
		Name: name,
		Kind: PaneSub,
		New:  func() Indicator { return NewMACD(3, 6, 2) },
	}); err != nil {
		t.Fatal(err)
	}
	defer Unregister(name)
	c := NewChart(ChartOptions{
		Symbols:    []string{"A"},
		Indicators: []string{"MACD", name},
	})
	panes := c.layoutPanes(ui.Rect{W: 900, H: 600}, true)
	inds := c.active(&c.panes[0])
	layers := subLayers(panes, inds, 0, c.Bars()-1)
	if len(layers) != 2 {
		t.Fatalf("two enabled sub-pane indicators yielded %d layers", len(layers))
	}
	if layers[0].plot != panes.MACD {
		t.Fatalf("built-in MACD band %+v, want %+v", layers[0].plot, panes.MACD)
	}
	if len(panes.Extra) != 1 || layers[1].plot != panes.Extra[0] {
		t.Fatalf("second MACD extra bands %+v, layers %+v", panes.Extra, layers)
	}
	if layers[0].plot == layers[1].plot || layers[0].plot.Y+layers[0].plot.H > layers[1].plot.Y {
		t.Fatalf("sub-pane bands overlap: %+v and %+v", layers[0].plot, layers[1].plot)
	}
}

func TestSameKindSubPanePaintsEveryDrawSub(t *testing.T) {
	const name = "TEST_SECOND_MACD"
	if err := Register(Spec{
		Name: name,
		Kind: PaneSub,
		New:  func() Indicator { return NewMACD(3, 6, 2) },
	}); err != nil {
		t.Fatal(err)
	}
	defer Unregister(name)
	c := NewChart(ChartOptions{
		Symbols:    []string{"A"},
		Indicators: []string{"MACD", name},
	})
	tt := ui.NewTester(c.View, 900, 700)
	tt.Frame()
	active := c.active(&c.panes[0])
	if len(active) != 2 {
		t.Fatalf("active indicators %d", len(active))
	}
	panes := c.painted[0]
	layers := subLayers(panes, active, 0, c.Bars()-1)
	if len(layers) != 2 || layers[1].ind != active[1] {
		t.Fatalf("painted layers %+v, active %+v", layers, active)
	}
	style := Style{MACDDIF: ui.RGB(1, 2, 3), MACDDEA: ui.RGB(4, 5, 6)}
	applyIndicatorStyle(active, panes, style)
	for i, ind := range active {
		macd, ok := ind.(*MACD)
		if !ok {
			t.Fatalf("active %d is %T", i, ind)
		}
		if macd.DIFColor != style.MACDDIF || macd.DEAColor != style.MACDDEA {
			t.Fatalf("MACD %d kept colors %+v/%+v, want the theme", i, macd.DIFColor, macd.DEAColor)
		}
	}
}

type nilSeriesIndicator struct {
	draw int
}

func (*nilSeriesIndicator) Name() string     { return "TEST_NIL_SERIES" }
func (*nilSeriesIndicator) PaneKind() string { return PaneMain }
func (*nilSeriesIndicator) Calc([]Candle)    {}
func (s *nilSeriesIndicator) DrawMain(p *ui.Painter, r ui.Rect, _ Viewport, _ chart.Linear) {
	s.draw++
	p.Fill(ui.Rect{X: r.X + 10, Y: r.Y + 10, W: 10, H: 10}, ui.RGB(255, 0, 255), 0)
}
func (*nilSeriesIndicator) DrawSub(*ui.Painter, ui.Rect, Viewport) {}
func (*nilSeriesIndicator) ValueAt(int) string                     { return "" }
func (*nilSeriesIndicator) Series() [][]float64                    { return nil }
func (*nilSeriesIndicator) LineColors() []ui.Color                 { return nil }

type emptySeriesIndicator struct {
	nilSeriesIndicator
}

func (*emptySeriesIndicator) Name() string           { return "TEST_EMPTY_SERIES" }
func (*emptySeriesIndicator) Series() [][]float64    { return [][]float64{} }
func (*emptySeriesIndicator) LineColors() []ui.Color { return nil }

func TestNilSeriesLinePlotFallsBackToDrawMain(t *testing.T) {
	nilLines := &nilSeriesIndicator{}
	emptyLines := &emptySeriesIndicator{}
	panes := Split(ui.Rect{W: 400, H: 300})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, _ ui.Rect) {
			Paint(p, panes, Generate(30, 1), []Indicator{nilLines}, Viewport{BarWidth: 8}, Pointer{}, -1, DarkDefault())
			Paint(p, panes, Generate(30, 1), []Indicator{emptyLines}, Viewport{BarWidth: 8}, Pointer{}, -1, DarkDefault())
		})
	}, 400, 300)
	tt.Frame()
	if nilLines.draw < 1 {
		t.Fatal("nil Series did not call DrawMain")
	}
	if emptyLines.draw != 0 {
		t.Fatalf("non-nil empty Series called DrawMain %d times", emptyLines.draw)
	}
}

func TestBuiltInIndicatorNamesStayReserved(t *testing.T) {
	for _, name := range []string{"MA", "MACD", "BOLL", "KDJ"} {
		Unregister(name)
		found := false
		for _, spec := range Registry() {
			if spec.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Unregister removed built-in %s", name)
		}
		err := Register(Spec{
			Name: name,
			Kind: PaneSub,
			New:  func() Indicator { return NewMACD(12, 26, 9) },
		})
		if err == nil {
			t.Fatalf("Register replaced built-in %s", name)
		}
	}
	c := NewChart(ChartOptions{Indicators: []string{"MA"}})
	if !c.IndicatorOn("MA") || c.IndicatorEnabled("MA") == nil {
		t.Fatal("new chart lost the built-in MA after Unregister")
	}
}

// The band order layout uses, the band paint uses, and the band the theme
// colors use must be one order. A host sub-pane before the built-in MACD
// would otherwise shift every later band by one.
func TestSubBandsMatchLayoutPaintAndStyle(t *testing.T) {
	const name = "TEST_LEADING_SUB"
	if err := Register(Spec{
		Name: name,
		Kind: PaneSub,
		New:  func() Indicator { return &countedSub{name: name} },
	}); err != nil {
		t.Fatal(err)
	}
	defer Unregister(name)
	c := NewChart(ChartOptions{
		Symbols:    []string{"A"},
		Indicators: []string{name, "MACD", "KDJ"},
	})
	tt := ui.NewTester(c.View, 900, 700)
	tt.Frame()
	active := c.active(&c.panes[0])
	panes := c.painted[0]
	// The registry order is MACD, KDJ, then the host name, so the bands
	// are the two built-ins and one extra. What matters is that the three
	// consumers agree on that order.
	bands := subBands(active)
	want := make([]subBand, len(active))
	for i, ind := range active {
		switch ind.(type) {
		case *MACD:
			want[i] = bandMACD
		case *KDJ:
			want[i] = bandKDJ
		default:
			want[i] = bandExtra
		}
	}
	if len(bands) != len(want) {
		t.Fatalf("bands %v, want %v", bands, want)
	}
	for i := range want {
		if bands[i] != want[i] {
			t.Fatalf("bands %v, want %v", bands, want)
		}
	}
	layers := subLayers(panes, active, 0, c.Bars()-1)
	if len(layers) != 3 {
		t.Fatalf("layers %d, want 3", len(layers))
	}
	// Recompute the rect each band should get from the layout's own
	// rects, and require paint to have used it.
	extra := 0
	for i, band := range bands {
		var plot ui.Rect
		switch band {
		case bandMACD:
			plot = panes.MACD
		case bandKDJ:
			plot = panes.KDJ
		default:
			plot = panes.Extra[extra]
			extra++
		}
		if layers[i].plot != plot {
			t.Fatalf("layer %d band %v painted %+v, layout put it at %+v", i, band, layers[i].plot, plot)
		}
	}
	// applyIndicatorStyle must reach the built-in MACD and KDJ in their
	// own bands, even with a host sub-pane in the list.
	style := Style{MACDDIF: ui.RGB(1, 2, 3), KDJK: ui.RGB(4, 5, 6)}
	applyIndicatorStyle(active, panes, style)
	for _, ind := range active {
		switch v := ind.(type) {
		case *MACD:
			if v.DIFColor != style.MACDDIF {
				t.Fatalf("MACD colors %+v, want the theme", v.DIFColor)
			}
		case *KDJ:
			if v.KColor != style.KDJK {
				t.Fatalf("KDJ colors %+v, want the theme", v.KColor)
			}
		}
	}
}
