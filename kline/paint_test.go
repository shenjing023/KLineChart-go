package kline

import (
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
)

func TestTimeTickUsesCandleIndexAcrossSessionGap(t *testing.T) {
	start := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC) // Friday.
	var cs []Candle
	for _, day := range []int{0, 3, 4, 5, 6, 7} {
		cs = append(cs, Candle{Time: start.AddDate(0, 0, day), Open: 10, High: 11, Low: 9, Close: 10})
	}
	const plotW = 600
	vp := Viewport{BarWidth: 100}
	monday := cs[1].Time
	position, ok := timeBarPosition(cs, monday)
	if !ok {
		t.Fatal("Monday is not on the candle axis")
	}
	x := (position - vp.Offset) * vp.BarWidth
	got, ok := vp.IndexAt(x, len(cs))
	if !ok || got != 1 {
		t.Fatalf("Monday tick x %.2f maps to bar %d (ok=%v), want [100,200)", x, got, ok)
	}
	if x < vp.BarLeft(1) || x >= vp.BarLeft(1)+vp.BarWidth {
		t.Fatalf("Monday tick x %.2f is outside Monday's bar [%.2f, %.2f)", x, vp.BarLeft(1), vp.BarLeft(1)+vp.BarWidth)
	}
}

// The linear position function is exactly the inverse of TimeAt. Paint
// still has to use it, so render a real grid and verify the Monday tick
// pixel falls over Monday's candle rather than a later session.
func TestTimeTickPaintUsesCandleIndex(t *testing.T) {
	start := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC) // Friday.
	var cs []Candle
	for _, day := range []int{0, 3, 4, 5, 6, 7} {
		cs = append(cs, Candle{
			Time: start.AddDate(0, 0, day),
			Open: 10, High: 11, Low: 9, Close: 10, Volume: 100,
		})
	}
	style := DarkDefault()
	style.Grid = ui.RGB(255, 0, 255)
	style.Cross = ui.Color{}
	panes := Split(ui.Rect{W: 672, H: 300})
	vp := Viewport{BarWidth: 100}
	left, right := TimeAt(cs, vp.Offset), TimeAt(cs, vp.Offset+float64(panes.Price.W)/vp.BarWidth)
	var mondayX float32
	for _, tick := range chart.NewTime(left, right).Ticks(6) {
		when := time.UnixMilli(int64(tick.Value))
		if when.Equal(cs[1].Time) {
			position, ok := timeBarPosition(cs, when)
			if !ok {
				t.Fatal("Monday is missing from the candle axis")
			}
			mondayX = panes.Price.X + float32((position-vp.Offset)*vp.BarWidth)
		}
	}
	if mondayX == 0 {
		t.Fatal("fixture did not generate a Monday tick")
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, _ ui.Rect) {
			Paint(p, panes, cs, nil, vp, Pointer{}, -1, style)
		})
	}, 672, 300)
	tt.Frame()
	img := tt.Image()
	gotX, ok := gridPixelX(img, panes.Price, style.Grid, mondayX)
	if !ok {
		t.Fatal("Monday grid line was not painted")
	}
	bar, under := vp.IndexAt(float64(gotX)-float64(panes.Price.X), len(cs))
	if !under || bar != 1 {
		t.Fatalf("Monday grid x=%d maps to bar %d (ok=%v), want Monday", gotX, bar, under)
	}
}

func gridPixelX(img *image.RGBA, rect ui.Rect, color ui.Color, near float32) (int, bool) {
	if img == nil {
		return 0, false
	}
	y := int(rect.Y + rect.H/2)
	best := -1
	bestDistance := float32(img.Bounds().Dx())
	for x := max(int(rect.X), img.Bounds().Min.X); x < min(int(rect.X+rect.W), img.Bounds().Max.X); x++ {
		if !nearColor(img.RGBAAt(x, y), []ui.Color{color}) {
			continue
		}
		d := float32(x) - near
		if d < 0 {
			d = -d
		}
		if d < bestDistance {
			best, bestDistance = x, d
		}
	}
	return best, best >= 0
}

