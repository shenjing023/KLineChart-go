package kline

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func TestPricePixelRoundtrip(t *testing.T) {
	scale := chart.NewLinear(80, 120)
	const plotH = 240.0
	for _, price := range []float64{80, 100, 119.5, 95.25} {
		y := PriceToLocalY(plotH, price, scale)
		if y < -0.01 || y > plotH+0.01 {
			t.Fatalf("price %v mapped to y %v outside the plot", price, y)
		}
		got := LocalYToPrice(plotH, y, scale)
		if math.Abs(got-price) > 1e-3 {
			t.Fatalf("price %v -> y %v -> %v", price, y, got)
		}
	}
	// A higher price sits above a lower one: smaller local Y.
	if PriceToLocalY(plotH, 110, scale) >= PriceToLocalY(plotH, 90, scale) {
		t.Fatal("higher price should map to a smaller local Y")
	}
}

func TestBarCenterIndexRoundtrip(t *testing.T) {
	vp := Viewport{Offset: 3.25, BarWidth: 8}
	for i := 0; i < 40; i++ {
		x := AnchorX(Point{BarIndex: i}, vp, nil)
		got, ok := vp.IndexAt(x, 80)
		if !ok || got != i {
			t.Fatalf("bar %d center x %v -> %d ok=%v", i, x, got, ok)
		}
	}
}

func TestHLineHitRadius(t *testing.T) {
	scale := chart.NewLinear(0, 100)
	const plotW, plotH = 400.0, 200.0
	vp := Viewport{BarWidth: 8}
	line := Overlay{ID: "h", Kind: OverlayHLine, Points: []Point{{Price: 50}}}
	y := PriceToLocalY(plotH, 50, scale)

	part, ok := HitOverlay(line, 10, y, plotW, plotH, vp, scale, nil)
	if !ok || part != HitBody {
		t.Fatalf("on the line: part %d ok %v", part, ok)
	}
	if _, ok := HitOverlay(line, 10, y+HitRadius, plotW, plotH, vp, scale, nil); !ok {
		t.Fatal("inside the hit radius")
	}
	if _, ok := HitOverlay(line, plotW-1, y-HitRadius, plotW, plotH, vp, scale, nil); !ok {
		t.Fatal("inside the radius at the far end of the plot")
	}
	if _, ok := HitOverlay(line, 10, y+HitRadius+1.5, plotW, plotH, vp, scale, nil); ok {
		t.Fatal("outside the hit radius")
	}
	if _, ok := HitOverlay(line, -1, y, plotW, plotH, vp, scale, nil); ok {
		t.Fatal("left of the plot")
	}
	if _, ok := HitOverlay(line, plotW+1, y, plotW, plotH, vp, scale, nil); ok {
		t.Fatal("right of the plot")
	}
}

func TestSegmentHitEndpointAndBody(t *testing.T) {
	scale := chart.NewLinear(0, 100)
	const plotH = 200.0
	vp := Viewport{Offset: 0, BarWidth: 10}
	seg := Overlay{ID: "s", Kind: OverlaySegment, Points: []Point{
		{BarIndex: 2, Price: 80},
		{BarIndex: 8, Price: 20},
	}}
	x0 := AnchorX(seg.Points[0], vp, nil)
	y0 := PriceToLocalY(plotH, 80, scale)
	x1 := AnchorX(seg.Points[1], vp, nil)
	y1 := PriceToLocalY(plotH, 20, scale)

	part, ok := HitOverlay(seg, x0, y0, 400, plotH, vp, scale, nil)
	if !ok || part != 0 {
		t.Fatalf("first endpoint: part %d ok %v", part, ok)
	}
	part, ok = HitOverlay(seg, x1+HitRadius-0.5, y1, 400, plotH, vp, scale, nil)
	if !ok || part != 1 {
		t.Fatalf("second endpoint: part %d ok %v", part, ok)
	}
	part, ok = HitOverlay(seg, (x0+x1)/2, (y0+y1)/2, 400, plotH, vp, scale, nil)
	if !ok || part != HitBody {
		t.Fatalf("segment body: part %d ok %v", part, ok)
	}
	if _, ok := HitOverlay(seg, x0, y0+40, 400, plotH, vp, scale, nil); ok {
		t.Fatal("point well off the segment")
	}
}

