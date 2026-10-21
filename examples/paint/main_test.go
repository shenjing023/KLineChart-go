package main

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func TestPaintViewDrawsHostOwnedPane(t *testing.T) {
	state := newPaintState()
	tt := ui.NewTester(paintView(state), 1000, 760)
	tt.Frame()
	if tt.Image() == nil {
		t.Fatal("no frame")
	}
	if !tt.HasText("PAINT API") {
		t.Fatalf("host chrome missing: %q", tt.Texts())
	}
	if len(state.bars) == 0 {
		t.Fatal("fixture has no bars")
	}
	box, ok := tt.Find("paint plot")
	if !ok {
		t.Fatal("host plot missing")
	}
	before := state.vp.BarWidth
	tt.Scroll(box.X+box.W/2, box.Y+box.H/2, 0, -360)
	if state.vp.BarWidth <= before {
		t.Fatalf("wheel did not zoom in: %v -> %v", before, state.vp.BarWidth)
	}
}

func TestPaintViewClearsPriceAxisHover(t *testing.T) {
	state := newPaintState()
	tt := ui.NewTester(paintView(state), 1000, 760)
	tt.Frame()
	box, ok := tt.Find("paint plot")
	if !ok {
		t.Fatal("host plot missing")
	}
	tt.Move(box.X+box.W/2, box.Y+box.H/3)
	if !state.ptr.On || state.index < 0 {
		t.Fatal("plot hover missing")
	}
	tt.Move(box.X+box.W-10, box.Y+box.H/3)
	if state.ptr.On || state.index != -1 {
		t.Fatalf("price axis retained hover: pointer=%+v index=%d", state.ptr, state.index)
	}
}

func TestPaintViewPointerUsesAbsoluteDrawCoordinates(t *testing.T) {
	state := newPaintState()
	tt := ui.NewTester(paintView(state), 1000, 760)
	tt.Frame()
	box, ok := tt.Find("paint plot")
	if !ok {
		t.Fatal("host plot missing")
	}
	x, y := box.X+box.W/2, box.Y+box.H/3
	tt.Move(x, y)
	if !state.ptr.On || state.ptr.X != x || state.ptr.Y != y {
		t.Fatalf("draw pointer %+v, want absolute (%v,%v)", state.ptr, x, y)
	}
	if box.X <= 0 || box.Y <= 0 || state.local.X != x-box.X || state.local.Y != y-box.Y {
		t.Fatalf("local %+v, absolute %+v, host %+v", state.local, state.ptr, box)
	}
	want, ok := state.vp.IndexAt(float64(x-state.panes.Price.X), len(state.bars))
	if !ok || state.index != want {
		t.Fatalf("hover bar %d, want %d", state.index, want)
	}
	line := ui.Rect{X: state.panes.Price.X + 40, Y: y - 2, W: state.panes.Price.W - 80, H: 4}
	if got := countColor(tt.Image(), line, kline.DarkDefault().Cross); got < int(line.W)/2 {
		t.Fatalf("horizontal crosshair missing at absolute Y=%v: %d pixels", y, got)
	}
	if !tt.HasText(state.readout()) || !strings.Contains(state.readout(), "UTC") {
		t.Fatalf("hover readout missing: %q", tt.Texts())
	}
}

func newPaintTester(t *testing.T) (*paintState, *ui.Tester) {
	t.Helper()
	s := newPaintState()
	tt := ui.NewTester(paintView(s), 1000, 760)
	tt.Frame()
	if s.panes.Price.W < 2 || !s.fitted {
		t.Fatal("plot was not laid out and fitted")
	}
	return s, tt
}

