package kline

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

// frameTool is a host-registered one-point drawing that paints a
// rectangle in a color taken from Overlay.Data, the shape a host uses to
// mark a forming weekly bar.
func frameTool() OverlaySpec {
	return OverlaySpec{
		Kind:    "hostframe",
		Anchors: 1,
		Draw: func(p *ui.Painter, plot ui.Rect, o Overlay, vp Viewport, scale chart.Linear, cs []Candle, style Style) {
			if len(o.Points) == 0 {
				return
			}
			c, ok := o.Data.(ui.Color)
			if !ok {
				return
			}
			x := plot.X + float32(AnchorX(o.Points[0], vp, cs))
			y := plot.Y + float32(PriceToLocalY(float64(plot.H), o.Points[0].Price, scale))
			p.Stroke(ui.Rect{X: x - 6, Y: y - 6, W: 12, H: 12}, c, 0, 2)
		},
		Hit: func(o Overlay, x, y, _, plotH float64, vp Viewport, scale chart.Linear, cs []Candle) (int, bool) {
			return HitBody, false
		},
	}
}

// TestSetOverlaysInjectsAHostDrawing is the gap that blocked REQ-11: a
// host could register a tool but had no way to place one without a
// click, so a forming-bar frame could not be drawn during a replay.
func TestSetOverlaysInjectsAHostDrawing(t *testing.T) {
	unregisterOverlayForTest(t, "hostframe")
	if err := RegisterOverlay(frameTool()); err != nil {
		t.Fatalf("register: %v", err)
	}
	hot := ui.RGB(9, 8, 7)
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
	})
	bar, _ := c.Bar("A", c.Bars()-1)
	err := c.SetOverlays("A", []Overlay{{
		ID:     "forming",
		Kind:   "hostframe",
		Points: []Point{{Time: bar.Time, Price: bar.Close}},
		Data:   hot,
	}})
	if err != nil {
		t.Fatalf("SetOverlays: %v", err)
	}
	tt := ui.NewTester(c.View, 900, 620)
	img := tt.Image()
	if countColor(img, hot) == 0 {
		t.Fatal("the injected drawing never reached the frame")
	}
	// The list reads back with the host's data intact.
	got, err := c.Overlays("A")
	if err != nil {
		t.Fatalf("Overlays: %v", err)
	}
	if len(got) != 1 || got[0].ID != "forming" || got[0].Data != hot {
		t.Fatalf("read back %+v", got)
	}
}

// TestSetOverlaysReplacesAndClears records the replace semantics: one ID
// rewritten each reveal, not a growing list.
func TestSetOverlaysReplacesAndClears(t *testing.T) {
	unregisterOverlayForTest(t, "hostframe")
	if err := RegisterOverlay(frameTool()); err != nil {
		t.Fatalf("register: %v", err)
	}
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(90)},
		Location:   ashareLocation,
	})
	bar, _ := c.Bar("A", c.Bars()-1)
	one := []Overlay{{ID: "forming", Kind: "hostframe", Points: []Point{{Time: bar.Time, Price: bar.Close}}}}
	for i := 0; i < 3; i++ {
		if err := c.SetOverlays("A", one); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
	}
	if got, _ := c.Overlays("A"); len(got) != 1 {
		t.Fatalf("three sets left %d drawings", len(got))
	}
	if err := c.SetOverlays("A", nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ := c.Overlays("A"); len(got) != 0 {
		t.Fatalf("clear left %d drawings", len(got))
	}
}

// TestSetOverlaysUnknownSymbol records the error a host can act on.
func TestSetOverlaysUnknownSymbol(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(40)},
		Location:   ashareLocation,
	})
	if err := c.SetOverlays("NOPE", nil); !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("unknown symbol err %v", err)
	}
	if _, err := c.Overlays("NOPE"); !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("unknown symbol read err %v", err)
	}
}

