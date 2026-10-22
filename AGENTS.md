# Agent notes

Overview, embed API, demo, and the full command list are in [README.md](README.md). Plans and the review status table are under [docs/](docs/).

## Commands

From the repo root (Go 1.27.1, module pin `github.com/egoist/mygo` v0.6.2):

- `go build -o k-line .` (`-o kline` writes inside the `kline/` package directory)
- `go test ./... -count=1`
- `go test ./... -race -count=1`
- `go run ./examples/live` (also `host`, `history`, `extend`, `indicator`, `trades`, `paint`, `updates`, and `replay`; each has a headless test; only `extend` and `replay` take `-headless`)
- The final desktop acceptance matrix lives in `docs/reviews/evidence/2026-10-21-final-window-acceptance.md`; a launch, `Responding` check, or headless tester is not an interaction PASS.
- `go test ./kline -run Example -v` runs the API examples and compares their output
- Bench check, without replacing the baseline: `go test ./kline -run '^$' -bench . -benchmem -count=6 > bench/new.txt` then `go tool benchstat bench/baseline.txt bench/new.txt`

`go test` already runs the allocation budgets. Do not pass `-bench` to that command.

## Invariants

- Native paint only. The window is `ui.View` and `ui.Painter`. Do not add `ui.WebView`, HTML, or WebView2. This repo does not open a market-data connection.
- A-share by default: rising candles and buy markers are red, falling candles and sell markers are green. `NewChart` defaults to `600519` / `000001` and Asia/Shanghai labels. `SyntheticSource` uses weekday 09:30-11:30 and 13:00-15:00; it does not invent exchange holidays. Its walk is generated backwards from the newest bar, so a bar keeps its OHLCV across windows. A period with no session bars (`3h`, `150m`) returns `ErrInvalidPeriod`; `Period.Duration` refuses a span that overflows `time.Duration`. `UnitWeek` is `Valid` but not `Loadable`: only a replay builds it, by aggregating the revealed days; `GetBars` and a plain `SetPeriod` refuse it.
- Replay aggregate opens: hourly and minute buckets truncate to the epoch; a daily bucket opens on the first revealed bar of its local calendar day (`ChartOptions.Location`), so a replay and a plain daily load name the same instant. `bucketOpen` is the only place that decides this.
- Replay has no lookahead. `StartReplay` keeps the full series off the chart; panes hold only the revealed prefix. A hidden bar must not move paint, the price or volume scale, indicators, the crosshair, the readout, or overlay compare. `ensureForward` returns while a replay is attached. A finer `SetPeriod` returns `ErrReplayPeriod` and does not call `GetBars`.
- A replay owns the series. `AppendBar` and `UpdateLast` return `ErrReplayActive` while `chart.replay` is set; `Replay` reveals through the unexported `appendBar` / `updateLast`. Do not call the public pair from inside `replay.go`. `Replay.Tick` stops playback when a reveal fails, so `View` stops asking for frames.
- One shared viewport. Bar `i` is the same open time on every pane. `ensureForward` prepends and shifts `Offset` only when every pane that already has bars accepts the same count. Otherwise the offset stays and each pane keeps the `more` flag the source reported.
- Live fence. Handlers capture `liveGen`. `dropLive` and `Chart.Close` bump that generation and unsubscribe. A handler that runs after `Close` must not change the chart. A `SubscribeBar` error unsubscribes the symbols that already succeeded. The handler must not call `SetPeriod`, `SetSource`, or `View`.
- Do not raise the allocation ceilings. `go test` runs `TestCompareScalesAllocationBudget`, `TestStackPanAllocationBudget`, `TestOverlayPanAllocationBudget`, `TestMarkerPanAllocationBudget`, and `TestAppendBarAmortizedAlloc`. The byte limits are in those tests. `bench/baseline.txt` predates the #15/#16 fixes; check bytes/op against the test ceilings, not against that file.
- `PriceDomain` skips non-finite OHLC and extra series. `VolumeMax` skips non-finite volume the same way. A NaN or infinity must not become the axis domain.
- Sub-pane bands come from one order. `subBands` is the only place that decides which `*MACD`, `*KDJ`, or host sub-pane takes which band; layout, paint, and the theme colors all read it. Do not add a second used/extra counter.
- A `*Chart` receiver is not nil-safe. Only `Close` accepts nil. Keep the doc comment on `Chart` in step with that.

## Conventions

- One PR per feature or fix. Subjects match `git log`: `feat:`, `fix:`, `docs:`, or `P10:`.
- A behavior change needs a test that fails on the old code.
- Replace `bench/baseline.txt` only when `benchstat` on the same OS, arch, and Go is neutral or better. Time can move 7–20% between runs; bytes/op should stay flat.
- Keep the exported API stable. Describe a behavior change in the README.
- Never move a tag that has been pushed. A force-updated tag breaks `go.sum` for every downstream module and reads as "same version, different code". A fix or an addition after a tag gets a new patch tag (`v0.1.0` → `v0.1.1`); that is one `go.mod` line for a host, versus a checksum mismatch it has to debug. An unpublished local tag may be moved while it is still local only.
- A doc-only change still needs a tag when the doc is something a host reads. A host pinned to a tag sees `go doc` output from that tag, not from `main`, so a corrected option comment or a README paragraph that changes how a host should call the API has not reached anyone until it is tagged. Tag it as a patch release and say in the message that only documentation changed. A typo in an internal comment does not need one.
- Do not add a feature that was not asked for. Perf and host DX are the standing work. `docs/plans/` is a record, not a queue.