func TestPaintViewPanAndRelease(t *testing.T) {
	s, tt := newPaintTester(t)
	r := s.panes.Price
	x, y := r.X+r.W/2, r.Y+r.H/3
	before := s.vp
	tt.Press(x, y)
	tt.Move(x+64, y)
	if math.Abs(s.vp.Offset-(before.Offset-64/before.BarWidth)) > 1e-8 {
		t.Fatalf("pan offset %.8f, before %+v", s.vp.Offset, before)
	}
	if want, _ := s.vp.IndexAt(float64(x+64-r.X), len(s.bars)); s.index != want {
		t.Fatalf("crosshair stale after pan: %d, want %d", s.index, want)
	}
	tt.Move(x, y)
	if s.vp != before {
		t.Fatalf("reverse pan %+v, want %+v", s.vp, before)
	}
	// A captured release outside the plot must end the drag.
	tt.Move(r.X+r.W/4, 5)
	tt.Release(r.X+r.W/4, 5)
	if s.pressOn || s.ptr.On || s.index != -1 {
		t.Fatalf("outside release left drag/hover active: press=%v ptr=%+v index=%d", s.pressOn, s.ptr, s.index)
	}
	stopped := s.vp
	tt.Move(x+100, y)
	if s.vp != stopped {
		t.Fatal("viewport continued panning after release")
	}
	if !tt.HasText(s.status()) {
		t.Fatalf("viewport status stale: %q", tt.Texts())
	}
	// Captured drags clamp at both ends, even far outside the element.
	tt.Press(x, y)
	tt.Move(x+100000, y)
	if s.vp.Offset != -kline.OverscrollBars {
		t.Fatalf("left pan limit: %+v", s.vp)
	}
	tt.Move(x-100000, y)
	wantRight := float64(len(s.bars)) - float64(r.W)/s.vp.BarWidth + kline.OverscrollBars
	if math.Abs(s.vp.Offset-wantRight) > 1e-8 {
		t.Fatalf("right pan limit %.8f, want %.8f", s.vp.Offset, wantRight)
	}
	tt.Release(x-100000, y)
}

func TestPaintViewGesturesOnEmptyPlotMargin(t *testing.T) {
	s, tt := newPaintTester(t)
	r := s.panes.Price
	// FitRight reserves two bar widths after the last candle. It is still
	// interactive plot space even though IndexAt cannot return a candle.
	x, y := r.X+r.W-2, r.Y+r.H/3
	tt.Move(x, y)
	if s.ptr.On || s.index != -1 {
		t.Fatal("empty margin reported a bar")
	}
	before := s.vp
	tt.Press(x, y)
	tt.Move(x+24, y)
	tt.Release(x+24, y)
	if math.Abs(s.vp.Offset-(before.Offset-24/before.BarWidth)) > 1e-8 {
		t.Fatal("drag starting in empty plot margin did not pan")
	}
	before = s.vp
	tt.Scroll(x, y, 0, -80)
	if s.vp.BarWidth <= before.BarWidth {
		t.Fatal("wheel in empty plot margin did not zoom")
	}
}

func TestPaintViewZoomBothDirectionsAndBounds(t *testing.T) {
	s, tt := newPaintTester(t)
	r := s.panes.Price
	x, y := r.X+r.W/2, r.Y+r.H/3
	before := s.vp
	anchor := before.Offset + float64(x-r.X)/before.BarWidth
	tt.Scroll(x, y, 0, -80)
	if s.vp.BarWidth <= before.BarWidth {
		t.Fatal("wheel up did not zoom in")
	}
	if got := s.vp.Offset + float64(x-r.X)/s.vp.BarWidth; math.Abs(got-anchor) > 1e-8 {
		t.Fatalf("zoom moved anchor %.8f -> %.8f", anchor, got)
	}
	if want, _ := s.vp.IndexAt(float64(x-r.X), len(s.bars)); !s.ptr.On || s.index != want {
		t.Fatal("zoom did not update hover")
	}
	tt.Scroll(x, y, 0, 80)
	if math.Abs(s.vp.BarWidth-before.BarWidth) > 1e-8 || math.Abs(s.vp.Offset-before.Offset) > 1e-8 {
		t.Fatalf("inverse wheel failed: %+v -> %+v", before, s.vp)
	}
	for _, tc := range []struct {
		dy, width float64
	}{{-100000, kline.MaxBarWidth}, {100000, kline.MinBarWidth}} {
		tt.Scroll(x, y, 0, float32(tc.dy))
		if s.vp.BarWidth != tc.width {
			t.Fatalf("width %v, want limit %v", s.vp.BarWidth, tc.width)
		}
		clamped := s.vp
		clamped.Clamp(len(s.bars), float64(r.W))
		if s.vp != clamped {
			t.Fatalf("zoom escaped viewport limits: %+v", s.vp)
		}
	}
	before = s.vp
	tt.Scroll(x, y, 50, 0)
	tt.Scroll(s.panes.PriceAxis.X+10, y, 0, -80)
	if s.vp != before {
		t.Fatal("horizontal wheel or axis wheel changed the viewport")
	}
}