// TestSetOverlaysCopiesPoints records that the host may reuse its slice.
func TestSetOverlaysCopiesPoints(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(40)},
		Location:   ashareLocation,
	})
	bar, _ := c.Bar("A", 5)
	pts := []Point{{Time: bar.Time, Price: bar.Close}}
	if err := c.SetOverlays("A", []Overlay{{Kind: OverlayHLine, Points: pts}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	pts[0].Price = -1
	if got, _ := c.Overlays("A"); got[0].Points[0].Price != bar.Close {
		t.Fatalf("the layer kept the host's slice: %+v", got[0].Points)
	}
}

// unregisterOverlayForTest keeps a registration from leaking into
// another test, since the overlay registry is process-wide.
func unregisterOverlayForTest(t *testing.T, kind string) {
	t.Helper()
	UnregisterOverlay(kind)
	t.Cleanup(func() { UnregisterOverlay(kind) })
}

// TestUpsertOverlaysKeepsOtherDrawings is the reason the incremental API
// exists: a host with two independent sources, a forming-week frame and a
// set of fill markers, must be able to update one without dropping the
// other. SetOverlays replaces the layer, so using it for both means every
// call has to rebuild the union.
func TestUpsertOverlaysKeepsOtherDrawings(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(60)},
		Location:   ashareLocation,
	})
	bar, _ := c.Bar("A", 10)
	week := Overlay{ID: "forming", Kind: OverlayHLine, Points: []Point{{Time: bar.Time, Price: bar.Close}}}
	fill := Overlay{ID: "fill-1", Kind: OverlayHLine, Points: []Point{{Time: bar.Time, Price: bar.Low}}}
	if err := c.UpsertOverlays("A", []Overlay{week}); err != nil {
		t.Fatalf("week: %v", err)
	}
	if err := c.UpsertOverlays("A", []Overlay{fill}); err != nil {
		t.Fatalf("fill: %v", err)
	}
	got, _ := c.Overlays("A")
	if len(got) != 2 {
		t.Fatalf("after two sources the layer holds %d, want 2: %+v", len(got), got)
	}
	// Updating the week must not touch the fill.
	moved := Overlay{ID: "forming", Kind: OverlayHLine, Points: []Point{{Price: bar.Close + 1}}}
	if err := c.UpsertOverlays("A", []Overlay{moved}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = c.Overlays("A")
	if len(got) != 2 {
		t.Fatalf("update changed the count to %d", len(got))
	}
	if got[0].ID != "forming" || got[0].Points[0].Price != bar.Close+1 {
		t.Fatalf("the week drawing was not replaced in place: %+v", got[0])
	}
	if got[1].ID != "fill-1" {
		t.Fatalf("the fill drawing moved or vanished: %+v", got[1])
	}
	// Removing one leaves the other.
	if err := c.RemoveOverlays("A", "forming"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	got, _ = c.Overlays("A")
	if len(got) != 1 || got[0].ID != "fill-1" {
		t.Fatalf("after remove: %+v", got)
	}
	// A missing ID is not an error, so an optional drawing needs no check.
	if err := c.RemoveOverlays("A", "nope"); err != nil {
		t.Fatalf("removing an unknown id: %v", err)
	}
}

// TestUpsertAndRemoveRejectUnknownSymbol records the error contract.
func TestUpsertAndRemoveRejectUnknownSymbol(t *testing.T) {
	c := hostChart(t, ChartOptions{
		Symbols:    []string{"A"},
		Period:     Period{Span: 1, Unit: UnitDay},
		Indicators: []string{},
		Source:     fixedSource{bars: hostBars(30)},
		Location:   ashareLocation,
	})
	if err := c.UpsertOverlays("NOPE", nil); !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("upsert err %v", err)
	}
	if err := c.RemoveOverlays("NOPE", "x"); !errors.Is(err, ErrUnknownSymbol) {
		t.Fatalf("remove err %v", err)
	}
}
