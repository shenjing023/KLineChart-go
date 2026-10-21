# KLineChart-go full review (2026-10-21)

Reviewed tree: `main` at `d966ad1` (`docs: add concise AGENTS.md, move details to README (#20)`). History on that commit is P0–P10 plus PR #15–#20.

Measured on this machine (Windows 11, `go1.27.1 windows/amd64`, `CGO_ENABLED=0`):

| Check | Result |
| --- | --- |
| `go build ./...` | ok |
| `go vet ./...` | clean |
| `go test ./... -count=1` | ok — root 0.27 s, `kline` 11.9 s |
| `go test ./kline -cover` | 86.1 % of statements |
| `go test . -cover` | 80.7 % of statements (demo) |
| Test / bench functions | 168 tests (166 in `kline`, 2 in the demo), 11 benchmarks |
| Library / test size | 27 files, 6857 lines / 27 files in `kline` plus 2 root test files, 7023 + 178 lines |
| `gofmt -l` | `kline/data.go` only (see Minor 1) |
| `go test ./... -race` | not runnable here: needs cgo and a C compiler |

`bench/baseline.txt` is from Linux amd64, so its times are not comparable on this host. Bytes/op are: this host measures `BenchmarkPanFrames/stack_8k` 172 979 B/op (738 allocs), `BenchmarkOverlayPanFrames/8k` 83 997 B/op (336 allocs, ceiling 94 208 → 10.8 % headroom), `CompareScales` 14 672 B/op (ceiling 49 152), `BenchmarkAppendBar/8k` 868 B/op (1 alloc, ceiling 8 192), marker slack used 3 B of 16 384.

## Executive summary

The previous review's findings are genuinely resolved. `PriceDomain` skips non-finite OHLC, `liveGen` fences handlers, `LoadError` / `AlignError` exist, `ErrMisaligned` is enforced at load, forward, and live append, both registries are under `sync.RWMutex` with `Unregister`, `IndicatorEnabled` pointers survive `SyncIndicators`, the overlay fallback scale is shared by paint and hit test, and `chart.go` was split into `chart_load.go` / `chart_live.go` / `chart_input.go`. The invariants in `AGENTS.md` are all backed by a test that fails on the old code.

The weak part in this round is **replay as an API surface, not as a paint path**. `Replay` itself is sound: the prefix is the only thing the chart holds, `ensureForward` returns, a finer period is refused, and `Tick` matches a full `Calc` within tolerance. But the chart has no fence around the series while a replay owns it. `Chart.AppendBar` and `Chart.UpdateLast` are public and are not gated, so a host (or the demo's own toolbar path, or a host's trade recorder) can push a bar past the cursor and the replay's "no lookahead" invariant is gone. Worse, that state is not recoverable: every later `Step` returns `ErrBarTime`, `SeekIndex` returns the same error, and `Playing` stays true, so `View` keeps asking for frames that can never advance.

Two smaller lifecycle bugs share the same root cause — `Replay` state is changed before the call that can fail. `SetSource(nil)` and a failed `SetSource` both close the replay first, so a *refused* call silently ends playback. And `Replay.Close` leaves `more == false` and a partial series with `SetPeriod(same)` as a no-op, so a host that closes a replay can never get the full series back without `SetSource` or a period switch.

Beyond replay: data loading is fully synchronous on the UI thread (`View` → `ensureForward` → `GetBars`), which is the main architectural limit for a real feed; one silent feed grows `held` without a bound (200 000 bars, 27 MB, no `AlignError`); marker stacking is O(m²) and runs on every live append (20 000 markers on one bar = 180 ms); and five exported paint/query entry points have 0 % coverage, including `Paint` / `PaintCompare`, the two functions a host is told to call directly.

## Critical (must fix)

### 1. Replay's no-lookahead invariant is reachable through `AppendBar` / `UpdateLast`

`kline/append.go` `AppendBar` (line 20), `UpdateLast` (line 58). `AGENTS.md` says "Replay has no lookahead … panes hold only the revealed prefix. A hidden bar must not move paint, …" and `replay.go` enforces that by keeping `base` private and revealing one bar at a time. Neither `AppendBar` nor `UpdateLast` checks `a.replay`.

