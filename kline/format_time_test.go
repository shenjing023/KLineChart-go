package kline

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// req02Chart builds a chart whose host formatter masks the month and day
// to a fixed width, the shape a blind training session needs.
func req02Chart(format func(time.Time) string) *Chart {
	return NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Indicators: []string{"MA"},
		Location:   time.FixedZone("Asia/Shanghai", 8*60*60),
		FormatTime: format,
	})
}

// TestFormatTimeMasksReadout is the failure that matters for a blind
// session: the header prints the bar's real date through Readout.
func TestFormatTimeMasksReadout(t *testing.T) {
	real := req02Chart(nil).Readout("600519")
	if !strings.Contains(real, "2024-") {
		t.Fatalf("unmasked readout should show a real year, got %q", real)
	}
	masked := req02Chart(func(time.Time) string { return "xxxx-xx-02" }).Readout("600519")
	if strings.Contains(masked, "2024") || strings.Contains(masked, "2023") {
		t.Fatalf("masked readout leaks a year: %q", masked)
	}
	if !strings.Contains(masked, "xxxx-xx-02") {
		t.Fatalf("masked readout should carry the host text, got %q", masked)
	}
}

// TestFormatTimeMasksAxisAndCrosshair checks the two painted labels. The
// tester reports every string the frame drew, so both the axis tick and
// the crosshair tag are in Texts.
func TestFormatTimeMasksAxisAndCrosshair(t *testing.T) {
	c := req02Chart(func(time.Time) string { return "MASKED" })
	tt := ui.NewTester(c.View, 960, 720)
	box := mustBox(t, tt, "600519 chart")
	tt.Move(box.X+box.W/2, box.Y+24)
	texts := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(texts, "MASKED") {
		t.Fatalf("axis or crosshair should print the host text, texts %q", tt.Texts())
	}
	// A real date would appear as 2024-01-02 from Generate's first bar.
	if strings.Contains(texts, "2024-01-02") {
		t.Fatalf("masked frame leaks a real date: %q", tt.Texts())
	}
}

// TestFormatTimeKeepsTickPositions proves the mask is text only. The
// grid lines and therefore the bar positions must not move when a host
// formatter is installed.
func TestFormatTimeKeepsTickPositions(t *testing.T) {
	plain := req02Chart(nil)
	masked := req02Chart(func(time.Time) string { return "MASKED" })
	plainTT := ui.NewTester(plain.View, 960, 720)
	maskedTT := ui.NewTester(masked.View, 960, 720)
	plainImg := plainTT.Image()
	maskedImg := maskedTT.Image()
	if plainImg.Bounds() != maskedImg.Bounds() {
		t.Fatalf("frame size changed: %v vs %v", plainImg.Bounds(), maskedImg.Bounds())
	}
	// Both charts load the same synthetic series on the same period, so
	// the price plot must be identical. Only the label strings differ,
	// and those sit in the axis and time strip, outside the candles.
	boxPlain := mustBox(t, plainTT, "600519 chart")
	boxMasked := mustBox(t, maskedTT, "600519 chart")
	if boxPlain != boxMasked {
		t.Fatalf("chart box moved: %v vs %v", boxPlain, boxMasked)
	}
	if plain.vp != masked.vp {
		t.Fatalf("viewport changed under a formatter: %+v vs %+v", plain.vp, masked.vp)
	}
	if plain.Bars() != masked.Bars() {
		t.Fatalf("bar count changed: %d vs %d", plain.Bars(), masked.Bars())
	}
	pf, pl := plain.Visible()
	mf, ml := masked.Visible()
	if pf != mf || pl != ml {
		t.Fatalf("visible range changed: %d..%d vs %d..%d", pf, pl, mf, ml)
	}
}

// TestFormatTimeReceivesDisplayZone pins the contract that the host gets
// the instant in ChartOptions.Location, so a host never converts twice.
func TestFormatTimeReceivesDisplayZone(t *testing.T) {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	var seen []string
	c := NewChart(ChartOptions{
		Symbols:    []string{"600519"},
		Period:     Period{Span: 1, Unit: UnitHour},
		Indicators: []string{"MA"},
		Location:   zone,
		FormatTime: func(t time.Time) string {
			seen = append(seen, t.Format("2006-01-02 15:04 MST"))
			return "MASKED"
		},
	})
	tt := ui.NewTester(c.View, 960, 720)
	box := mustBox(t, tt, "600519 chart")
	tt.Move(box.X+box.W/2, box.Y+24)
	if len(seen) == 0 {
		t.Fatal("host formatter was never called")
	}
	for _, got := range seen {
		if !strings.Contains(got, "+08") && !strings.Contains(got, "Asia/Shanghai") && !strings.Contains(got, "HKT") && !strings.Contains(got, "CST") {
			t.Fatalf("formatter got a time outside the chart zone: %q", got)
		}
	}
}

// TestFormatTimeNilKeepsDefaults records that the option is opt-in.
func TestFormatTimeNilKeepsDefaults(t *testing.T) {
	c := req02Chart(nil)
	if c.timeFormat != nil {
		t.Fatal("a nil option must not install a formatter")
	}
	out := c.Readout("600519")
	if !strings.Contains(out, "2024-") {
		t.Fatalf("default readout should keep the real year, got %q", out)
	}
}
