# K-line

Defaults follow mainland China A-share conventions: red rising candles, green falling candles, `600519` / `000001` sample symbols, and Asia/Shanghai trading-session labels. `SyntheticSource` uses weekday 09:30–11:30 and 13:00–15:00 sessions; it does not invent exchange holidays, and it refuses a period that has no session bars (3h, 150m) with `ErrInvalidPeriod`.

A small candlestick chart drawn with [MyGo](https://mygo.egoist.dev) Native UI. The window is `ui.View`: Go paints it, and the process does not open a WebView or WebView2.

It is a demo of the interaction in [KLineChart](https://github.com/klinecharts/KLineChart) (candles, volume, pan, zoom, crosshair), not a port of that library. MyGo's [`ui/chart`](https://mygo.egoist.dev/docs/ui/chart) package has scales, axes, and bar/line/area marks. It has no OHLC geometry and no pan or zoom, so this repo draws the candles itself and keeps the viewport in Go. Price and time ticks come from `chart.NewLinear` and `chart.NewTime`.

## What you can do

- **Stack / Overlay.** Stack is the default: 600519 and 000001 each get a price plot, a volume strip, and a MACD sub-pane, on their own price scales. Turning **KDJ** on adds a KDJ sub-pane under MACD. Overlay puts both symbols on one price plot.
- In Overlay, 600519 stays candles and 000001 is a close line. Both are divided by their own close at the first visible bar where both closes are positive, so that bar is 100 on the shared Y axis. Panning or zooming moves that base with the window. The header shows each symbol's own OHLCV at the crosshair index, plus that index (`idx`). Overlay compares the first two symbols: a third one is not drawn and has no header until Stack. When no bar has a positive close on both, nothing can be indexed, so 600519 is drawn on its own price scale, and drawings and hit tests use that same scale.
- Overlay keeps one volume strip and the open sub-panes (MACD, and KDJ when that toggle is on), all for 600519. 000001's volume, MACD, and KDJ are not drawn. 000001's header is the bar's OHLCV and index, not its MA, BOLL, MACD, or KDJ.
- The panes share one horizontal viewport in both layouts. Dragging or scrolling pans and zooms that viewport, so the same bar indexes stay aligned.
- One crosshair. In Stack, moving the pointer in either pane selects one bar index and draws that vertical line through that symbol's price plot, volume strip, and open sub-panes, and through the other symbol as well. The symbol under the pointer also draws the horizontal line. Each header shows that symbol's own OHLCV, and the MA, BOLL, MACD, and KDJ values when those indicators are on. The time axis is drawn once, under the lower symbol. In Overlay the same index drives both headers, and the vertical line runs through the one price plot, the primary symbol's volume strip, and that symbol's open sub-panes.
- Candles with wicks. Red when close is at or above open, green otherwise (A-share convention). **Hollow** draws a rising body as an outline and keeps a falling body filled. Off, both bodies are filled. Volume bars and the MACD histogram stay filled either way.
- MA5 and MA20 on each price plot. The **MA** toggle turns both lines off together. It starts on.
- MACD(12, 26, 9) in a sub-pane under each volume strip: DIF, DEA, and a histogram of `2*(DIF-DEA)`, on that pane's own Y scale. The **MACD** toggle starts on. Turning it off removes the sub-pane and the MACD readout.
- **BOLL(20, 2)** on the price plot: the middle line is SMA20, and the upper and lower lines are that average ± 2 population standard deviations. The **BOLL** toggle starts off. The crosshair readout shows `MID`, `UP`, and `LOW`.
- **KDJ(9, 3, 3)** in its own sub-pane under MACD (or under volume when MACD is off). The Y scale covers 0–100 and widens when K, D, or J leaves that range. The **KDJ** toggle starts off. The readout shows `K`, `D`, and `J`.
- Volume is the built-in VOL strip between price and the sub-panes. It is not a separate indicator and has no toggle.
- Colors come from `kline.Style`, built from the window theme by `StyleFrom`. Light and dark each get their own plot, grid, crosshair, and MA / MACD colors. Rising stays red and falling stays green on both. `DarkDefault` and `LightDefault` are those two styles. `LimitUp` and `LimitDown` are brighter versions of those two. They paint a candle only when a host sets a positive limit percent; the default percent is 0.
- Toolbar **Classic / US / Contrast** replaces the candle colors and calls `SetStyle`. Classic is red up, green down. US swaps them. Contrast is yellow up and blue down. **Limit 10%** sets the percent to 0.10, and a second click clears it. The sample walk rarely moves 10% in one bar, so the limit colors usually stay unused until a host feeds a series that does. **Hollow** still outlines a rising body, including a limit-up body.
- In Stack, the price scale of each pane follows that symbol's visible bars, including BOLL when that toggle is on. In Overlay, the one price scale follows the indexed range of both series, including the primary symbol's moving averages and Bollinger bands when those toggles are on.
- **Reset view** returns both panes to the latest bars at the default width.
- **1h / 4h / 1d** reload both panes from the data source and fit the latest bars. Synthetic 1h bars open at 09:30, 10:30, 13:00, and 14:00 Asia/Shanghai; 4h and 1d synthetic bars use one weekday 09:30 open.
- Dragging to the left edge loads older bars, prepends them, and shifts the shared offset by that count so the bars on screen stay put. A load already in flight does not start another.
- **Pan / H-Line / Segment / Fib.** Pan drags the chart. H-Line places a horizontal price line with one click; dragging it changes the price. Segment takes two clicks, with a rubber band between them. **Fib** takes two clicks and draws seven standard retracement levels (0 through 1) across the horizontal span between the anchors, with a level preview while the second click is pending. Ratio 0 is the second click and ratio 1 is the first click. A draw tool turns pan off. The selected drawing is highlighted. Delete removes it. Escape cancels a drawing that does not have its second point yet, and so does a right-click; that right-click does not also delete a finished drawing. With no draft up, a right-click removes the drawing under the pointer. A press that arrives before the first frame is applied once the price plot has a size. Turning a sub-pane off grows that plot before the next frame, and a click in the new area is accepted. Drawings belong to the pane that was clicked. In Overlay, new drawings are stored on the first symbol in raw price, and the second symbol's existing lines stay hidden until you return to Stack.
- **Trade markers.** `SetMarkers`, `AppendMarker`, and `ClearMarkers` store buy and sell arrows per symbol. A buy points up from below the bar's low and a sell points down from above the high. `Time` picks the bar whose open interval contains it, so a period change remaps the trade and a forward prepend keeps it on that bar. A time outside the loaded bars is hidden. `Price` 0 uses the bar; any other price also draws a tick there. Markers on one bar stack. Their arrows, labels, and price ticks stay inside the price plot. `Style.Buy` and `Style.Sell` color them and default to `Up` and `Down`, so Classic, US, and Contrast recolor the arrows with the candles. The header names the markers on the hovered bar.
- **Replay.** `AppendBar` and `UpdateLast` push or revise the newest bar without recalculating the whole series. MA, MACD, BOLL, and KDJ implement `BarTail`, so an append updates a rolling sum or the last window. The view follows the right edge when it was already there, and stays put when it was panned. `StartReplay` keeps the full series private and shows only the revealed prefix, so a hidden bar cannot move the price or volume scale, an indicator, the crosshair, the readout, or an overlay compare. While a replay is attached, `AppendBar` and `UpdateLast` return `ErrReplayActive` and change nothing: only the replay adds bars. `Step`, `StepBack`, `SeekIndex`, and `SeekTo` move the cursor. `Play` / `Tick` reveal bars at a speed in bars per second and do not start a goroutine; `Chart.View` ticks once per frame, and a reveal that fails stops playback instead of asking for frames that cannot advance. A callback that closes, pauses, or replaces the replay stops the current reveal batch at the bar already returned to `OnBar`. A successful `StartReplay` clears `LoadError`: the replay supplies its own bars. A higher period during replay aggregates only the revealed bars, so the last bucket can still be forming. Markers are clipped by the revealed base interval before aggregation, so a trade inside a partial coarse bucket stays hidden until its base bar is revealed. The demo's **Replay / Step / Back / Play / Speed** buttons drive that.
- **Magnet Off / Weak / Strong.** Off leaves the pointer price alone. Weak snaps to the nearest Open, High, Low, or Close on the bar under the pointer when that OHLC is within 6 DIPs in local Y. Strong always snaps to the nearest OHLC. Any other `MagnetMode` does not snap. `SetMagnet` returns `ErrUnknownMagnet` for a value other than those three and leaves the current mode. `NewChart` treats an out-of-range `ChartOptions.Magnet` as off. New points snap, including the press that places a new H-Line, and so do endpoint drags on Segment and Fib. Dragging an existing H-Line body does not. Changing the mode does not rewrite lines already placed. Default is Off.

## Run

MyGo needs **Go 1.27.1 or later**. This module pins `github.com/egoist/mygo` v0.5.5 and the `mygo` CLI as a tool, so the CLI matches the library.

```bash
go build -o k-line .
go test ./... -count=1
go test ./... -race -count=1
go run .
```

`go build -o kline .` does not write a root binary: `kline` is the library directory, so Go places `kline/KLineChart-go` inside that package. `-o k-line` matches the name `mygo build` uses.

`go test` runs the allocation-budget tests. The benchmark matrix is not part of that command; see [Benchmarking](#benchmarking).

`-race` needs cgo and a C compiler, so it is not available with the default `CGO_ENABLED=0`: it fails with `-race requires cgo`. Windows is the primary target. Install a MinGW-w64 GCC toolchain, put its `bin` directory on `PATH`, and run with `CGO_ENABLED=1`; a portable distribution works and does not need to change machine-wide settings. CI runs both the plain and the race suite on Windows amd64. The other targets remain build-compatible but are not the current acceptance baseline.

With the CLI, from this directory:

```bash
go tool mygo dev     # rebuilds and restarts when a .go file changes
go tool mygo build   # packages the app into build/<os>-<arch>/
```

Install the CLI on `PATH` only if you want `mygo` without `go tool`:

```bash
go install github.com/egoist/mygo/cmd/mygo@v0.5.5
mygo dev
```

Packaging (`mygo build`) is optional. `go build` does not need cgo.

### Windows, macOS, Linux

Native UI draws with Direct3D 11 on Windows, Metal on macOS, and OpenGL on Linux. A native window does not use WebView2, Edge, or WebKit.

- **Windows.** Go is enough to build and run. The WebView2 runtime is not required. MinGW is only needed for `-race`.
- **macOS.** Go is enough. `mygo dev` builds a development app bundle so its window position stays separate from a packaged app.
- **Linux.** Building needs no cgo. Running the window needs GTK 3, which the desktop already has on current GNOME and KDE sessions (`libgtk-3-0` on Debian and Ubuntu). WebKitGTK is only for MyGo windows that load a web page; this demo does not.

`go test` drives the view with `ui.NewTester`, which renders frames in memory and does not open a window, so CI needs no display or window server. It runs `.github/workflows/ci.yml`: build, vet, `gofmt -l`, `go test`, and `go test -race` with MinGW on Windows amd64. Windows 11 was also checked manually with a real Direct3D 11 window for pan, zoom, crosshair, drawing, replay, live updates, and extension registration. A display is still required for manual visual checks.

## Embed

Another module can import the chart and skip this demo's toolbar. `Chart.View` is the MyGo view: pass it to `ui.View` for a window, or call it under the host's own chrome. It draws the stack (or overlay) candles, volume, and the indicators named in the options. Pan, zoom, and the crosshair work with no toolbar. The host calls the setters for anything the toolbar used to do.

```go
chart := kline.NewChart(kline.ChartOptions{
    Source:     src, // nil uses SyntheticSource; LiveSource is optional
    Symbols:    []string{"600519", "000001"}, // unique names; empty uses these A-share examples
    Period:     kline.Period{Span: 1, Unit: kline.UnitHour},
    Layout:     kline.LayoutStack, // or kline.LayoutOverlay
    Indicators: []string{"MA", "MACD"}, // nil keeps MA and MACD; empty turns all off
    Magnet:     kline.MagnetOff,
    HideHeader: true, // host draws its own bar; Readout still returns the text
    // PriceDecimals: &digits, // nil keeps two decimals; Location nil is Asia/Shanghai
})
mygo.NewWindow(mygo.WindowOptions{
    Title:   "Chart",
    Width:   960,
    Height:  720,
    Content: ui.View(chart.View),
})
if err := chart.LoadError(); err != nil {
    // the first GetBars or SubscribeBar failed; chart.SetPeriod(chart.Period()) retries
}

_ = chart.SetPeriod(kline.Period{Span: 1, Unit: kline.UnitDay})
_ = chart.SetIndicators([]string{"MA", "BOLL"}) // nil or empty turns all off; an unknown name changes nothing
_ = chart.SetLayout(kline.LayoutOverlay) // unknown name returns ErrUnknownLayout and keeps the current layout
_ = chart.SetSource(src) // nil returns ErrNilSource; a failed reload keeps the previous source
chart.EditStyle(nil, func(s *kline.Style) {
    s.Up = ui.RGB(239, 83, 80) // rising red, rest of the palette stays
    s.LimitPct = 0.10
})

symbol, index, ok := chart.Crosshair()
if ok {
    bar, _ := chart.Bar(symbol, index)
    _ = bar
    _ = chart.Readout(symbol) // OHLCV, indicators that are on, and markers on that bar
}
if bar, ok := chart.Bar("600519", chart.Bars()-2); ok {
    _ = chart.SetMarkers("600519", []kline.Marker{{Time: bar.Time, Side: kline.SideBuy, Qty: 1}})
    _ = chart.AppendMarker("600519", kline.Marker{Time: bar.Time, Side: kline.SideSell, Price: bar.Close, Label: "S"})
    _ = chart.ClearMarkers("600519")
}
series := map[string][]kline.Candle{}
for _, sym := range chart.Symbols() {
    cs := make([]kline.Candle, chart.Bars())
    for i := range cs {
        cs[i], _ = chart.Bar(sym, i)
    }
    series[sym] = cs
}
replay, _ := kline.StartReplay(chart, series) // hidden bars are not on the chart
replay.OnBar(func(ev kline.ReplayBar) {})     // host records a trade here
_ = replay.SeekIndex(48)
_ = replay.Step(1)
replay.Play(2) // bars per second; Chart.View calls Tick
first, last := chart.Visible()
_, _ = first, last
```

`NewChart` with a `Style` that only sets a candle color fills the rest from `DarkDefault`. A zero color on that partial style means "not set". `Style.Clear` names fields that should stay transparent, for example `ClearGrid`. A style that sets `Plot`, `Grid`, or `Text` is kept as given, so a transparent field there needs no bit. `EditStyle` starts from the theme you pass (`nil` is dark), keeps the live hollow switch and limit percent, then applies the edit. `SetHollow` and `SetLimitPct` change those two switches without dropping the color override, and both write the stored style when one is set. `Style.CandleBodyColor` is the rule Paint uses: a non-positive `limitPct` always returns `Up` or `Down`. `Style.Buy` and `Style.Sell` color trade markers. Left at zero, they follow `Up` and `Down`, including a partial palette that only sets those two. `ClearBuy` and `ClearSell` keep them transparent. The demo's Classic, US, and Contrast buttons set the marker colors to the same pair as the candles.

`SetMarkers(symbol, markers)` replaces that symbol's trades. `AppendMarker` adds one. `ClearMarkers` removes them. `Markers` returns a copy. An unknown symbol is `ErrUnknownSymbol`. A side other than `SideBuy` or `SideSell` is `ErrUnknownMarker` and does not change the stored list. `Marker.Time` is the lookup key and is not unique: every marker whose time falls in `[bar open, next bar open)` is drawn on that bar, and the last bar extends one period past its open. Times outside the loaded series stay stored and are not drawn. `Price` 0 anchors the arrow on the bar low or high. Any other price also draws a short tick at that price. An empty `Label` is `B` or `S`. A non-zero `Qty` is appended to the label. Buys point up from below the low; sells point down from above the high. Several markers on one bar step away from the candle so they do not overlap; the step is assigned in one pass, so a bar carrying thousands of trades does not cost more than a linear walk. Each symbol keeps its own list in both Stack and Overlay. A period change and `SetSource` keep the markers and map them onto the new bars. A forward prepend shifts the bar index with the candles. The hovered bar's header lists each marker on it.

`AppendBar(symbol, bar)` holds the bar until every symbol has that open time, then appends them together and updates indicators from the new close. `UpdateLast` revises the newest bar in place and does not move the viewport. A time that is not the edge returns `ErrBarTime`. An unknown symbol is `ErrUnknownSymbol`. While a replay is attached both return `ErrReplayActive` and change nothing. A view on the right edge follows an append. A panned view does not.

`StartReplay(chart, bars)` copies a full series and shows none of it until `Step` or `SeekIndex` / `SeekTo`. The chart panes only contain the revealed prefix. `OnBar` runs once for each base bar that becomes visible, on the caller, so a host can record a trade without a future bar on screen. `Play(barsPerSec)` and `Tick` use a clock (`SetClock` in tests). `Pause` and `Close` stop playback. A callback that pauses stops the current `Tick` batch; a callback that closes or starts a replacement replay stops the old batch immediately and no later bar from it reaches the new chart. A reveal that fails stops playback too, so a chart that cannot advance does not keep requesting frames. There is no replay goroutine. `Chart.Close` detaches the replay and fences live handlers; the replay drops the live subscription when it starts. `SetPeriod` during replay aggregates the revealed bars into the coarser period, partial bucket included, and does not call `GetBars`. A finer period returns `ErrReplayPeriod`. A successful `SetSource` ends the replay and loads that source; a `SetSource` that returns `ErrNilSource` or a load error leaves the replay running. `Replay.Close` keeps the revealed bars and restores each pane's `more` flag, so paging left asks the source again. The hidden suffix comes back from a later `SetPeriod` (the same period works: a replay close is not final, like `Chart.Close`) or `SetSource`. Markers are filtered by the revealed base interval before they are mapped to an aggregate bar, so markers whose base bar is still hidden are not drawn or listed in the readout. Drawings stay across a step; a period change drops them, as a normal `SetPeriod` does. An index past the revealed prefix does not crash a paint.

`ChartOptions.PriceDecimals` is the number of digits after the decimal in `Readout` and in the crosshair price tags. Nil, or a negative value, keeps two decimals (`FormatPrice`). Zero prints a whole number. `FormatPriceDigits` and `FormatBarIn` are the same formatting for a host that draws its own label. `ChartOptions.Location` is the time zone of those times. Nil is Asia/Shanghai for the A-share default; pass `time.UTC` for UTC. A direct `Paint` or `PaintCompare` call stays at two decimals and UTC. Indicator `ValueAt` strings still use `FormatPrice`.

`NewChart` has no error result. `LoadError` reports a first load whose `GetBars` failed (the panes stay empty) or whose `SubscribeBar` failed (the bars have no live updates). An invalid `SetPeriod` returns `ErrInvalidPeriod` before the source is asked and leaves the loaded bars in place. `SetPeriod` with the period already loaded does nothing while `LoadError` is nil: drawings, a segment in progress, and the live subscription stay. Earlier versions reloaded and dropped the drawings. While `LoadError` is set, the same period loads again. A real period change and `SetSource` drop drawings, any drawing or drag in progress, and the crosshair. `SetSource` reloads even when it is handed the source already in use, which is how a host refreshes a plain `DataSource`. The chart copies every slice `GetBars` returns, so a source may reuse its buffer.

`ChartOptions.Marshal` hops a live bar onto the UI thread. Pass `win.Update` (`(*mygo.Window).Update`): it runs the callback on the main thread and redraws. `mygo.RunOnMain` hops and waits, and does not redraw, so the host must `Invalidate`. A nil `Marshal` applies the bar on the caller and does not request a frame. `SetPeriod` and `SetSource` return a `SubscribeBar` error; subscriptions opened for that call are rolled back. `Close` unsubscribes and ignores a handler that runs afterward. It is not final: a later `SetPeriod`, including the current period, or `SetSource` loads again and resubscribes, which is how a host hides and shows a chart. The same-period no-op applies only while the chart is open and `LoadError` is nil. The live callback must not call `SetPeriod`, `SetSource`, or `View`.

`kline/host_embed_test.go` is the same path from an external package: embed with no toolbar, a host `DataSource`/`LiveSource`, a registered indicator, and a registered overlay.

## Extend

Register indicators and drawings before `NewChart`, usually from `init`. A chart that already exists picks up new specs from `SyncIndicators`, which keeps every existing toggle in place: a `*bool` from `IndicatorEnabled` that a `ui.Toggle` holds stays bound. `Unregister` and `UnregisterOverlay` remove a host entry; the chart drops an indicator on the next `SyncIndicators`. Built-in names and tool names stay reserved, and `UnregisterOverlay` does not remove them. The lists are safe to read and update from more than one goroutine. `ChartOptions.Indicators` and `SetIndicators` read nil differently: nil options keep MA and MACD, and `SetIndicators(nil)` turns everything off.

### Indicator

`PaneMain` plus `LinePlot` is the line case. `Series` widens the price scale. When `Series` is non-nil, Paint strokes those lines and does not call `DrawMain`, so leave `DrawMain` empty; an empty non-nil slice is still the line case. A main indicator that paints fills or markers instead of lines leaves `Series` nil and implements `DrawMain`. Overlay mode does not call `DrawMain`.

`PaneSub` opens a band under volume, after MACD and KDJ when those are on. Implement `DrawSub`. `SubPlot.Scale` supplies the Y axis; without it the axis is 0..1. `ValueAt` is the header text.

```go
type band struct {
    values []float64
    series [][]float64
    color  ui.Color
}

func (b *band) Name() string     { return "BAND" }
func (b *band) PaneKind() string { return kline.PaneMain }
func (b *band) Calc(cs []kline.Candle) {
    b.values = make([]float64, len(cs))
    for i, c := range cs {
        b.values[i] = c.Close
    }
    if cap(b.series) < 1 {
        b.series = make([][]float64, 1)
    }
    b.series = b.series[:1]
    b.series[0] = b.values
}
func (b *band) DrawMain(*ui.Painter, ui.Rect, kline.Viewport, chart.Linear) {}
func (b *band) DrawSub(*ui.Painter, ui.Rect, kline.Viewport)                 {}
func (b *band) ValueAt(i int) string {
    if i < 0 || i >= len(b.values) {
        return ""
    }
    return "BAND " + kline.FormatPrice(b.values[i])
}
func (b *band) Series() [][]float64    { return b.series }
func (b *band) LineColors() []ui.Color { return []ui.Color{b.color} }

func init() {
    _ = kline.Register(kline.Spec{
        Name: "BAND",
        Kind: kline.PaneMain,
        New:  func() kline.Indicator { return &band{color: ui.RGB(255, 140, 0)} },
    })
}
```

`Register` returns `ErrDuplicateIndicator` when the name is already in the list. `New().PaneKind` must equal `Spec.Kind`; a mismatch returns `ErrIndicatorKind` and is not registered. Layout uses the instance kind, which is then the same value the spec declared. MACD and KDJ keep their existing rects. A host sub-pane is an extra band and does not move those two.

### Overlay

`RegisterOverlay` adds a tool `SetTool` accepts. `Anchors` is 1 (click to place, drag to move) or 2 (two clicks, then drag an endpoint). `pan`, `hline`, `segment`, `fib`, and `fibonacci` are reserved. `ToolFib` and `OverlayFibonacci` are both `"fibonacci"`, and `SetTool("fib")` selects that same tool, so `Tool()` matches `Overlay.Kind` on a Fibonacci drawing. Delete and right-click use the same hit test as the built-ins. `Draw` must not change `o.Points`: the chart passes the layer's own slice.

```go
_ = kline.RegisterOverlay(kline.OverlaySpec{
    Kind:    "mark",
    Anchors: 1,
    Draw: func(p *ui.Painter, plot ui.Rect, o kline.Overlay, vp kline.Viewport, scale chart.Linear, cs []kline.Candle, _ kline.Style) {
        if len(o.Points) == 0 {
            return
        }
        x := plot.X + float32(kline.AnchorX(o.Points[0], vp, cs))
        y := plot.Y + float32(kline.PriceToLocalY(float64(plot.H), o.Points[0].Price, scale))
        p.Fill(ui.Rect{X: x - 4, Y: y - 4, W: 8, H: 8}, ui.RGB(0, 220, 255), 1)
    },
    Hit: func(o kline.Overlay, x, y, _, plotH float64, vp kline.Viewport, scale chart.Linear, cs []kline.Candle) (int, bool) {
        if len(o.Points) == 0 {
            return 0, false
        }
        ax := kline.AnchorX(o.Points[0], vp, cs)
        ay := kline.PriceToLocalY(plotH, o.Points[0].Price, scale)
        if math.Hypot(x-ax, y-ay) <= kline.HitRadius {
            return kline.HitBody, true
        }
        return 0, false
    },
})
_ = chart.SetTool("mark")
```

### Data source

History is `DataSource.GetBars`, in strictly ascending open time. The chart sends `KindInit` and `KindForward` only. `KindBackward` is reserved and is never requested. A batch that is out of order, or that repeats an open time, is refused with `ErrBarOrder`: aggregation, the marker lookup, and the overlay point lookup all binary-search the series, so a swapped bar would silently attach a trade or a drawing to the wrong candle. A feed that can push also implements `LiveSource`. The chart subscribes when `SetSource` or `SetPeriod` sees that interface, and ignores a plain `DataSource`. Older bars are prepended only when every pane that already has bars accepts the same count; otherwise the shared offset stays put and each pane keeps the `more` flag the source reported. A forward `GetBars` error leaves the panes unchanged and is not retried on the next frame. `SyntheticSource` is not a live feed. `StubLiveSource` is the in-memory double. This repo does not open a connection. `kline/host_embed_test.go` has a complete feed: init, forward, subscribe, and one pushed bar.

Every symbol shares one bar index: bar `i` is the same open time on each pane. That is what lets one viewport, one crosshair, and the overlay base work across symbols, so the chart enforces it wherever bars arrive:

- **Load.** The init windows are trimmed to the open times every symbol has. Bars before the latest first open time are dropped; a forward load brings them back once every symbol has them. Bars after the earliest last open time are held, which covers one feed having opened the next bar before the other, and the newest 512 are kept. If what is left still differs in open time, or one symbol has no bars while another does, `SetPeriod` and `SetSource` return `ErrMisaligned` and keep the previous bars. `NewChart` reports it through `LoadError`. Pending bars beyond the cap are a successful load with an `AlignError`, because the aligned window remains usable.
- **Forward.** Only the trailing run of older bars whose open times every symbol shares is prepended. A batch that ends on different open times is left out, and `AlignError` reports it.
- **Live.** A bar with a new open time is held until every symbol has a bar at that open time. Then those bars are shown together, and a view on the right edge follows them. A revision of a symbol's newest bar, held or shown, replaces it. A held bar that another symbol skipped, because that feed already moved on to a later open time, is dropped, and `AlignError` reports it. A symbol whose feed stops also holds back the other symbols' new bars. One pane holds at most 512 pending bars: past that the oldest are dropped, keeping the newest open times the other symbols can still match, and `AlignError` reports the drop.

Symbol names must be unique. `NewChart` with a repeated name has no panes, and `LoadError` wraps `ErrDuplicateSymbol`. An empty series pins the viewport to the left margin like any series shorter than the plot.

Indicator series are cached until the candles change. Pan and zoom do not call `Calc`. An indicator that is off is not calculated when bars load, prepend, or arrive live; it is calculated once when it is turned on, if the bars changed while it was off.

## Examples

Runnable examples live in `examples/`. Each is a `main` package with a test that drives the same code headlessly, so none of them can rot:

| Example | Run | Shows |
| --- | --- | --- |
| `examples/live` | `go run ./examples/live` | A host `DataSource` + `LiveSource` pushing bars from its own goroutine, `ChartOptions.Marshal` set to `window.Update`, a period toolbar, and `Close` on window close. |
| `examples/extend` | `go run ./examples/extend` | A host indicator registered with `kline.Register` (a `LinePlot` that widens the price scale) and a host drawing tool registered with `kline.RegisterOverlay`, then selected with `SetTool`. |
| `examples/replay` | `go run ./examples/replay` | Blind playback: `StartReplay`, `OnBar`, a coarser `SetPeriod` over the revealed prefix, the refusal of a finer one, `Play`/`Tick` on a test clock, and `AppendBar` returning `ErrReplayActive`. |

`kline/example_test.go` holds the API-level `Example` functions. `go test` compiles and **runs** them and compares their output, so they are the closest thing to a guarantee that the documented calls still work:

```bash
go test ./kline -run Example -v          # just the examples
go doc github.com/shenjing023/KLineChart-go/kline   # same text in the package docs
```

They cover `NewChart`, `ChartOptions`, `Chart.View`, `SetPeriod`, `SetLayout`, `SetIndicators`, `Bar`, `SetMarkers`, `Period`, and `FormatPrice`.

## Performance

Overlay mode used to normalize every bar on each frame. On 8000 bars at 960×720 (`go test ./kline -bench Benchmark -benchtime=400ms -count=1`):

| Path | Before | After |
| --- | --- | --- |
| `CompareScales` | 307 µs, 910 KB/op | 10.7 µs, 15 KB/op |
| Overlay pan frame | 3.50 ms, 2.91 MB/op | 2.64 ms, 194 KB/op |
| Stack pan frame | 3.24 ms, 190 KB/op | 2.67 ms, 191 KB/op |

Stack paint already walked the visible bars. The stack frame is mostly MyGo layout, so it was left alone. `BenchmarkIndicatorCalc/8k` is MA, BOLL, MACD, and KDJ on 8000 bars, and it runs only when candles are replaced. `TestCompareScalesAllocationBudget` fails if the overlay scale copies the whole series again (budget 48 KB/op at this window). That table is the P9 before/after. Later runs compare against `bench/baseline.txt`, whose rows predate the P10+ fixes; check bytes/op against the test ceilings instead.

An overlay pan uses the scale the frame already computed, for both the candles and the drawings, and `BOLL.Series` keeps one header until the next `Calc`. `TestOverlayPanAllocationBudget` fails if `BenchmarkOverlayPanFrames/8k` goes above 92 KB/op. That is less than the cost of computing the overlay scale a second time. `TestStackPanAllocationBudget` does the same for the default layout, which paints two full panes: 216 KB/op.

## Benchmarking

`kline/perf_bench_test.go` is a fixed matrix: 1k, 8k, and 20k bars from `Generate`, at 960×720. Names look like `BenchmarkPanFrames/stack_8k` and `BenchmarkOverlayPanFrames/8k`. The same file also times zoom frames, crosshair moves, and a `StubLiveSource` revise of the last bar plus one paint. Pan and zoom alternate direction so the window does not walk into the clamp. `go test` does not run these; pass `-bench`.

```bash
go test ./kline -run '^$' -bench . -benchmem -count=6
./bench/run.sh                  # writes bench/baseline.txt
go test ./kline -run '^$' -bench . -benchmem -count=6 > bench/new.txt
go tool benchstat bench/baseline.txt bench/new.txt
```

`benchstat` is the `golang.org/x/perf/cmd/benchstat` tool in `go.mod`. Compare runs from the same OS, architecture, and Go version. Machine notes and the column list are in `bench/README.md`.

## Layout

Module `github.com/shenjing023/KLineChart-go`. The root is the demo. The library is `kline/`.

- `main.go` — window and toolbar. Buttons call `kline.Chart`. `main_test.go` drives that toolbar with `ui.NewTester`. `mygo.json` is the app name, identifier, version, and `build/` output directory.
- `kline/chart.go` — `Chart.View`, pane paint, and the readout. `chart_opts.go` is `ChartOptions`, `NewChart`, and the setters. `chart_load.go` loads history and prepends. `chart_live.go` subscribes and fences live handlers. `chart_input.go` is pan, zoom, and drawing input.
- `kline/data.go` — `Period`, `BarRequest`, `DataSource`, `LiveSource`. `synthetic.go` is `SyntheticSource` (no network, not a live feed): its walk is generated backwards from a fixed newest session, so a bar keeps the same OHLCV in every request, and a period whose step does not fit the two-hour sessions (3h, 150m) returns `ErrInvalidPeriod` instead of having no bars. `live_stub.go` is the in-memory `StubLiveSource`.
- `kline/indicator.go` — `Indicator`, optional `BarTail`, `LinePlot`, `SubPlot`, and `Register`. Built-ins are `ind_ma.go`, `ind_macd.go`, `ind_boll.go`, `ind_kdj.go`.
- `kline/overlay.go` — `Point`, `Overlay`, `Layer`, `RegisterOverlay`. Drawings are `ov_hline.go`, `ov_segment.go`, `ov_fibonacci.go`. `magnet.go` is `MagnetMode` and `SnapPrice`.
- `kline/style.go` — `Style`, `StyleFrom`, `DarkDefault`, `LightDefault`, `CandleBodyColor`. `paint.go` draws candles and the volume strip. `compare.go` indexes the overlay. `viewport.go` is the shared window. `layout.go` splits the panes. `candle.go` is `Candle` and `Generate`.
- `kline/marker.go` — `Marker`, `SetMarkers`, `AppendMarker`, `ClearMarkers`. `append.go` is `AppendBar` and `UpdateLast`. `replay.go` is `StartReplay`.
- `kline/example_test.go` — runnable `Example` functions for the exported API. `go test` compiles and runs them, so an example that drifts from the API fails the build.
- `examples/live/`, `examples/extend/`, `examples/replay/` — small programs (`go run ./examples/live`) that show a pushing feed, a host indicator plus drawing tool, and blind playback. Each has a test that drives the same code headlessly, so they cannot rot.
- `bench/` — `run.sh` writes `baseline.txt`. Column notes are in `bench/README.md`.
- `docs/plans/` — historical P0–P9 plans, not a work queue. `docs/reviews/2026-10-20-full-review.md` and `docs/reviews/2026-10-21-full-review.md` are the reviews. Each Status table is its resolution log: when a finding is fixed, add a row with the finding, what changed, and the commit. Leave the original finding text as written.
- `AGENTS.md` — command list and invariants for coding agents. Behavior and the embed API stay in this file.

Bar `i` starts `(i - offset) * barWidth` DIPs from the left of every price plot. Dragging right decreases `offset` for all panes. The wheel keeps the bar under the pointer fixed while the width changes, and the other pane uses that same width. In Stack, each pane's Y axis is its own `chart.Linear` over that symbol's visible high/low. In Overlay, both series share one index scale: `close[i] / close[base] * 100`, and the primary symbol's OHLC uses that same divisor. `Paint` takes the shared bar index so a pane draws the vertical line even when the pointer is on the other one.

## P0

本阶段在原有双窗联动图上补了数据层，没有改画布和手势模型。

能做：

- 通过 `kline.DataSource.GetBars` 取数。当前只有 `SyntheticSource`（本地合成，不访问网络）。
- `init` 取最新一段（默认 600 根）。`forward` 取锚点之前的更早 K 线，时间不重叠。`backward` 只留在接口里，图表不会发出这个请求，合成源返回错误。
- 工具条 `1h` / `4h` / `1d`。切换时两个 symbol 一起重新加载，再 `FitRight`。
- 共享 `Offset < 5` 且还有更早数据时，prepend 一批并加上同样的根数，画面不跳。加载过程中不重复请求。

明确不做（也不在本阶段开始）：

- 真实行情 HTTP、WebSocket、`subscribeBar`
- 缺会话的空隙压缩
- P2 画线
- P3 同轴叠加（本阶段不做，见下文）
- P4 外置样式、明暗主题、空心阳线

## P1

本阶段把主图均线和 MACD 收成可开关的指标。布局仍是两个 symbol 上下排列，时间轴只画在最底下。

能做：

- `kline.Indicator`：`Calc`、主图 `DrawMain`、副图 `DrawSub`、十字线 `ValueAt`。`kline.Registry` 列出已注册指标；窗口启动时抄进自己的列表，每个 symbol 各算一份。
- **MA** 开关。打开时主图画 MA5 和 MA20，读数带上这两条；关掉后两条线一起消失，价格轴也不再为它们留空。默认开。
- **MACD(12, 26, 9)** 副图。DIF = EMA12 − EMA26，DEA 是 DIF 的 9 周期 EMA，柱 = `2*(DIF-DEA)`。EMA 的第一笔用 SMA 做种子，和 KLineChart 的算法一致。副图有自己的 Y 轴，零轴在刻度里。默认开。
- 每个 symbol 自上而下是：价格（含 MA）、内置 VOL 量柱、MACD。只有最下面的 symbol 画时间轴。关掉 MACD 后，该副图行消失，回到价格 + 量柱。
- 十字线的竖线穿过价格、量柱和 MACD。指针所在的那一块另外画横线；停在 MACD 上时，右侧标出该副图的 Y 值。两个 symbol 的抬头都显示这一根的 DIF / DEA / HIST。
- VOL 仍是 `Paint` 里的量柱带，不是 `Indicator`。这一阶段不另做 VOL 开关。

明确不做：

- BOLL、SAR 等其余指标，以及运行时 `registerIndicator` 热加载
- 把 VOL 再包成可替换指标
- P3 同轴叠加（本阶段不做，见下文）、P4 样式外置

## P2

本阶段在价格区画线。指标和视窗没有改交互模型，画线是挂在每个 pane 上的一层。

能做：

- `kline.Point` 用 `BarIndex` + `Price` 记下位置。放上去的时候同时记下那根 K 线的开盘 `Time`。之后按时间找回这根 bar，所以向左加载、前面插入更早的 K 线时，线还停在原来那根上，不会跟着序号滑走。
- `kline.Layer` 是**一个 pane 自己的线**。一个 symbol 上的线不会出现在另一个上，反过来也一样。两个 pane 各有各的选中。
- **H-Line**：在价格区点一下放下一条水平价线。拖动改价格。选中时加粗，并在左端画一个方块。
- **Segment**：点两下。第一下之后，指针和第一点之间有一条橡皮筋；第二下完成。选中时两端有方块。在 Segment 工具下拖端点，可以改那一端的 bar 和价格。
- 工具条 **Pan / H-Line / Segment**。选了画线工具时，拖动不再平移。滚轮缩放仍然可用。切回 Pan 之后，拖动重新平移；在线上点一下（没有拖开）会选中它。
- **Delete** 删掉当前 pane 里选中的那条。**右键**删掉指针下面的那条，只影响这个 pane。
- 切换周期会重新加载 K 线，两个 pane 上的线一起清掉。

明确不做：

- 磁吸、斐波那契见 P7
- 笔刷
- 一条线同时画在两个 pane 上
- P3 同轴叠加（见下文）、P4 样式外置

## P3

本阶段加了同轴比较，没有改 Stack 的画法和手势。默认仍是 Stack。

能做：

- 工具条 **Stack / Overlay**。Stack 是原来的上下两栏：各自的价格、量柱、MACD。Overlay 收成一栏。
- Overlay 里主序列（第一个 symbol）仍是蜡烛。第二序列画收盘折线，不用第二套蜡烛。两套蜡烛叠在一起时实体会互相挡住，折线能看清相对走势。
- 归一化是 `close[i] / close[base] * 100`。`base` 是当前可见区间里，两列收盘都为正的第一根；可见区间里没有这样的 bar 时，退到整段序列里的第一根。主序列的开高低收都除以它在 `base` 的收盘，所以这根蜡烛的收盘是 100。第二序列除以它自己在同一根的收盘。Y 轴跟着两条归一化序列（以及打开时的主序列均线）的范围走。平移或缩放会改 `base`，两条都从新的左端重新回到 100。
- 视窗和十字线仍然共用。两个抬头都显示这一根的 OHLCV，并带上 `idx`（归一化后的收盘）。主序列的抬头继续带 MA 和 MACD。第二序列的抬头不带它自己的 MA / MACD。
- 量柱和 MACD 只留主序列的一条。第二序列的量柱和 MACD 不画，避免一栏里再塞第二套副图。
- 画线仍按原始价格存在主序列的 `Layer` 上。切到 Overlay 时用同一条价格到像素的映射，线还贴在主序列的蜡烛上。第二序列上已有的线先藏起来，回到 Stack 再出现。

明确不做：

- 双价格轴、对数轴、锁定 Y
- 第二序列的量柱或 MACD 副图
- 两条都画成蜡烛
- P4 样式外置、明暗主题、空心阳线

## P4

本阶段把颜色收到 `kline.Style`，并加了空心阳线。画线、指标算法、Stack / Overlay 的布局都没有改。

能做：

- `kline.Style` 在 `kline/style.go`。里面有涨跌色、网格、十字线、MA5 / MA20，以及 MACD 的 DIF、DEA 和零轴。柱状图（量柱和 MACD 柱）用同一套涨跌色。
- `StyleFrom(theme)` 按当前 MyGo 主题填这些颜色。`DarkDefault` 和 `LightDefault` 分别是暗色和亮色主题的结果。传入 nil 等于 `DarkDefault`。
- 明暗两套的绘图区、网格、十字线、坐标字，以及 MA / MACD 线，跟着主题的背景、边框、弱化文字、Accent、Warning 走。默认涨跌色按 A 股习惯红涨绿跌，不随明暗对调。
- 工具条 **Hollow**。打开后，收盘不低于开盘的 K 线只画框，收盘更低的仍是实心。关掉后两种实体都填满。Overlay 里的主序列蜡烛走同一条绘制，所以这个开关在两种布局里都生效。
- 画线颜色、比较折线的颜色也在 `Style` 里，绘制时从这里取，不再在各个绘制函数里写死。

明确不做：

- 调色板编辑器，或给每个 symbol 单独指定颜色
- 把量柱或 MACD 柱也改成空心
- 美国线、平均 K 线、面积图
- 导出图片的 API
- 行情订阅（接口在下一节；本阶段没有）

## P5

本阶段只定实时 K 线的契约，并给测试一个内存 stub。窗口仍用 `SyntheticSource` 演示，不连任何行情服务器。

能做：

- `kline.LiveSource` 在 `DataSource` 上增加 `SubscribeBar` / `UnsubscribeBar`。`GetBars` 仍负责历史。`BarHandler` 收到一根 K 线：新的开盘时间，或最后一根的修订。
- 图表在 `setPeriod` 里对数据源做 type assert。源实现了 `LiveSource` 就按当前周期给每个 symbol 订阅，并在换周期或换源时退订上一个源。没有实现就只走 `GetBars`，`SyntheticSource` 属于这一类，它不会假装在推送。
- 收到的 bar：开盘时间比最后一根新，就追加；开盘时间相同，就替换最后一根；更早的开盘时间丢掉。指标会重算。视窗正贴在右边缘时，新的一根仍留在右边；已经平移开的视窗不跳。
- `SetPeriod` / `SetSource` 把 `SubscribeBar` 的错误返回给宿主。同一轮里已经订上的 symbol 会退订。`Close` 退订并让已经交出去的 handler 不再改图。
- `ChartOptions.Marshal` 把实时 bar 送到 UI 线程。传 `win.Update`（`(*mygo.Window).Update`），它会在主线程执行并请求重绘。`nil` 表示就在调用方的 goroutine 上同步执行，测试走这条。`mygo.RunOnMain` 只跳线程、不重绘，宿主要自己 `Invalidate`。handler 不能再调用 `SetPeriod`、`SetSource` 或 `View`。
- `kline.StubLiveSource` 把历史和订阅都放在内存里。`SetBars` 写入历史，`Push` 在已订阅时调用 handler，`UnsubscribeBar` 之后不再投递。测试用它，不需要网络。

真实源由宿主实现，不在本仓库里。历史实现 `GetBars`；能推送时再实现 `LiveSource`。`Push` 的调用方决定在哪一个 goroutine 上进入 handler，stub 是同步调用。宿主从别的 goroutine 推送时，把 `ChartOptions.Marshal` 设为 `win.Update`。下面是形状，不是可运行的客户端：

```go
type feed struct{}

func (f *feed) GetBars(req kline.BarRequest) ([]kline.Candle, bool, error) {
	return nil, false, nil // history the host already has
}

func (f *feed) SubscribeBar(symbol string, period kline.Period, onBar kline.BarHandler) error {
	return nil // host starts delivering bars to onBar
}

func (f *feed) UnsubscribeBar(symbol string, period kline.Period) error {
	return nil
}
```

明确不做：

- HTTP、WebSocket，以及任何券商或行情客户端
- A 股 / 港股协议、鉴权、账号
- 在本仓库里替 dapu 接真实行情

## P6

本阶段加上打谱常用的两条：主图布林带，副图 KDJ。两条默认都关着，第一帧仍是价格、量柱、MACD。

能做：

- **BOLL(20, 2)** 画在价格区。中轨是收盘的 SMA20。上下轨是中轨 ± 2×标准差。标准差是窗口内 `(close − mid)²` 的平均再开方，除以 N，和 KLineChart 一致。三条线在窗口满 20 根时一起出现。
- 工具条 **BOLL**。默认关。打开后十字线读数带上 `MID` / `UP` / `LOW`。价格轴把上下轨算进去，带不会被裁掉。Overlay 里这三条跟着主序列的归一化价格走。
- **KDJ(9, 3, 3)** 单独占一条副图。RSV 用 9 根里的最高价和最低价；最高价等于最低价时分母按 1。K、D 的递推是 `((周期−1)×前值 + 当日值) / 周期`，没有前值时用 50。通常参数下就是 `(2×前值 + 当日值) / 3`。J = `3K − 2D`。和 KLineChart 的算法一致。
- 副图 Y 轴通常覆盖 0–100。K、D 或 J 超出这个范围时轴跟着拉开。
- 工具条 **KDJ**。默认关。Stack 里每个 symbol 自上而下是：价格、量柱、MACD（开着时）、KDJ（开着时）。只开 KDJ 时，它紧挨在量柱下面。Overlay 里副图仍只画主序列的。关掉后这一行消失，时间轴回到上面一条。
- 十字线的竖线穿过 KDJ。指针停在 KDJ 上时，右侧标出该副图的 Y 值。两个 symbol 的抬头都显示这一根的 `K` / `D` / `J`。

明确不做：

- RSI、SAR，以及其余指标
- 自定义脚本指标

## P7

本阶段加磁吸与斐波那契回撤线。

能做：

- `kline.MagnetMode`：`MagnetOff`、`MagnetWeak`、`MagnetStrong`。弱磁在指针的 local Y 与最近 OHLC 相差不超过 `HitRadius`（6 DIP）时，把新点或拖端点吸到该 bar 的 Open / High / Low / Close（跳过 NaN）。距离相同按 Open、High、Low、Close 先后。强磁总是吸到最近 OHLC。
- 工具条 **Magnet Off / Magnet Weak / Magnet Strong**，默认 Off。选中项为 `PrimaryButton`。新点会吸，包括按下时放下的新 H-Line，以及 Segment 与 Fib 的端点拖动。拖动已有 H-Line 的线身不吸，仍用原始指针价。切换磁吸模式不会批量改已有线的价格。
- 新 H-Line（含同一次按下里的移动）、新 Segment / Fib 端点、以及 Segment / Fib 端点拖动都会经过 `SnapPrice`。
- 工具条 **Fib**：两击在价格区定两个锚点，画出标准回撤水平 0 / 0.236 / 0.382 / 0.5 / 0.618 / 0.786 / 1（`FibRatios`）。比例 0 是第二次点击，比例 1 是第一次点击。水平线只横跨两锚点之间的 X 跨度，不铺满整幅图。第二击未落下时预览七条线；Segment 仍只显示一条橡皮筋。选中后只拖两个端点，Delete 与右键删除规则同 Segment。画线落在点击的那个 pane；Overlay 仍只记在主序列的原始价格上。

明确不做：

- 比例文字标注、射线 / 平行通道 / 笔刷、自定义比例
- 跨 pane 共享同一条 overlay

## P8

本阶段把窗体收成可嵌入的 `kline.Chart`。`main` 只留下窗口和工具条，工具条上的按钮改调 Chart 的方法。

能做：

- `kline.NewChart`。选项有 `Source`、`Symbols`、`Period`、`Layout`（`"stack"` 或 `"overlay"`）、`Style`、`Hollow`、`Indicators`、`Magnet`、`PriceDecimals`、`Location`。没填的项用演示默认值：合成源、600519 和 000001、1h、Stack、MA 和 MACD 开、磁吸关、价格两位小数、Asia/Shanghai 时间、颜色跟窗口主题。`PriceDecimals` 管表头和十字线价格；指标自己的 `ValueAt` 仍是两位。`Location` 为 nil 时用 Asia/Shanghai。
- `View` 是 MyGo 的视图函数。单独做窗口时用 `ui.View(chart.View)`。宿主有自己的工具条时，在那一栏下面调用 `chart.View(ctx)`。没有工具条也会画出 Stack 的 K 线、量柱和选中的指标，平移、缩放、十字线都能用。
- `SetPeriod`、`SetLayout`、`SetSource`、`Close`。换周期会重新加载并清掉画线。`SetSource(nil)` 返回错误。`SetLayout` 只接受 `"stack"` 和 `"overlay"`，别的字符串返回 `ErrUnknownLayout`，布局保持不变。`GetBars` 失败时保留原来的源。`SubscribeBar` 失败时返回错误，并退掉这一轮已经订上的 symbol。源实现了 `LiveSource` 仍会订阅，没有实现就只走 `GetBars`。`Close` 退订。
- 工具条还绑定了 `SetTool`、`SetMagnet`、`SetIndicator`、`SetHollow`、`ResetView`。`SetMagnet` 只接受关、弱、强，别的值返回 `ErrUnknownMagnet`，当前模式不变。指标开关和 Hollow 也可以直接拿 `IndicatorEnabled` / `HollowEnabled` 的 `*bool` 交给 `ui.Toggle`。
- `SetStyle` 换成一整套颜色。`SetLimitPct` 单独改涨跌停阈值。`Style.CandleBodyColor` 的规则是：`limitPct <= 0` 时只用 `Up` / `Down`（收盘不低于开盘为涨）。`limitPct > 0` 且上一根收盘为正时，`(close-prevClose)/prevClose >= limitPct` 用 `LimitUp`，`<= -limitPct` 用 `LimitDown`。收盘等于最高价本身不算涨停。量柱和 MACD 柱仍用 `Up` / `Down`。
- 演示工具条 **Classic / US / Contrast** 改 K 线颜色，买卖标记跟着同一对涨跌色。Classic 是 A 股习惯的红涨绿跌。**Limit 10%** 把阈值设为 0.10，再点一次关掉。默认阈值是 0，所以涨跌停色先备着，不画上去。Hollow 仍然只描涨的实体，涨停那根若收盘不低于开盘，描的是 `LimitUp`。
- `SetMarkers`、`AppendMarker`、`ClearMarkers` 按 symbol 存买卖标记。`Time` 落在哪根 K 线的开区间，就画在那根上；换周期会重映射，向左补历史时下标跟着移。数据之外的时间留着但不画。买在最低价下方朝上，卖在最高价上方朝下，同一根上多个标记会错开。`Price` 为 0 只用 K 线价，其它价格再画一个短横。`Style.Buy` / `Style.Sell` 着色，不填就跟 `Up` / `Down`。十字线表头会写出当前 bar 上的标记。未知 symbol 返回 `ErrUnknownSymbol`，非法方向返回 `ErrUnknownMarker`。
- `AppendBar` 在所有 symbol 都有同一开盘时间后追加一根，指标只更新新的这一点。`UpdateLast` 改最后一根，视口不动。贴着右边缘时追加会跟着走，平移过的视口留在原地。`StartReplay` 把完整序列留在图外，图上只有已经揭开的前缀，隐藏的 K 线进不了价格轴、量轴、指标、十字线、表头和叠加比较。`Step`、`StepBack`、`SeekTo` 移动游标。`Play` 按每秒根数揭开，不另起 goroutine，`View` 每帧调用 `Tick`。回放中换成更大周期时，只聚合已揭开的低周期 bar，最后一根可以是未走完的桶。更小周期返回 `ErrReplayPeriod`。游标之后的买卖标记不画。演示工具条有 **Replay / Step / Back / Play / Speed**。

明确不做：

- 打谱的对局流程（属于 dapu）。本仓库提供不偷看未来的逐根追加和回放，训练会话本身不在这里
- 打包成 C ABI 或动态库
- 从行情里读取涨跌停幅度。本仓库不连网络。阈值由宿主传入

## Out of scope

- Feature parity with KLineChart beyond the list above: the rest of the overlay set, a full indicator set, log scale, or session gaps.
- A WebView, or embedding the JavaScript chart.
- A market-data client. A host implements `DataSource` and, when it can push bars, `LiveSource`. This repo only defines that interface and an in-memory `StubLiveSource`. It does not open an HTTP or WebSocket connection.
- Installer polish beyond `go build` and `mygo build`.

## Limitations

- Bars are evenly spaced by index. The sample is hourly, so the time axis matches the bars. A real series with missing sessions would still be drawn one slot per bar.
- Each price scale refits the visible range. Stack gives each symbol its own scale. Overlay indexes both to 100 at the first visible common bar and refits that index range. There is no locked or manual scale.
- Zoom changes bar width between 3 and 28 DIPs. It is not KLineChart's gesture model.
- CI compiles and tests the view headlessly. It does not open the window, so it cannot catch a layout or gesture problem that only shows with a real display.
- A live bar is applied under `ChartOptions.Marshal` (normally `win.Update`), so a feed that pushes from its own goroutine must pass that hook. With a nil `Marshal` the bar is applied on the caller's goroutine, which is only safe there when the caller is the UI thread.
- `DataSource.GetBars` runs synchronously on the UI goroutine, inside a frame: `View` calls `ensureForward` while the user drags at the left edge. A source that reads from a slow disk or a network must keep its own cache so `GetBars` returns immediately.
- One pane holds at most 512 live bars waiting for the other symbols. A feed that stops for longer than that loses the oldest pending bars, and `AlignError` reports it.