Reproduced against `main`:

- `StartReplay` at 60 bars, `SeekIndex(5)`, then `AppendBar("ALPHA", base[40])` → `Step` succeeds once and the chart holds 6 bars with `cursor == 5`. Bar 40's time is on screen while 35 bars in between are still hidden. That is exactly the leak the invariant forbids; the price scale, the volume scale, every indicator, and the readout all now see a future bar.
- The same call with a bar whose time is *behind* the corrupted edge wedges the replay. `Step(1)` returns `kline: bar time is not at the series edge: … is not after …`, and so does every later `Step` and `SeekIndex`. `StepBack` recovers only because it rebuilds from `base`. `Tick` keeps `playing == true`, so `View` calls `c.Invalidate()` forever on a replay that cannot advance.
- `UpdateLast("ALPHA", base[40])` with `Time` rewritten to the last revealed open **succeeds** and replaces the revealed close (`100.74` → `101.78`, the hidden value). No error, no way for the host to notice.

Direction: `StartReplay` is the only writer while a replay is attached. Make `AppendBar` and `UpdateLast` return `ErrReplayActive` (or route them through the replay cursor when the time is at or before it). Add the three cases above as tests, including `Playing` going false when an advance cannot happen, so a wedged replay cannot spin the frame loop.

### 2. A refused `SetSource` ends the replay; a failed one does too

`kline/chart_opts.go` `SetSource` (line 346) opens with

```go
if a != nil && a.replay != nil {
    a.replay.Close()
}
if src == nil {
    return ErrNilSource
}
```

so `SetSource(nil)` — a call that changes nothing and returns an error — detaches the replay. `StartReplay`'s contract is "Close and Pause stop playback"; a rejected argument is not a command to stop. Reproduced: `SetSource(nil) = kline: nil data source`, `chart.replay == nil`, and the next `Step` returns `ErrReplayClosed`.

The same ordering means a `SetSource` whose `GetBars` fails also ends the replay, even though its documented behavior is "A failed GetBars keeps the previous source, its bars, its drawings, and its subscription." `SetPeriod` gets this right: a refused period leaves the replay attached. Reproduced: a source that returns an error → `SetSource(down) = down`, `replay attached=false`, `bars=12`, then `Step` → `kline: replay is closed`.

Direction: validate `src` first, close the replay only after `fetchInit` succeeds, and document that a successful `SetSource` ends replay. One test per path: `SetSource(nil)` and `SetSource(failing)` must leave `Step` working.

### 3. `Close` is not sticky, and closing a replay cannot be undone

`kline/chart_opts.go` `Close` (line 292) drops the live subscription and the replay but records no state. A later `SetPeriod` goes through the normal load path and calls `followLive` again. Reproduced with `keepHandlers` on top of `StubLiveSource`: `Close()`, then `SetPeriod(4h)` → `handlers 2 -> 4`; the handler the host believed was released is subscribed again on a chart it already closed.

Separately, `Replay.Close` (`replay.go` line 281) leaves the chart on the revealed prefix. `StartReplay` cleared `more` (`replay.go` line 93) and `rebuild` keeps it false (line 447), so `ensureForward`'s `canForward` is false and paging left is dead. Reproduced at `SeekIndex(500)` of a 600-bar series: after `Close`, `more=false`, `GetBars` is never called again for a forward request, `bars=500`, and `SetPeriod(same)` returns nil without reloading, so the host has no way back to 600 bars except `SetSource` or a different period. The README's "Close does not restore the hidden suffix" documents the suffix, not the loss of `more`.

Direction: a `closed bool` on `Chart` that `SetPeriod` / `SetSource` / `followLive` respect (or document `Close` as "reopenable" and prove it with a test). On `Replay.Close`, restore `more` from the source's last report, or document that `SetPeriod`-with-the-same-period after a replay performs a load.

## Important (should fix soon)

### 4. One silent feed grows `held` without bound and never reports it