func TestHitLayerIsPerPaneAndTopmost(t *testing.T) {
	scale := chart.NewLinear(0, 100)
	const plotW, plotH = 300.0, 200.0
	vp := Viewport{BarWidth: 8}
	var alpha, beta Layer
	a := alpha.Add(OverlayHLine, Point{Price: 40})
	beta.Add(OverlayHLine, Point{Price: 40})
	y := PriceToLocalY(plotH, 40, scale)

	hit, ok := alpha.Hit(10, y, plotW, plotH, vp, scale, nil)
	if !ok || hit.ID != a.ID {
		t.Fatalf("alpha hit %+v, want its own %s", hit, a.ID)
	}
	if len(alpha.Items()) != 1 || len(beta.Items()) != 1 {
		t.Fatalf("layers leaked: alpha %d beta %d", len(alpha.Items()), len(beta.Items()))
	}
	// Same price: the last overlay added to this pane wins.
	beta.Add(OverlayHLine, Point{Price: 40})
	hit, ok = beta.Hit(10, y, plotW, plotH, vp, scale, nil)
	items := beta.Items()
	if !ok || hit.ID != items[len(items)-1].ID {
		t.Fatalf("topmost hit %+v, want %s", hit, items[len(items)-1].ID)
	}
	if hit.ID != a.ID && len(alpha.Items()) != 1 {
		t.Fatal("hitting beta changed alpha")
	}
	if _, ok := alpha.Hit(10, y+80, plotW, plotH, vp, scale, nil); ok {
		t.Fatal("alpha should miss a point far from its line")
	}
}

func TestPointTimeWinsOverStaleIndex(t *testing.T) {
	start := time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)
	cs := make([]Candle, 6)
	for i := range cs {
		cs[i].Time = start.Add(time.Duration(i) * time.Hour)
	}
	// The point was placed on bar 1. After a prepend it still names that
	// open time, while BarIndex is the old index and would be wrong.
	pt := Point{BarIndex: 1, Time: cs[4].Time, Price: 10}
	got, ok := pt.Index(cs)
	if !ok || got != 4 {
		t.Fatalf("index %d ok %v, want time match at 4", got, ok)
	}
	vp := Viewport{Offset: 0, BarWidth: 10}
	x := AnchorX(pt, vp, cs)
	want := vp.BarLeft(4) + vp.BarWidth/2
	if math.Abs(x-want) > 1e-9 {
		t.Fatalf("anchor x %v, want bar 4 center %v", x, want)
	}
}

func TestLayerSelectAndDelete(t *testing.T) {
	var l Layer
	first := l.Add(OverlayHLine, Point{Price: 1})
	second := l.Add(OverlaySegment, Point{BarIndex: 1, Price: 2}, Point{BarIndex: 3, Price: 4})
	if first.ID == second.ID {
		t.Fatal("ids should differ")
	}
	if sel := l.Selected(); sel == nil || sel.ID != second.ID || !sel.Selected {
		t.Fatalf("adding selects the new overlay, got %+v", sel)
	}
	if l.ByID(first.ID).Selected {
		t.Fatal("the previous overlay should lose selection")
	}
	l.Select(first.ID)
	if sel := l.Selected(); sel == nil || sel.ID != first.ID {
		t.Fatalf("select %+v", sel)
	}
	if !l.SetPoint(second.ID, 0, Point{BarIndex: 2, Price: 9}) {
		t.Fatal("set point")
	}
	if got := l.ByID(second.ID).Points[0]; got.BarIndex != 2 || got.Price != 9 {
		t.Fatalf("point %+v", got)
	}
	if !l.Delete(first.ID) {
		t.Fatal("delete")
	}
	if l.ByID(first.ID) != nil {
		t.Fatal("deleted overlay still present")
	}
	if l.Selected() != nil {
		t.Fatalf("deleted selection stuck: %+v", l.Selected())
	}
	if l.Delete("missing") {
		t.Fatal("missing delete")
	}
}

func TestOverlayRegistryReadRacesRegister(t *testing.T) {
	const n = 80
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("raceov%04d", i)
	}
	t.Cleanup(func() {
		for _, name := range names {
			UnregisterOverlay(name)
		}
	})
	draw := func(*ui.Painter, ui.Rect, Overlay, Viewport, chart.Linear, []Candle, Style) {}
	hit := func(Overlay, float64, float64, float64, float64, Viewport, chart.Linear, []Candle) (int, bool) {
		return 0, false
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, name := range names {
			_ = RegisterOverlay(OverlaySpec{Kind: name, Anchors: 1, Draw: draw, Hit: hit})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			_, _ = overlayByKind(names[i%len(names)])
		}
	}()
	wg.Wait()
	UnregisterOverlay(names[0])
	if _, ok := overlayByKind(names[0]); ok {
		t.Fatal("unregistered overlay still looked up")
	}
	if err := RegisterOverlay(OverlaySpec{Kind: names[0], Anchors: 1, Draw: draw, Hit: hit}); err != nil {
		t.Fatal(err)
	}
}