func TestTimeTickSpansSessionGapsWithoutGlobalDistortion(t *testing.T) {
	start := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	cs := []Candle{
		{Time: start},
		{Time: start.AddDate(0, 0, 3)},
		{Time: start.AddDate(0, 0, 4)},
	}
	vp := Viewport{BarWidth: 100}
	// Midday Saturday contributes no trading bar. Its position belongs
	// inside the Friday-to-Monday segment, rather than shifting every
	// later candle as a global elapsed-time scale would.
	when := start.AddDate(0, 0, 1)
	position, ok := timeBarPosition(cs, when)
	if !ok {
		t.Fatal("Saturday scale position is missing")
	}
	if position <= 0 || position >= 1 {
		t.Fatalf("Saturday position %.3f, want inside (0, 1)", position)
	}
	x := (position - vp.Offset) * vp.BarWidth
	if x < vp.BarLeft(0) || x >= vp.BarLeft(1) {
		t.Fatalf("Saturday x %.2f is outside the Friday-Monday segment", x)
	}
	// Monday and Tuesday still land exactly on their candle centers'
	// bar starts, so the long weekend cannot push them right.
	if pos, _ := timeBarPosition(cs, cs[1].Time); pos != 1 {
		t.Fatalf("Monday position %.3f, want 1", pos)
	}
	if pos, _ := timeBarPosition(cs, cs[2].Time); pos != 2 {
		t.Fatalf("Tuesday position %.3f, want 2", pos)
	}
}

func TestTimeTickAcceptsOffGridSessionBars(t *testing.T) {
	cs := []Candle{
		{Time: time.Date(2024, 1, 2, 9, 30, 0, 0, time.UTC)},
		{Time: time.Date(2024, 1, 2, 10, 15, 0, 0, time.UTC)},
		{Time: time.Date(2024, 1, 2, 13, 0, 0, 0, time.UTC)},
	}
	for _, when := range []time.Time{
		time.Date(2024, 1, 2, 9, 45, 0, 0, time.UTC),
		time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
		time.Date(2024, 1, 2, 13, 0, 0, 0, time.UTC),
	} {
		pos, ok := timeBarPosition(cs, when)
		if !ok {
			t.Fatalf("timeBarPosition(%s) rejected an off-grid session time", when)
		}
		if pos < -OverscrollBars || pos > float64(len(cs))+OverscrollBars {
			t.Fatalf("timeBarPosition(%s) = %.3f, outside the axis", when, pos)
		}
	}
}

func TestApplyIndicatorStyleColorsOpenPanes(t *testing.T) {
	macd := NewMACD(12, 26, 9)
	kdj := NewKDJ(9, 3, 3)
	macd.DIFColor, macd.DEAColor, macd.Up, macd.Down, macd.Zero = ui.Color{}, ui.Color{}, ui.Color{}, ui.Color{}, ui.Color{}
	kdj.KColor, kdj.DColor, kdj.JColor = ui.Color{}, ui.Color{}, ui.Color{}
	style := DarkDefault()
	applyIndicatorStyle([]Indicator{macd, kdj}, Panes{}, style)
	if macd.DIFColor != (ui.Color{}) || kdj.KColor != (ui.Color{}) {
		t.Fatal("a closed sub-pane should not take theme colors")
	}
	open := Panes{MACD: ui.Rect{W: 10, H: 40}, KDJ: ui.Rect{W: 10, H: 40}}
	applyIndicatorStyle([]Indicator{macd, kdj}, open, style)
	if macd.DIFColor != style.MACDDIF || macd.DEAColor != style.MACDDEA || macd.Up != style.Up || macd.Zero != style.MACDZero {
		t.Fatalf("MACD colors %+v", macd)
	}
	if kdj.KColor != style.KDJK || kdj.DColor != style.KDJD || kdj.JColor != style.KDJJ {
		t.Fatalf("KDJ colors %+v", kdj)
	}
}

func TestSplitGivesPriceAndVolume(t *testing.T) {
	panes := Split(ui.Rect{X: 10, Y: 20, W: 800, H: 500})
	if panes.Price.W <= 0 || panes.Price.H <= 0 {
		t.Fatalf("price pane %+v", panes.Price)
	}
	if panes.Volume.Y <= panes.Price.Y+panes.Price.H {
		t.Fatalf("volume should sit below price: %+v %+v", panes.Price, panes.Volume)
	}
	if panes.PriceAxis.X != panes.Price.X+panes.Price.W {
		t.Fatalf("price axis %+v", panes.PriceAxis)
	}
	if panes.Volume.W != panes.Price.W {
		t.Fatal("volume and price plots should share a width")
	}
	if panes.Time.Y < panes.Volume.Y+panes.Volume.H-0.5 {
		t.Fatalf("time axis %+v volume %+v", panes.Time, panes.Volume)
	}
	if panes.Time.H <= 0 {
		t.Fatal("Split should keep a time axis")
	}
	noTime := SplitNoTime(ui.Rect{X: 10, Y: 20, W: 800, H: 500})
	if noTime.Time.H != 0 {
		t.Fatalf("SplitNoTime time axis %+v", noTime.Time)
	}
	if noTime.Volume.Y <= noTime.Price.Y+noTime.Price.H || noTime.Price.W != panes.Price.W {
		t.Fatalf("SplitNoTime panes %+v", noTime)
	}
}