`kline/chart_live.go` `applyLiveBar` (line 107) appends every new open time to `s.held` (line 134) and waits for `promote` (line 146) to match them. The README says "A symbol whose feed stops also holds back the other symbols' new bars" — correct, and the drop path reports `ErrMisaligned`. There is no cap and no report for the *waiting* case.

Reproduced: 200 000 one-sided pushes on a 200-bar two-symbol chart → `held = 200 000`, heap +27 MB, `AlignError() == nil`, `Bars() == 200`. A long-running host chart with one dead symbol grows until the process ends, with no signal to the host other than reading `Bars()`.

Direction: cap `held` (a few hundred bars is generous) and set `AlignError` when the cap is hit, or expose the pending count so a host can decide. The chart already has the vocabulary for this: `AlignError` wraps `ErrMisaligned`, and a "held too long" case is the same class of problem.

### 5. Marker stacking is O(m²) and runs on every append and live revision

`kline/marker.go` `reindexMarks` (line 147) resets every marker's `bar`, then for each marker counts earlier markers on the same bar and side. That inner loop is quadratic in the number of markers per bar, and `mount`, `afterPromote`, `reindexMarks` after each live apply, and `SetMarkers` all call it.

Measured on this host (2000-bar pane, all markers on the same bar, the worst case for the inner loop):

| markers | `reindexMarks` |
| --- | --- |
| 1 000 | 0.5 ms |
| 5 000 | 9.4 ms |
| 20 000 | 181 ms |

And through the public append path (8000 bars, MA + MACD + BOLL + KDJ on):

| markers | per `AppendBar` |
| --- | --- |
| 500 | 105 µs |
| 5 000 | 7.7 ms |

A per-bar 181 ms stall lands on the UI thread because `applyLiveBar` runs under `Marshal` (`win.Update`), i.e. inside the frame.

Direction: assign `stack` in one pass with a `map[int]int` keyed by (bar, side), or sort by (bar, side) once. The allocation-free property that `TestMarkerPanAllocationBudget` protects is about painting, not about reindexing, so a small temporary map here is acceptable — or reuse a `map` field on `Chart`. Add a budget test the way the pan tests do it, e.g. 5 000 markers under 200 µs per append.

### 6. Every load is synchronous on the UI thread

`Chart.View` calls `ensureForward` (chart.go line 382), which calls `a.src.GetBars` in a loop over the panes (chart_load.go line 213). `NewChart`, `SetPeriod`, and `SetSource` call `fetchInit` synchronously too. `loading` guards re-entry, not duration, because the call is synchronous.

For the synthetic source this is free. For the host this package is written for — "A host implements `DataSource`" — a `GetBars` that touches disk or a network is a frame stall, and `ensureForward` runs from `View` while the user is dragging at the left edge. There is no async load path and no cancellation.

Direction: say it in the API. `DataSource`'s comment should state that `GetBars` runs on the UI goroutine inside a frame, so it must be fast or cached, and that a host with a slow source should keep the cache in `GetBars` (which is already the natural design: `StubLiveSource` and `SyntheticSource` both do). If async loading is in scope later, it is a `ChartOptions` hook plus a generation fence like `liveGen` — the same shape as `Marshal`.

### 7. Five exported entry points have no coverage

`go tool cover -func` on `kline` at 86.1 %:

| Function | Coverage | Why it matters |
| --- | --- | --- |
| `Paint` (paint.go:53) | 0.0 % | Public wrapper the README documents for a host that draws its own chrome |
| `PaintCompare` (compare.go:241) | 0.0 % | Same, overlay variant |
| `HollowEnabled` (chart_opts.go:383) | 0.0 % | Public binding for `ui.Toggle` |
| `Replay.Speed` (replay.go:244) | 0.0 % | Public accessor |
| `Layer.Clear` (overlay.go:296) | 0.0 % | Public |
| `splitBoth` (layout.go:159) | 54.3 % | The two-sub-pane geometry |
| `applyClear` (style.go:227) | 57.6 % | 28 `Clear*` bits; only `ClearGrid` and `ClearOverlay` are exercised |
| `UpdateLast` (append.go:58) | 59.1 % | Held-bar and error branches |

