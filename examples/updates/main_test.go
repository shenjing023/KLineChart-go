package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/shenjing023/KLineChart-go/kline"
)

func TestUpdatesViewAppendReviseAlignAndReset(t *testing.T) {
	d, err := newUpdatesDemo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.chart.Close() })
	tt := ui.NewTester(updatesView(d), 960, 700)
	tt.Frame()
	if !tt.HasText("APPEND / UPDATE") || !tt.HasText("bars 32") {
		t.Fatalf("initial view: %q", tt.Texts())
	}
	if _, ok := tt.Find("Reset"); !ok {
		t.Fatal("control row did not expose Reset")
	}
	tt.Scroll(760, 20, -80, 0)
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	click("Append A")
	if d.err != nil || d.chart.Bars() != 32 || !strings.Contains(d.note, "held") {
		t.Fatalf("staged append: err=%v bars=%d note=%q", d.err, d.chart.Bars(), d.note)
	}
	if enabled := d.reviseEnabled("600519"); enabled {
		t.Fatal("Revise A must be disabled while A has a newer held bar")
	}
	click("Append B")
	if d.err != nil || d.chart.Bars() != 33 || errors.Is(d.chart.AlignError(), kline.ErrMisaligned) {
		t.Fatalf("paired append: err=%v bars=%d align=%v", d.err, d.chart.Bars(), d.chart.AlignError())
	}
	a, _ := d.chart.Bar("600519", d.chart.Bars()-1)
	b, _ := d.chart.Bar("000001", d.chart.Bars()-1)
	if !a.Time.Equal(b.Time) {
		t.Fatalf("paired tails differ: %s / %s", a.Time, b.Time)
	}
	click("Append A")
	if d.chart.Bars() != 33 || !strings.Contains(d.note, "held") {
		t.Fatalf("next staged append: bars=%d note=%q", d.chart.Bars(), d.note)
	}
	heldBefore := d.pending["600519"].Close
	click("Revise held A")
	held, _ := d.chart.Bar("600519", d.chart.Bars()-1)
	if !a.Time.Equal(b.Time) || d.chart.Bars() != 33 || held.Close != a.Close {
		t.Fatalf("visible-edge revise after staging changed the pair: %+v", held)
	}
	if !strings.Contains(d.note, "held") || d.pending["600519"].Close == heldBefore {
		t.Fatalf("held revision was not recorded: %q %+v", d.note, d.pending["600519"])
	}
	click("Append B")
	paired, _ := d.chart.Bar("600519", d.chart.Bars()-1)
	if paired.Close != d.pending["600519"].Close {
		t.Fatalf("paired reveal ignored revised held close: revealed %.2f, submitted %.2f",
			paired.Close, d.pending["600519"].Close)
	}
	click("Revise A")
	after, _ := d.chart.Bar("600519", d.chart.Bars()-1)
	if d.err != nil || d.chart.Bars() != 34 || after.Time != paired.Time || after.Close == paired.Close {
		t.Fatalf("revision: err=%v bars=%d %+v -> %+v", d.err, d.chart.Bars(), paired, after)
	}
	click("Skew feeds")
	if d.err != nil || !errors.Is(d.chart.AlignError(), kline.ErrMisaligned) {
		t.Fatalf("skew: err=%v align=%v", d.err, d.chart.AlignError())
	}
	click("Revise held A")
	if d.err != nil || !strings.Contains(d.note, "UpdateLast(600519, held)") {
		t.Fatalf("held edge revision after skew: %v %q", d.err, d.note)
	}
	click("Reset")
	if d.err != nil || d.chart.LoadError() != nil || d.chart.AlignError() != nil ||
		d.chart.Bars() != 32 || !strings.Contains(d.note, "reload revised fixture") {
		t.Fatalf("reset: err=%v load=%v align=%v bars=%d", d.err, d.chart.LoadError(), d.chart.AlignError(), d.chart.Bars())
	}
}

func TestUpdatesSkewTracksHeldBarsDespitePriorAlignmentError(t *testing.T) {
	d, err := newUpdatesDemo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.chart.Close() })
	tt := ui.NewTester(updatesView(d), 960, 700)
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}

	click("Skew feeds")
	if !errors.Is(d.chart.AlignError(), kline.ErrMisaligned) {
		t.Fatalf("skew did not report misalignment: %v", d.chart.AlignError())
	}
	if first, ok := d.pending["600519"]; !ok || d.state["600519"] != "held" {
		t.Fatalf("skew did not retain A's submitted bar: %+v %q", first, d.state["600519"])
	}

	// B is one hour behind A. A's +2h edge pins the shared index, and the
	// ordinary next-open helper continues from the latest held B bar.
	click("Append B")
	click("Append B")
	if d.err != nil {
		t.Fatalf("a prior AlignError disabled successful held appends: %v", d.err)
	}
	if state := d.state["000001"]; state != "held" {
		t.Fatalf("B's successfully submitted edge became %q", state)
	}
	if !d.revisePendingEnabled("000001") {
		t.Fatal("B's held bar was mistaken for a dropped one")
	}

	// Revising the shown A edge while B has a held tail must be a no-op.
	before, _ := d.chart.Bar("600519", d.chart.Bars()-1)
	toolbar, ok := tt.Find("Revise A")
	if !ok || toolbar.W <= 0 {
		t.Fatal("Revise A disappeared")
	}
	click("Revise A")
	after, _ := d.chart.Bar("600519", d.chart.Bars()-1)
	if after != before || d.err != nil {
		t.Fatalf("disabled shown revision changed the chart: %v -> %v", before, after)
	}
	if !strings.Contains(tt.Texts()[0], "APPEND / UPDATE") {
		t.Fatal("view did not survive guarded revision")
	}
}

func TestUpdatesNarrowScrollingAndReviseGuard(t *testing.T) {
	d, err := newUpdatesDemo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.chart.Close() })
	tt := ui.NewTester(updatesView(d), 640, 460)
	tt.Frame()
	tt.Scroll(500, 20, -420, 0)
	tt.Frame()
	if d.toolbar.MaxX <= 0 {
		t.Fatalf("narrow toolbar did not become scrollable: %+v", d.toolbar)
	}
	reset, ok := tt.Find("Reset")
	if !ok {
		t.Fatal("scrolled control row lost Reset")
	}
	d.toolbar.X = d.toolbar.MaxX
	tt.Frame()
	reset, ok = tt.Find("Reset")
	if !ok || reset.W <= 0 || reset.H <= 0 {
		t.Fatalf("Reset has no tappable rect at the scrolled end: %+v %v", reset, ok)
	}
	tt.ClickAt(reset.X+reset.W/2, reset.Y+reset.H/2)
	tt.Frame()
	if d.err != nil || !strings.Contains(d.note, "reload revised fixture") {
		t.Fatalf("narrow reset at scroll end: %v %q", d.err, d.note)
	}
	if _, ok := tt.Find("Reset"); !ok {
		t.Fatal("horizontal toolbar disappeared after reset")
	}
}