func TestPaintViewClearsAxesGapAndOutsideHover(t *testing.T) {
	s, tt := newPaintTester(t)
	p := s.panes
	for _, tc := range []struct {
		name string
		x, y float32
	}{
		{"price axis boundary", p.PriceAxis.X, p.Price.Y + 50},
		{"volume axis", p.VolumeAxis.X + 10, p.Volume.Y + 10},
		{"time axis", p.Time.X + p.Time.W/2, p.Time.Y + p.Time.H/2},
		{"pane gap", p.Price.X + 100, (p.Price.Y + p.Price.H + p.Volume.Y) / 2},
		{"chrome", p.Price.X + 100, 5},
		{"left outside", p.Price.X - 1, p.Price.Y + 30},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tt.Move(p.Price.X+p.Price.W/2, p.Price.Y+p.Price.H/3)
			if !s.ptr.On {
				t.Fatal("initial plot hover missing")
			}
			tt.Move(tc.x, tc.y)
			tt.Frame()
			if s.ptr.On || s.index != -1 || s.hit.ID != "" || strings.Contains(s.readout(), "Hover #") {
				t.Fatalf("stale hover: pointer=%+v index=%d hit=%+v", s.ptr, s.index, s.hit)
			}
			if !tt.HasText(s.readout()) {
				t.Fatal("host readout did not clear")
			}
		})
	}
	tt.Move(p.Volume.X+p.Volume.W/2, p.Volume.Y+p.Volume.H/2)
	if !s.ptr.On || s.index < 0 || s.ptr.Y != p.Volume.Y+p.Volume.H/2 || s.hit.ID != "" {
		t.Fatalf("volume hover %+v index=%d hit=%+v", s.ptr, s.index, s.hit)
	}
	if strings.Contains(s.readout(), "raw pointer") {
		t.Fatal("volume pointer was described as a price")
	}
}

func TestPaintViewLayerDrawAndHitInBothModes(t *testing.T) {
	s, tt := newPaintTester(t)
	items := s.layer.Items()
	if len(items) != 1 || items[0].Kind != kline.OverlayHLine {
		t.Fatalf("price drawing missing: %+v", items)
	}
	o := items[0]
	for _, compare := range []bool{false, true} {
		if compare {
			if err := tt.Click("PaintCompare"); err != nil {
				t.Fatal(err)
			}
		}
		r := s.panes.Price
		w := float64(r.W)
		scale := kline.MainScale(s.bars, nil, s.vp, w)
		if compare {
			_, scale, _ = kline.CompareScales(s.bars, s.compare, nil, s.vp, w)
		}
		x := r.X + r.W/2
		y := r.Y + float32(kline.PriceToLocalY(float64(r.H), o.Points[0].Price, scale))
		if !contains(r, x, y) {
			t.Fatalf("drawing outside plot in compare=%v: y=%v rect=%+v", compare, y, r)
		}
		band := ui.Rect{X: r.X + 30, Y: y - 2, W: r.W - 60, H: 4}
		if countColor(tt.Image(), band, kline.DarkDefault().Overlay) < int(band.W)/2 {
			t.Fatalf("raw price line missing from painted scale in compare=%v", compare)
		}
		tt.Move(x, y)
		if s.hit.ID != o.ID || s.hit.Part != kline.HitBody {
			t.Fatalf("painted line missed in compare=%v: %+v", compare, s.hit)
		}
		if !tt.HasText(s.status()) || !strings.Contains(s.status(), "Layer hit: "+o.ID) {
			t.Fatalf("Layer hit not reported: %q", tt.Texts())
		}
		tt.Press(x, y)
		tt.Release(x, y)
		if selected := s.layer.Selected(); selected == nil || selected.ID != o.ID {
			t.Fatal("click did not select hit Layer item")
		}
		if countColor(tt.Image(), band, kline.DarkDefault().OverlayHot) < int(band.W)/2 {
			t.Fatal("selected Layer line was not rendered")
		}
		tt.Move(x, y+3*kline.HitRadius)
		if s.hit.ID != "" {
			t.Fatal("Layer hit survived outside hit radius")
		}
		// Deselect via the real input path before checking the next mode's pixels.
		tt.Press(x, y+3*kline.HitRadius)
		tt.Release(x, y+3*kline.HitRadius)
	}
}