The internal `drawPrice` / `drawCompare` are well covered, so the untested part is the label and formatting defaults: `Paint` and `PaintCompare` are documented to stay at two decimals and UTC while the chart honours `PriceDecimals` / `Location`. That difference is exactly what a host would hit, and no test asserts it.

### 8. The stack layout has no allocation ceiling

`AGENTS.md`: "Do not raise the allocation ceilings." There are ceilings on `CompareScales` (48 KB), the overlay pan (92 KB), marker pan slack (16 KB), and an amortized append (8 KB). Stack pan — the default layout — has none, and it is the most expensive frame measured: 172 979 B/op and 738 allocs/op, about twice the overlay frame because it paints two full panes.

The README explains stack was left alone in P9 because "the stack frame is mostly MyGo layout". That is a reasonable decision, but with no ceiling a future change can double it silently. The overlay ceiling is what caught the double `CompareScales` in the previous round; the same guard on `BenchmarkPanFrames/stack_8k` would cost one constant.

### 9. Unsorted input is accepted everywhere and corrupts derived state

`DataSource` documents "Bars are in ascending open-time order", and nothing checks it. `StartReplay` validates alignment across symbols (`alignedSeries`) but not ordering. Reproduced with one swapped pair in a 20-bar series:

- `StartReplay` succeeds; the prefix is not ascending.
- `SetPeriod(4h)` succeeds and aggregates the unsorted prefix, so a 4h bucket's High/Low/Close come from the wrong bars.
- `barContaining` (`marker.go` line 175) is a binary search, so a marker for bar 10 lands on bar 15.

A host that gets the order wrong sees a wrong chart with no error. Direction: one ascending check in `fetchInit` and in `StartReplay`, returning `ErrMisaligned` (or a new error), and the same check on a live bar against the previous one (already done for the newest bar, not for a batch).

### 10. The repo has no CI, but the docs say it does

`README.md:70` — "That is the check used in CI and in this repo's cloud build." There is no `.github/`, no `.gitlab-ci.yml`, and `git log --all -- '.github/**'` is empty. `README.md:38` lists `go test ./... -race -count=1` as a normal command, but `-race` needs cgo plus a C compiler: on this Windows host it fails with `go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`, and with cgo enabled it fails again with `C compiler "gcc" not found`. The race path is therefore the one check nobody in this repo can run, and it is the check that matters most for the `liveGen` fence and `Marshal` contract.

Direction: add a workflow that runs `go build`, `go vet`, `go test -count=1`, and `go test -race -count=1` on ubuntu with cgo available, or drop the CI claim from the README. State the cgo requirement next to the `-race` line.

## Minor / nits

