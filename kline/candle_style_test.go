package kline

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestHollowUpOutlinesRisingBody(t *testing.T) {
	cs := []Candle{
		{Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Open: 10, Close: 20, High: 21, Low: 9, Volume: 5},
		{Time: time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC), Open: 20, Close: 10, High: 21, Low: 9, Volume: 5},
	}
	style := DarkDefault()
	style.Plot = ui.RGB(8, 8, 12)
	style.VolumeBg = ui.RGB(8, 8, 12)
	style.Grid = ui.RGB(40, 40, 48)
	style.Up = ui.RGB(0, 220, 120)
	style.Down = ui.RGB(230, 40, 40)

	solidUp, solidDown := bodyInterior(t, cs, style)
	if !nearUI(solidUp, style.Up) {
		t.Fatalf("solid rising body interior %+v, want %+v", solidUp, style.Up)
	}
	if !nearUI(solidDown, style.Down) {
		t.Fatalf("solid falling body interior %+v, want %+v", solidDown, style.Down)
	}

	style.HollowUp = true
	hollowUp, hollowDown := bodyInterior(t, cs, style)
	if !nearUI(hollowUp, style.Plot) {
		t.Fatalf("hollow rising body interior %+v, want plot %+v", hollowUp, style.Plot)
	}
	if nearUI(hollowUp, style.Up) {
		t.Fatalf("hollow rising body is filled %+v", hollowUp)
	}
	if !nearUI(hollowDown, style.Down) {
		t.Fatalf("hollow falling body interior %+v, want %+v", hollowDown, style.Down)
	}
}

func TestPaintCopiesMACDColorsFromStyle(t *testing.T) {
	cs := Generate(80, 42)
	macd := NewMACD(12, 26, 9)
	macd.Calc(cs)
	style := LightDefault()
	style.MACDDIF = ui.RGB(1, 2, 3)
	style.MACDDEA = ui.RGB(4, 5, 6)
	style.MACDZero = ui.RGB(7, 8, 9)
	style.Up = ui.RGB(10, 11, 12)
	style.Down = ui.RGB(13, 14, 15)
	paintSeries(t, cs, []Indicator{macd}, style)
	if macd.DIFColor != style.MACDDIF || macd.DEAColor != style.MACDDEA || macd.Zero != style.MACDZero {
		t.Fatalf("macd lines %+v %+v %+v", macd.DIFColor, macd.DEAColor, macd.Zero)
	}
	if macd.Up != style.Up || macd.Down != style.Down {
		t.Fatalf("macd histogram %+v %+v", macd.Up, macd.Down)
	}
}

// bodyInterior samples inside the first rising body and the second
// falling body, off the wick so an outline does not count as a fill.
func bodyInterior(t *testing.T, cs []Candle, style Style) (up, down color.RGBA) {
	t.Helper()
	const (
		w, h   = 480, 320
		barW   = 48.0
		bodyFr = 0.68
	)
	var panes Panes
	img := paintAt(t, w, h, func(p *ui.Painter, r ui.Rect) {
		panes = SplitNoTime(r)
		vp := Viewport{BarWidth: barW}
		Paint(p, panes, cs, nil, vp, Pointer{}, -1, style)
	})
	up = sampleBody(t, img, panes.Price, 0, barW, bodyFr)
	down = sampleBody(t, img, panes.Price, 1, barW, bodyFr)
	return up, down
}

func paintSeries(t *testing.T, cs []Candle, inds []Indicator, style Style) {
	t.Helper()
	paintAt(t, 480, 320, func(p *ui.Painter, r ui.Rect) {
		panes := SplitStack(r, false, true)
		vp := Viewport{BarWidth: 12}
		vp.FitRight(len(cs), float64(panes.Price.W))
		Paint(p, panes, cs, inds, vp, Pointer{}, -1, style)
	})
}

func paintAt(t *testing.T, w, h int, draw func(*ui.Painter, ui.Rect)) *image.RGBA {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(draw)
	}, w, h)
	img := tt.Image()
	if img == nil {
		t.Fatal("nil frame")
	}
	return img
}

func sampleBody(t *testing.T, img *image.RGBA, plot ui.Rect, bar int, barW, bodyFrac float64) color.RGBA {
	t.Helper()
	bodyW := barW * bodyFrac
	// Quarter of a body left of the wick, still inside the body.
	x := plot.X + float32(float64(bar)*barW+barW/2-bodyW/4)
	y := plot.Y + plot.H/2
	return pixelAt(t, img, x, y)
}

func pixelAt(t *testing.T, img *image.RGBA, x, y float32) color.RGBA {
	t.Helper()
	b := img.Bounds()
	px := int(x + 0.5)
	py := int(y + 0.5)
	if px < b.Min.X || px >= b.Max.X || py < b.Min.Y || py >= b.Max.Y {
		t.Fatalf("sample %v,%v outside %v", x, y, b)
	}
	return img.RGBAAt(px, py)
}

func nearUI(got color.RGBA, want ui.Color) bool {
	dr := int(got.R) - int(want.R)
	dg := int(got.G) - int(want.G)
	db := int(got.B) - int(want.B)
	return dr*dr+dg*dg+db*db <= 24*24
}
