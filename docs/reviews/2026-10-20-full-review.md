# KLineChart-go full review (2026-10-20)

Reviewed tree: `main` at `dc6f471` (`P9: host embed/extend DX + overlay pan perf`, PR #12). History on that commit is P0–P9. There is no P10 commit.

`go test ./... -count=1` on that tree: `github.com/shenjing023/KLineChart-go` ok (0.082s), `kline` ok (2.044s). This change adds only this document.

## Executive summary

The demo path is in good shape. Viewport pan/zoom/clamp, SMA / MACD / BOLL / KDJ formulas, overlay hit-testing, magnet, hollow and limit colors, and the P9 visible-window compare scale all match their tests. Stack and overlay paint walk the visible bars. Pan does not call `Indicator.Calc`.

The host contract is the weak part. The chart assumes every symbol shares one bar index and one open time, then `ensureForward` advances that shared offset even when a pane did not grow. Live updates are closures with no generation, no mutex, and no frame request, so a real feed can write the chart after unsubscribe or from a background goroutine while `View` is reading it. Several setters disagree with their own comments once a host leaves the synthetic two-symbol demo: `SetPeriod` / `SetSource`, `SetIndicators(nil)`, and `IndicatorEnabled`.

Nothing below was changed in code. Fix direction is in each finding and in the ordered PR list at the end.

## Critical (must fix)

### 1. Forward load shifts the shared offset when a pane did not prepend

`kline/chart.go` `ensureForward`.

The chart's invariant, stated on `series` and in the README, is that bar `i` is the same open time on every symbol. `Viewport.Offset` is one number for all panes. `bars` is the shortest series. Crosshair, overlays (`Point.Index`), and compare mode all use that index.

`ensureForward` does not keep the invariant:

- A pane with `more == false` or an empty series is skipped (`got[i].ok` stays false).
- The shift `n` is the shortest *non-empty* batch. Panes that returned nothing are left as they are.
- `a.vp.Offset += float64(n)` and `a.crossIndex += n` still run.

Concrete case: ALPHA has older history, BETA's `more` is already false, shared offset is 2. ALPHA prepends 200 bars and the offset becomes 202. ALPHA's on-screen bars stay put. BETA's bar 20, still at index 20, leaves the window. The two panes no longer share open times, and the crosshair index moves on both.

The same thing happens when a forward call returns `ok == true` and an empty slice: that pane's `more` is forced false and it is not prepended, and the offset still moves. `strictlyBefore` can produce that empty slice when the source included the anchor bar.

When every pane returns a non-empty batch, the code trims to the shortest tail (`older[len(older)-n:]`) and the counts match. That path is what `TestForwardPrependKeepsScreenIndex` covers, and it is the only path the synthetic source hits, because ALPHA and BETA share a floor and a step. A host whose symbols disagree about `more` hits the broken path on the first pan to `Offset < forwardEdge` (5).

`loading` only blocks re-entry during that call (`TestForwardDoesNotReenter`). A `GetBars` error returns before any mutation, then the next frame calls `ensureForward` again from `View`, on the UI thread, with no backoff. A slow or failing forward stalls or spins the frame.

Direction: treat the prepend as one transaction. Shift the offset only when every pane that currently has bars accepts the same count of strictly-earlier bars. If one pane cannot, leave the offset alone, keep `more` as the source reported it, and expose the error to the host. Add a test where one symbol's `more` is false and another returns a batch; the on-screen index of both symbols must stay put.

### 2. Live callbacks outlive the subscription and run on the caller's goroutine

`kline/chart.go` `followLive`, `dropLive`, `applyLiveBar`. `kline/data.go` `BarHandler`. `kline/live_stub.go` `Push`.

`followLive` registers:

```go
symbol := s.name
err := live.SubscribeBar(symbol, a.period, func(bar Candle) {
    a.applyLiveBar(symbol, bar)
})
```

The closure captures the symbol and the chart. It does not capture a generation or the period. `dropLive` calls `UnsubscribeBar` and nils `a.live`, and the closure stays callable. `applyLiveBar` appends or replaces on whatever series is current. A feed that delivers one more bar after unsubscribe — normal for an async client, and possible for `StubLiveSource` itself because `Push` copies the handler and calls it outside the mutex — writes that bar onto the new period or the new source.

`SubscribeBar` errors are discarded (`continue`). `SetPeriod` / `SetSource` still return nil when every subscription failed, or when only one symbol subscribed. There is no exported `Close`. Dropping the chart leaves the feed holding the closure.

`Chart` has no mutex. `View` reads `panes`, `vp`, and `crossIndex` on the UI thread. `applyLiveBar` writes those fields on the goroutine that calls `Push`. The README tells the host to serialize onto the UI thread. The chart cannot do that itself: it does not hold a `mygo.Window`. MyGo's `(*Window).Update` hops to the main thread and redraws; `mygo.RunOnMain` hops and waits, and does not request a frame. `applyLiveBar` never calls `Invalidate` either, so a push that does land on the UI thread changes memory and leaves an idle window stale until the next input.

`TestLiveSubscriptionFollowsPeriodAndSource` only proves the stub stops calling a handler it has deleted. It does not cover a handler invoked after `UnsubscribeBar`, a second goroutine, or a frame request.

Direction:

- Bump a `liveGen` in `dropLive`. The closure returns immediately when the generation does not match, and when `symbol` is no longer on the current source and period.
- Return subscription failures from `SetPeriod` / `SetSource`. Roll back a partial subscribe.
- Add `Close() error` that unsubscribes and fences the generation.
- Add an optional `ChartOptions` hook, `Marshal func(func())`, documented as `win.Update`. Call it around `applyLiveBar`, and have the hook request a frame. Keep the nil hook synchronous so today's tests stay valid.
- State that `BarHandler` must not re-enter `SetPeriod`, `SetSource`, or `View`.

## Important (should fix soon)

### 3. `SetPeriod` / `SetSource` footguns

`setPeriod` (`kline/chart.go`), `SetPeriod` / `SetSource` / `NewChart` (`kline/chart_opts.go`).

- `SetPeriod`'s comment says an invalid period returns `ErrInvalidPeriod` and leaves the bars in place. `setPeriod` does not call `Period.Valid`. It returns whatever `GetBars` returns. `SyntheticSource` and `StubLiveSource` do return `ErrInvalidPeriod`. A source that accepts `Period{}` will load it, store it, and subscribe live with it. A nil `src` inside `setPeriod` also returns `ErrInvalidPeriod`.
- Success always rebuilds panes from `series{name: s.name}`. Overlays are dropped. That matches the `SetPeriod` comment and `TestPeriodSwitchClearsOverlays`. It also runs when the period is already current, so the selected "1h" button in `main.go` reloads, resubscribes, and deletes drawings. `SetSource` uses the same path and its comment does not mention drawings.
- `draftOn`, `dragOn`, and `pressOn` survive. `SetLayout` and `SetTool` clear the draft. After a period change the rubber band still draws, and `Point.Index` falls back to the old `BarIndex` when the open time is absent from the new series.
- `NewChart` ignores `setPeriod`'s error (`_ = a.setPeriod(period)`). A failed first load looks like an empty chart. The comment on `NewChart` says this; there is still no `LoadError` accessor.
- On `GetBars` failure, `SetSource` restores `a.src` and does not call `followLive`, so an existing subscription stays. That part is right and covered by `TestSetSourceRejectsNilAndSwaps`. The success path still drops drawings and resubscribes with the holes in finding 2.
- `mount` keeps the `[]Candle` from `GetBars`. `setPeriod` does not copy it. `ensureForward` only copies at `prependBars`, after every symbol's `GetBars` has returned. A source that reuses one buffer across symbols corrupts the earlier batch. `StubLiveSource.cloneCandles` and `SyntheticSource` allocate fresh slices, so the in-repo sources are safe. The interface comment on `DataSource` does not say the chart retains the slice.

Direction: validate `Period` in `setPeriod` before touching the source. If `p` equals `a.period` and the source is unchanged, return nil. Copy bars at the boundary. Clear draft and drag state whenever panes are rebuilt. Document drawing loss on `SetSource`. Add `LoadError() error` (or make `NewChart` return an error) so a failed init is visible. Copy in `mount` or state in the `DataSource` comment that returned slices are owned by the chart and must not be reused.

### 4. Shortest series decides the viewport; paint uses each series' own length

`Chart.bars` (`kline/chart.go`), `Viewport.Clamp` / `IndexAt`, `Paint` (`kline/paint.go` uses `len(cs)`).

`bars` is the minimum length. `Clamp`, wheel zoom, crosshair `IndexAt`, and `FitRight` all use it. `Paint` and `drawCandles` use the pane's own length.

- One empty series makes `bars` 0. `Clamp` returns without limiting offset, so a drag can walk the other symbol without a bound. `IndexAt` fails, so the crosshair stays off on every pane.
- A live append on ALPHA only (`TestLiveBarAppendsRevisesAndRecalculates` checks that BETA's length is unchanged, and stops there) makes `bars` stay at BETA's length. `pinnedRight` calls `FitRight(bars())`, so the view does not advance. `Paint` can still draw ALPHA's extra bar in the right-hand overscroll, where BETA has no candle. The shared right edge splits by one bar until the other symbol catches up.
- Duplicate symbol names: `Bar` and `applyLiveBar` update the first match and return. The second pane never receives the push. `NewChart` does not reject duplicates.

Direction: define the rule in the API. Either require equal lengths and equal open times at load and on each live bar (drop or hold a one-sided bar, surface `ErrMisaligned`), or stop painting and hit-testing past `bars()`. Reject duplicate names in `NewChart`.

### 5. Overlay scale is right for two positive series, and wrong in the fallback

`CompareScales` / `PaintCompare` (`kline/compare.go`), `Chart.priceScale` / `paintCompare` (`kline/chart.go`).

For the normal case the P3/P9 design holds. Drawings stay in raw price. `rawScale` is the index scale multiplied by `primary[base].Close / 100`, a linear map, so a raw horizontal line, segment, and Fibonacci ratio land on the indexed candles. `CompareScales` builds that domain from the visible window (`normalizeWindow`, `windowCloses`, `windowScaled`). `TestCompareScalesMatchesFullNormalize` and `TestCompareScalesAllocationBudget` (48KB/op) lock that in. `NormalizeClose` / `NormalizeToIndex` still allocate a full series, and only the tests call them.

Two holes:

- When the base close is non-positive, `PaintCompare` falls back to `Paint` and a real `MainScale`, and returns false. `paintCompare` then draws overlays with `MainScale`. `priceScale`, which hit-testing and `pointAt` use, ignores that bool and always returns `CompareScales`' raw scale. In this fallback that scale is `chart.NewLinear(0, 1)`. New points are stored in 0..1 while the candles are raw prices, so the line misses the candle.
- `LayoutOverlay` with three or more symbols draws only `panes[0]` and `panes[1]`. Headers still run for every symbol. `compareBase` / `IndexBase` consider every pane, while `PaintCompare` does not. The header `idx` and the painted 100-base can differ. `ChartOptions.Symbols` does not say overlay is a two-series mode.

`IndexBase` also scans the whole series when the visible window has no shared positive close. That is a rare pan-time full walk, and it is the fallback the README describes.

Direction: one function should return the scale drawings and hits both use, including the non-indexed fallback. Document overlay as two symbols, or loop the compare line and use the same base as the headers. Keep the full-series helpers for tests, or stop exporting them if hosts might call them per frame.

### 6. `IndicatorEnabled` pointers die in `SyncIndicators`

`kline/chart_opts.go` `IndicatorEnabled`, `SyncIndicators`, `SetIndicators`.

The comment says the `*bool` stays valid for the life of the chart. `SetIndicator` writes the slot in place, so a pointer taken once keeps working. `SyncIndicators` replaces `a.slots` with a new slice. The old pointer still addresses the detached `indicatorSlot.on`. A `ui.Toggle` that cached it, which the comment tells the host to do, flips a bool the chart no longer reads. `main.go` happens to call `IndicatorEnabled` every frame, so the demo hides this.

`SetIndicators(nil)` and `SetIndicators([]string{})` turn every indicator off. `NewChart` with `Indicators: nil` leaves MA and MACD on, and an empty slice turns them off. Both behaviors are documented. They are still easy to get wrong, and `SetLayout` silently ignores an unknown string while `SetTool` and `SetIndicators` return errors.

`mount` runs `Calc` on every slot, including ones that are off, on each prepend and each live bar. The known cost is recalc when candles change. Recalculating a disabled BOLL or a host indicator on that path is extra. `BOLL.Calc` and `KDJ.Calc` are O(n × period) over the whole series.

Direction: store the toggle in a map or a struct that `SyncIndicators` updates in place, and fix the comment if the pointer is only valid until the next sync. Make `SetIndicators(nil)` match `NewChart`, or name the two nil meanings in the method comment in one sentence each. Skip `Calc` while a slot is off, and calc it when it is turned on if the candle generation changed.

### 7. `PriceDomain` turns one NaN into a NaN scale

`kline/viewport.go` `PriceDomain`.

`math.Min` / `math.Max` return NaN if either argument is NaN. One visible candle with a NaN low or high makes `lo` and `hi` NaN, the empty-window guard does not catch NaN (`lo > hi` is false, `IsInf` is false), and `chart.NewLinear` receives a NaN domain. Indicator code and `NormalizeClose` already skip NaN. The candle domain does not. `VolumeMax` ignores non-positive volume and returns 1, so volume is safer than price.

Direction: skip non-finite OHLC in `PriceDomain` the same way the extra-series loop skips non-finite indicator values. Add a fixture with a NaN low in the visible window and assert a finite scale.

### 8. Pan still allocates past the compare-scale fix

Not a full-series walk. `drawCandles`, `drawVolume`, `drawMA`, MACD, and KDJ all use `Viewport.Visible`. The remaining per-frame work on a pan:

- Overlay frames call `CompareScales` twice: once inside `PaintCompare`, again in `paintCompare` for `DrawOverlays`. Stack frames call `MainScale` inside `Paint` and again for overlays.
- `Layer.Items` deep-copies every overlay and its points on every paint (`DrawOverlays` in `paintPane` / `paintCompare`).
- `active` allocates a filtered slice per header and per paint. `readout` runs from `symbolHeader` every frame.
- `(*BOLL).Series` and the README's sample `band.Series` allocate a fresh `[][]float64` on every scale and draw. `(*MA).Series` returns the stored slice.
- `drawGrid` allocates a band slice and sorts it every frame.
- `subLayers` computes `SubPlot.Scale`, then MACD and KDJ `DrawSub` compute it again.

`BenchmarkPanFrames` / `BenchmarkOverlayPanFrames` measure this and do not fail on a budget. The 48KB guard is only `CompareScales`.

Direction: keep the scale computed for the frame and pass it to overlays. Draw from the layer's slice under a clip, or cache the `Items` copy until the layer changes. Give BOLL a stable series header. A small alloc budget on `BenchmarkOverlayPanFrames` would keep the next paint change honest.

## Minor / nits

- **Names.** `ToolFib` is `"fib"`. The stored kind is `OverlayFibonacci` (`"fibonacci"`). `RegisterOverlay` reserves both, plus `"hline"`, `"segment"`, and `"pan"`. `SetTool("fibonacci")` returns `ErrUnknownTool`. Hosts comparing `Tool()` to `Overlay.Kind` will miss.
- **`SetLayout`** ignores anything other than `"stack"` and `"overlay"` and returns nothing. A typo stays on the previous layout (`TestSetLayoutIgnoresUnknown` locks the silence in).
- **`SetMagnet`** accepts any `MagnetMode`. `SnapPrice` treats every value other than off and strong as weak.
- **Formatting is fixed.** `FormatPrice` is two decimals. Crosshair tags use `01-02 15:04` UTC (`drawCrosshair`). `FormatBar` uses UTC. A host cannot set precision or a location. `TimeAt` extrapolates past either end with `cs[1].Time.Sub(cs[0].Time)`, so a short first gap labels the right-hand empty region with the wrong step. Interior ticks use adjacent bars and are fine.
- **`completeStyle` / `pickColor`** treat alpha 0 as unset when merging a partial palette onto `DarkDefault`. A transparent color cannot be merged that way. A style that already sets `Plot`, `Grid`, or `Text` is kept as given, so a fully specified palette can still contain a transparent field. `SetHollow` updates `hollowUp` and leaves `style.HollowUp` stale; `chartStyle` overwrites it before paint. `SetLimitPct` does write through to `style.LimitPct`. The two switches should update the stored style the same way.
- **`applyIndicatorStyle`** returns `(*MACD, *KDJ)` and both callers ignore the result. The color writes are the useful part.
- **`Spec.Kind` is checked at `Register` and then ignored.** Layout uses `Indicator.PaneKind` on the instance. A spec registered as `PaneSub` whose `New` returns `PaneMain` follows the instance.
- **Registries are process-global and unsynchronized.** `registry` (`kline/indicator.go`) and `overlayReg` (`kline/overlay.go`). `init` is safe. A `Register` racing `Registry` or `NewChart` is a data race. There is no unregister. `host_embed_test.go` registers `HOST`, `HOSTOSC`, and `HOSTLATE` for the process lifetime; later tests see those names in `IndicatorNames`.
- **First frame vs hit-testing.** `View` handles input before `Draw` fills `painted`. Until the first frame, `plotLocal` sees an empty price rect and refuses the click. After a layout change, the click in that same frame uses the previous rect.
- **No Escape** to cancel `draftOn`. Right-click deletes the overlay under the pointer and leaves the draft up.
- **`KindBackward`** is reserved. `SyntheticSource` and `StubLiveSource` reject it. The chart never sends it. Fine, and easy for a host to miss because the request kind is a string.
- **File shape.** `Chart` is split across `chart.go` (1010 lines: forward, live, input, paint, readout) and `chart_opts.go` (435 lines: options and setters). `chart_test.go` is 1587 lines. `layout.go` repeats the axis/gap arithmetic in `splitOne`, `splitBoth`, and `splitMany`. `hitSegment` and `hitFibonacci` repeat the endpoint test. This is workable; the next feature will make `chart.go` worse.
- **Docs drift.** `docs/plans/2026-10-20-klinechart-go-next.md` still has P5 and P8 unchecked and stops at P8. The parity plan's file map still lists `paint_price.go`, `ind_vol.go`, and a fat `main`. The README's embed, extend, and performance sections match the code. The plan checkboxes do not.

## Test gaps

Covered well: viewport clamp/zoom, `Generate` / `SMA`, MACD and KDJ fixtures, BOLL symmetry and scale widening, overlay hit radius and time-stable anchors, fib levels, magnet weak/strong including a new H-line drag, period switch happy path, forward prepend when both synthetic panes match, live append/revise/ignore-old via the stub, style/limit/hollow pixels, host `Register` / `RegisterOverlay`, compare scale against a full normalize, and the 48KB `CompareScales` budget.

Missing, in the order they can hide a regression:

| Gap | What to assert |
| --- | --- |
| `ensureForward` with one pane `more == false`, or one empty batch | Offset and on-screen index of *both* symbols stay put |
| `GetBars` forward error | `more` unchanged, and the next frame does not call the source in a tight loop (backoff or sticky error) |
| Live handler invoked after `UnsubscribeBar` | Bar is not appended to the new period |
| `SubscribeBar` returns an error | `SetPeriod` / `SetSource` return it; a later `Push` does not apply |
| One-sided `Push` while the other symbol is shorter | `bars`, `FitRight`, and the rightmost painted index stay aligned |
| `SetPeriod` with the current period | Drawings and `draftOn` stay, or the docs and a test say they are cleared on purpose |
| `SetPeriod` / `SetSource` while a segment draft is up | `draftOn` is false afterward |
| `SyncIndicators` after `IndicatorEnabled` | The old `*bool` still tracks the toggle, or the comment is corrected and a test shows the refresh |
| `PriceDomain` with a NaN low in range | Finite `lo`/`hi` |
| Overlay layout, non-positive base close | Hit-test price equals the price `Paint` used |
| `NewChart` when `GetBars` fails | Host can observe the error |
| Three symbols, `LayoutOverlay` | Either the third series is drawn on the same base as the header, or `NewChart` / `SetLayout` rejects it |
| `go test -race` with `Push` off the UI thread | Fails today; should pass once finding 2 has a fence |
| Pan-frame alloc budget | `BenchmarkOverlayPanFrames` under a byte ceiling, so the double `CompareScales` and `Items` copy cannot grow quietly |

`TestForwardDoesNotReenter` only checks nested `ensureForward` during `GetBars`. `TestLiveSubscriptionFollowsPeriodAndSource` only checks a stub that has already deleted the handler.

## Recommended next PRs

1. **Forward alignment.** `ensureForward` commits a prepend only when every non-empty pane accepts the same count. Sticky error with backoff. Tests from the first row of the table above.
2. **Live fence.** Generation token, subscribe errors returned, `Close`, optional `Marshal` hook documented with `Window.Update`, and a frame request. Tests for a stale handler and a failed `SubscribeBar`.
3. **Setter contract.** `Period.Valid` inside `setPeriod`, no-op when the period and source are unchanged, copy bars on ingest, clear draft/drag, `LoadError` or a `NewChart` error, `SetSource` comment mentions drawings.
4. **Index guard.** Equal open times at init, forward, and live append. Hold or reject a one-sided bar. Reject duplicate symbols. Decide and test the three-symbol overlay case.
5. **Overlay scale fallback.** One scale value shared by `PaintCompare`, `DrawOverlays`, and `priceScale`, including the non-indexed fallback.
6. **Stable indicator toggles.** `SyncIndicators` must not detach `IndicatorEnabled` pointers. Skip `Calc` for slots that are off, and calc on enable when candles changed.
7. **NaN-safe `PriceDomain`.** Small, independent, can land with 6 or earlier.
8. **Pan allocs.** Reuse the frame's scale, stop deep-copying overlays every frame, stable `BOLL.Series` header, alloc ceiling on the overlay pan benchmark.
9. **Host formatting.** Price decimals and a `*time.Location` on `ChartOptions`, used by `FormatPrice`, `FormatBar`, and `drawCrosshair`.
10. **Split `chart.go`.** After the behavior fixes: `chart_live.go`, `chart_input.go`, `chart_load.go`. Update the two plan docs so P5–P9 checkboxes match `main`, or mark them historical so the next agent does not re-implement them.