1. **Repository formatting is masked by CRLF.** `core.autocrlf=true` with no `.gitattributes`, so a Windows worktree is CRLF and `gofmt -l .` flags all 56 Go files. On LF copies (both the worktree and the materialized `HEAD` blobs) exactly one file is misformatted: `kline/data.go:93`, `Kind string` should be `Kind   string` to line up with `Anchor` / `Limit`. Add a `.gitattributes` with `*.go text eol=lf` and fix that one line; otherwise the real defect stays hidden in the noise.
2. **`go.sum` has a stale entry.** `go mod tidy` drops `golang.org/x/image v0.46.0` (and its `/go.mod` line) and changes nothing else. `go.mod` itself is tidy.
3. **`bench/new.txt` is not ignored.** `README.md:268` tells the reader to write `bench/new.txt`; `.gitignore` ignores `*.out` but not that path, so the documented benchmark flow leaves an untracked file. Add `bench/new.txt`.
4. **`Close` is not the last word on a replay either.** `Chart.Close` calls `a.replay.Close()` (`chart_opts.go:296`) and then `dropLive`; the replay's `Close` sets `closed` and detaches, but the chart keeps painting the prefix, and `View` still calls `Tick` — which is now a no-op. Consistent, just worth stating.
5. **Empty symbol names are accepted.** `NewChart(ChartOptions{Symbols: []string{"ALPHA", ""}})` loads, paints an unnamed header, and `SetMarkers("")` stores markers on that pane. `uniqueSymbols` only rejects duplicates. Reject an empty name the way a duplicate is rejected.
6. **Overlay with one symbol indexes to 100 against itself.** `LayoutOverlay` with `Symbols: []string{"ALPHA"}` draws a 95–115 index axis and a header ending in `idx 103.76`, because `overlayBase` accepts a single series. Defensible, but the README only describes the two-symbol case. Either document it or treat one symbol in overlay as stack.
7. **`Chart` is a broad surface.** 40 exported methods on a type with 49 fields, 97 methods total, backed by one shared viewport and one `crossIndex`. `View` also mutates state (`ensureForward`, `replay.Tick`, `fitted`, `painted`, `plotW`), so it is not idempotent; a host that paints the chart twice a frame does forward loads twice. Nothing is wrong, but the "small chart" framing in the README undersells how much the type does.
8. **`SetLayout` still accepts an unknown layout in `NewChart`.** `NewChart` silently treats `Layout: "sideways"` as stack while `SetLayout` returns `ErrUnknownLayout`. The asymmetry is documented, but the `NewChart` fallback has no test at all: `TestSetLayoutRejectsUnknown` only drives the method, and every other test passes a valid `Layout` constant.
9. **Review status table cites branch commits.** `docs/reviews/2026-10-20-full-review.md` lists `dffe2b6`, `cdd4074`, `666a448`, `da02cf7`, `4a97e97`, `4f9249d`, `7e1ecba`, `4f29092`, `76d147f`, `99161a2`, `0683e82`, `2c1f3cb` as the fixes. None of them is an ancestor of `main`; they live on `origin/cursor/minor-review-fixes-b26a`, and `main` has the squashed `95aa416`. A reader on `main` cannot `git show` any of them. Point the table at `95aa416` (or list both).
10. **A selected custom tool outlives `UnregisterOverlay` and silently swallows presses.** `SetTool("mark")`, then `UnregisterOverlay("mark")`: `Tool()` still reports `"mark"`, `pointerDown`'s `toolCustom` branch finds no spec and returns false, and no drawing or draft is created — with no error and no way for the host to notice. Reproduced. `SetTool` validates at the time of the call, which is right; the gap is that a removed spec leaves a dead selection. Either fall back to pan when the kind disappears or report it from `Tool()`.
11. **The budget tests are 92 % of the suite's wall time.** Non-budget `kline` tests: 1.0 s. The five allocation-budget tests: 11.1 s (`TestOverlayFrameSkipsItemCopy` 3.6 s, `TestMarkerPanAllocationBudget` 3.1 s, `TestAppendBarAmortizedAlloc` 1.7 s, the two pan/scale budgets 1.3 s each). That is fine in CI and painful in a tight loop (`.bench`/`-short` split or a build tag would keep both).
12. **Init window is not configurable.** `sampleBars = 600` is a const (`chart.go:13`); a host with 20 000 bars must wrap its source to return more, and `ChartOptions` has no `Limit`. `SyntheticSource.Extra` is the only knob. A `Limit int` option would be a one-line change for hosts.
13. **`Chart.View` asks for an extra frame whenever the pointer moves.** The crosshair index/pane/hover comparison (`chart.go:452`) invalidates on any 1-DIP hover change, so a moving pointer paints twice per event. Intentional (headers lag one frame) and documented in the comment; noted only because it doubles the cost of the most common interaction.
14. **`docs/plans/` are correctly marked historical**, and both file maps match the tree. No action.

## Test assessment

Strengths, in the order they matter:

- Every invariant in `AGENTS.md` has a test that fails on the old code: `TestForwardSkipsWhenOnePaneHasNoMore`, `TestForwardEmptyBatchDoesNotShift`, `TestLiveHandlerAfterUnsubscribeDoesNotAppend`, `TestSubscribeBarErrorRollsBack`, `TestOneSidedLivePushKeepsAlignment`, `TestIndicatorEnabledSurvivesSync`, `TestPriceDomainSkipsNonFinitePrices`, `TestOverlayFallbackHitMatchesPaint`, `TestReplayPanesMarkersDrawingsAndLiveFence`.
- Replay is verified against a truth model, not against itself: `TestReplayMatchesLoadedPrefix` compares a replay chart to a chart loaded with the same prefix, including every indicator line, the price domain, and the volume max.
- `TestTailAppendMatchesFullCalc` is the right shape for `BarTail`: append and revise over a generated series, compare to a full `Calc`, tolerate float noise.
- Allocation budgets are real measurements with a sink-friendly shape; I confirmed `CompareScales` reports 14 672 B/op whether the result is discarded or stored, so the compiler is not eliding it.
- Pixel assertions are used sparingly and where a color contract matters (`TestOverlayPixels`, `TestHollowToggleThinsRisingBodies`, `TestThemeChangesPlotBackground`).
- `kline/host_embed_test.go` is `package kline_test`, so the host path is exercised through the exported API only.
- Test-to-code ratio is 1.02 lines per line.

Gaps, in the order they can hide a regression:

| Gap | What to assert |
| --- | --- |
| `AppendBar` / `UpdateLast` while a replay is attached (Critical 1) | A bar past the cursor is refused, and `Playing` goes false if an advance is impossible |
| `SetSource(nil)` and a failing `SetSource` during replay (Critical 2) | `Step` still works afterwards |
| `SetPeriod` after `Close` (Critical 3) | No new `SubscribeBar` |
| `Replay.Close` then paging left (Critical 3) | Either `more` is restored or the doc says it is not |
| One silent feed for N pushes (Important 4) | `held` is bounded and `AlignError` reports it |
| 5 000 markers, one append (Important 5) | Under a byte/time ceiling |
| `Paint` / `PaintCompare` (Important 7) | Two decimals and UTC regardless of `ChartOptions.PriceDecimals` / `Location` |
| `applyClear` (Important 7) | Every `Clear*` bit clears its field |
| `splitBoth` (Important 7) | The two-sub-pane geometry for a short and a tall box |
| `UpdateLast` on a held bar, and its error branches (Important 7) | `ErrBarTime`, held revision |
| Unsorted init / replay input (Important 9) | Rejected |
| Stack pan (Important 8) | Byte ceiling |
| `go test -race` with `Marshal: nil` and a background `Push` (Important 10) | Should fail or be documented as UB; currently untested and unrunnable here |

## Documentation assessment

The README is unusually good for a demo: the "What you can do" list matches the code line by line, the embed snippet compiles (it matches `ChartOptions` field by field), the Extend section's `band` example matches `Indicator` / `LinePlot` / `LineSpec`, and the invariants section on the shared bar index is the clearest statement of the design in the repo. `AGENTS.md` is the right size and its invariants are all real.

Drift found:

- README's CI claim (Important 10) and the missing `.gitattributes` (Minor 1).
- README:28 and README:140 describe replay's fence as complete; `AppendBar` / `UpdateLast` are outside it (Critical 1). When that is fixed, both lines need the new error.
- README:140 "A hidden bar cannot move … the readout" is tested; "no lookahead" is not, because of Critical 1.
- The `docs/reviews/2026-10-20-full-review.md` Status table's commit hashes (Minor 9).
- `docs/plans/*` are correctly labelled historical; no change needed.

## Recommended next PRs

1. **Replay fence.** `ErrReplayActive` from `AppendBar` / `UpdateLast`; `Playing` false when an advance fails; tests for the three repros in Critical 1. Small, self-contained, no API removal.
2. **Replay lifecycle.** Validate before mutating in `SetSource`; `Close` sets a sticky flag; `Replay.Close` restores `more` or documents the reload rule. Tests for Critical 2 and 3.
3. **Live held cap.** Bound `held`, report through `AlignError`, test 10 000 one-sided pushes.
4. **Marker reindex.** One-pass stacking, a per-append budget test at 5 000 markers.
5. **Coverage holes.** `Paint` / `PaintCompare` formatting, all `Clear*` bits, `splitBoth`, `UpdateLast` error branches. No behavior change.
6. **Stack pan ceiling.** One constant next to `overlayPanByteCeiling`, measured on the same OS/arch/Go as the baseline.
7. **Repo hygiene.** `.gitattributes` (`*.go text eol=lf`), fix `data.go:93`, `go mod tidy`, ignore `bench/new.txt`, add `.github/workflows/ci.yml` (build, vet, test, `-race` on ubuntu) or drop the CI sentence.