func TestPaintViewCompareToggleRendersAndRestores(t *testing.T) {
	s, tt := newPaintTester(t)
	style := kline.DarkDefault()
	if len(s.bars) != len(s.compare) || len(s.bars) != 600 {
		t.Fatal("synthetic series lengths differ")
	}
	for i := range s.bars {
		if !s.bars[i].Time.Equal(s.compare[i].Time) {
			t.Fatalf("comparison bar %d is not aligned", i)
		}
	}
	r := s.panes.Price
	if countColor(tt.Image(), r, style.Up)+countColor(tt.Image(), r, style.Down) < 100 {
		t.Fatal("Paint candles missing")
	}
	if countColor(tt.Image(), r, style.Compare) != 0 {
		t.Fatal("compare line visible before toggle")
	}
	before := s.vp
	if err := tt.Click("PaintCompare"); err != nil {
		t.Fatal(err)
	}
	if !s.comparing || !s.indexed || s.vp != before {
		t.Fatalf("toggle failed or changed viewport: comparing=%v indexed=%v viewport=%+v", s.comparing, s.indexed, s.vp)
	}
	if countColor(tt.Image(), r, style.Compare) < 100 {
		t.Fatal("PaintCompare did not render the second series")
	}
	if !tt.HasText(s.status()) || !strings.Contains(s.status(), "indexed to 100") {
		t.Fatalf("compare renderer status missing: %q", tt.Texts())
	}
	if err := tt.Click("PaintCompare"); err != nil {
		t.Fatal(err)
	}
	if s.comparing || s.indexed || s.vp != before || countColor(tt.Image(), r, style.Compare) != 0 {
		t.Fatal("toggle did not restore Paint and its viewport")
	}
}

func TestPaintViewTimeAxisAndResize(t *testing.T) {
	s, tt := newPaintTester(t)
	for _, size := range [][2]int{{1000, 760}, {640, 500}, {1200, 800}} {
		tt.SetSize(size[0], size[1])
		tt.Frame()
		if s.panes.Time.H < 1 || countColor(tt.Image(), s.panes.Time, kline.DarkDefault().Muted) == 0 {
			t.Fatalf("time axis not painted at %v: %+v", size, s.panes.Time)
		}
		x, y := s.panes.Price.X+s.panes.Price.W/2, s.panes.Price.Y+s.panes.Price.H/3
		tt.Move(x, y)
		if !s.ptr.On || s.ptr.X != x || s.ptr.Y != y {
			t.Fatalf("resize lost absolute mapping: pointer=%+v want=(%v,%v)", s.ptr, x, y)
		}
		clamped := s.vp
		clamped.Clamp(len(s.bars), float64(s.panes.Price.W))
		if s.vp != clamped || !tt.HasText(s.status()) {
			t.Fatalf("resize left stale viewport/status: %+v %q", s.vp, tt.Texts())
		}
	}
}

// Count near the stroke color to allow edge antialiasing without accepting
// unrelated candles or a merely nonempty image as proof of rendering.
func countColor(img *image.RGBA, r ui.Rect, color ui.Color) int {
	if img == nil {
		return 0
	}
	n := 0
	for y := max(0, int(r.Y)); y < min(img.Bounds().Max.Y, int(r.Y+r.H)); y++ {
		for x := max(0, int(r.X)); x < min(img.Bounds().Max.X, int(r.X+r.W)); x++ {
			p := img.RGBAAt(x, y)
			if abs(int(p.R)-int(color.R)) <= 15 && abs(int(p.G)-int(color.G)) <= 15 && abs(int(p.B)-int(color.B)) <= 15 && p.A > 0 {
				n++
			}
		}
	}
	return n
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