## Status

Every row below is open as of `d966ad1`. When a finding is fixed, add the commit and the one-line change, following the convention in `docs/reviews/2026-10-20-full-review.md`.

| Finding | Status | Commit |
| --- | --- | --- |
| Critical 1. Replay fence around `AppendBar` / `UpdateLast` | fixed. `AppendBar` and `UpdateLast` return `ErrReplayActive` while a replay is attached and delegate to the new unexported `appendBar` / `updateLast`, which `Replay.revealOne` and `revealAggregate` call. `Replay.Tick` sets `playing = false` when `advance` fails, so `View` stops invalidating. Tests: `TestAppendBarRejectedDuringReplay`, `TestUpdateLastRejectedDuringReplay`, `TestReplayRevealsThroughPrivateAppend`, `TestReplayTickStopsWhenRevealFails`. | this PR |
| Critical 2. `SetSource` ends the replay before it can fail | fixed. `SetSource` validates `src`, the symbol list, and the period, and runs `fetchInit` before touching the replay, so `ErrNilSource`, `ErrDuplicateSymbol`, `ErrInvalidPeriod`, and a `GetBars` error all leave playback running. A successful load still closes the replay. Tests: `TestSetSourceNilKeepsReplay`, `TestSetSourceFailureKeepsReplay`, `TestSetSourceSuccessEndsReplay`. | this PR |
| Critical 3. `Close` not sticky; `Replay.Close` loses `more` | partly fixed, partly documented. `Replay` records each pane's `more` at `StartReplay` and `Replay.Close` restores it, so paging left asks the source again (`TestReplayCloseRestoresMore`). `Chart.Close` deliberately stays reopenable: a later `SetPeriod` / `SetSource` loads and resubscribes, which is the hide/show path a host needs. That is now stated in the README and pinned by `TestCloseThenSetPeriodResubscribes`. | this PR |
| Important 4. Unbounded `held` | fixed. `maxHeldBars = 512`; both `applyLiveBar` and `AppendBar` go through the new `Chart.hold`, which keeps the newest 512 pending bars, drops the oldest, and reports the drop through `AlignError` (wrapping `ErrMisaligned`). A 200 000-push one-sided feed now holds 512 bars (40 KB) instead of growing to 27 MB, and `AlignError` is set. Tests: `TestHeldIsCapped`, `TestHeldCapOnAppendBar`. | this PR |
| Important 5. O(m²) `reindexMarks` on append | fixed. The stacking pass is one walk with a `map[markKey]int` keyed by bar and side, instead of rescanning the prefix per marker. Measured on this host: 20 000 markers on one bar 181 ms → 1.5 ms; a 5 000-marker append 7.7 ms → 0.62 ms; a 500-marker append 105 µs → 52 µs. Stack values are asserted identical to the old formula, and `TestMarkerPanAllocationBudget` still passes. Tests: `TestMarkerStackingIsLinear`, `TestMarkerStackingSkipsUnloadedBars`. | this PR |
| Important 6. Synchronous loads from `View` | documented, deliberately not changed. `AGENTS.md` forbids unrequested features, and an async load path is a new API plus a generation fence. The README Limitations section now states that `GetBars` runs on the UI goroutine inside a frame and that a host with a slow source must keep its own cache, and the `DataSource` comment says the chart copies the slice so a cache may be reused. | this PR |
| Important 7. Coverage of `Paint` / `PaintCompare` / `Clear*` / `splitBoth` | fixed. `Paint`, `PaintCompare`, `HollowEnabled`, `Replay.Speed`, `Layer.Clear`, and `applyClear` are at 100 %, and `UpdateLast` is at 100 % with `updateLast` at 95.5 %. `splitBoth` moved from 54.3 % to 87.0 % by exercising the collapsed-box fallback. Package coverage 86.1 % → 88.1 %. Tests: `TestPaintUsesDefaultLabels`, `TestPaintRendersPanes`, `TestHollowEnabledBinding`, `TestReplaySpeedAndLayerClear`, `TestClearBitsClearTheirFields` (all 28 bits, and a check that no bit clears a neighbour), `TestSplitBothKeepsEveryBandPositive`, `TestSplitBothNarrowBoxShrinksAxis`, `TestUpdateLastHeldBar`, `TestUpdateLastEmptySeries`. | this PR |
| Important 8. Stack pan allocation ceiling | fixed. Added `TestStackPanAllocationBudget` with `stackPanByteCeiling = 216 * 1024`. This host measures 173 KB/op, the committed Linux baseline 174 493 B/op, and a cold first sample about 193 KB, so the ceiling keeps room for the first sample without allowing a second copy of the per-frame work. `AGENTS.md` and the README now carry the new ceiling. | this PR |
| Important 9. Unsorted input accepted | fixed. New `ErrBarOrder`; `checkBarOrder` runs in `fetchInit`, in `StartReplay`, and on a forward batch, where an out-of-order batch is dropped, reported through `AlignError`, and held off by the existing backoff. A duplicate open time counts as out of order. A refused load keeps the bars already on screen. Tests: `TestLoadRejectsUnsortedBars`, `TestLoadRejectsDuplicateOpenTime`, `TestStartReplayRejectsUnsortedBars`, `TestLiveBarOrderIsStillChecked`. | this PR |
| Important 10. No CI, `-race` undocumented as cgo-only | fixed. Added `.github/workflows/ci.yml` (ubuntu, `setup-go` from `go.mod`): build, vet, `gofmt -l`, `go test`, and `go test -race` with `CGO_ENABLED=1`. The README now says `-race` needs cgo and a C compiler, and the CI sentence names the workflow instead of claiming an unnamed "cloud build". | this PR |
| Minor. `bench/baseline.txt` is a mix of three runs | documented. `bench/README.md` gains a "Stale rows" section: the matrix is from P10, `BenchmarkMarkerPanFrames/8k` from #18, and `BenchmarkAppendBar/*` and `BenchmarkReplayStepFrames/*` from #19. The file's `BenchmarkOverlayPanFrames/8k` row (99 154 B/op) is above the 92 KB ceiling the code now enforces, so that row is superseded. Regeneration needs Linux amd64 + go1.27.1; a Windows run writes a different header and is not comparable. | this PR |
| Minor. CRLF masking gofmt | fixed. Added `.gitattributes` (`*.go`, `*.md`, `*.sh`, `*.txt`, `*.json` → `text eol=lf`) and corrected the one real defect, `kline/data.go` `BarRequest.Kind` alignment. `gofmt -l` is now meaningful on a Windows worktree. | this PR |
| Minor. Stale `go.sum` entry | fixed. `go mod tidy` dropped `golang.org/x/image v0.46.0`; `go.mod` was already tidy. | this PR |
| Minor. `bench/new.txt` untracked | fixed. `.gitignore` now lists `bench/new.txt`, the path the README benchmarking flow writes. | this PR |

## How this review was verified

`go build ./...`, `go vet ./...`, `go test ./... -count=1`, `go test ./kline -cover`, `go test . -cover`, `go test ./kline -run '^$' -bench …` on this host, plus `gofmt -l` on LF copies of both the worktree and the `HEAD` blobs, `go mod tidy` in a scratch copy, and `git merge-base --is-ancestor` for every commit hash cited in the previous review.

Behavior claims were checked with throwaway tests in `kline/` (replay + `AppendBar`, replay + `UpdateLast`, `SetSource(nil)` / failing `SetSource` during replay, `SetPeriod` after `Close`, `Close` + `SetPeriod` resubscribe, `Replay.Close` + forward load, 200 000 one-sided live pushes, marker reindex timing, `Paint`/`PaintCompare` budget sanity, tiny frame sizes, unsorted input, nil series, partial style). All throwaway files were deleted; `git status` is clean and the suite still passes.
