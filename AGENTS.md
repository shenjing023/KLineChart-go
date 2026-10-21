# Agent notes

Overview, embed API, demo, and the full command list are in [README.md](README.md). Plans and the review status table are under [docs/](docs/).

## Commands

From the repo root (Go 1.27.1, module pin `github.com/egoist/mygo` v0.5.5):

- `go build -o k-line .` (`-o kline` writes inside the `kline/` package directory)
- `go test ./... -count=1`
- `go test ./... -race -count=1`
- `go run ./examples/live` (also `extend` and `replay`; each opens a window or prints, and each has a headless test)
- `go test ./kline -run Example -v` runs the API examples and compares their output
- Bench check, without replacing the baseline: `go test ./kline -run '^$' -bench . -benchmem -count=6 > bench/new.txt` then `go tool benchstat bench/baseline.txt bench/new.txt`

`go test` already runs the allocation budgets. Do not pass `-bench` to that command.

## Invariants

- Native paint only. The window is `ui.View` and `ui.Painter`. Do not add `ui.WebView`, HTML, or WebView2. This repo does not open a market-data connection.
- A-share by default: rising candles and buy markers are red, falling candles and sell markers are green. `NewChart` defaults to `600519` / `000001` and Asia/Shanghai labels. `SyntheticSource` uses weekday 09:30-11:30 and 13:00-15:00; it does not invent exchange holidays.
- Replay has no lookahead. `StartReplay` keeps the full series off the chart; panes hold only the revealed prefix. A hidden bar must not move paint, the price or volume scale, indicators, the crosshair, the readout, or overlay compare. `ensureForward` returns while a replay is attached. A finer `SetPeriod` returns `ErrReplayPeriod` and does not call `GetBars`.
- A replay owns the series. `AppendBar` and `UpdateLast` return `ErrReplayActive` while `chart.replay` is set; `Replay` reveals through the unexported `appendBar` / `updateLast`. Do not call the public pair from inside `replay.go`. `Replay.Tick` stops playback when a reveal fails, so `View` stops asking for frames.
- One shared viewport. Bar `i` is the same open time on every pane. `ensureForward` prepends and shifts `Offset` only when every pane that already has bars accepts the same count. Otherwise the offset stays and each pane keeps the `more` flag the source reported.
- Live fence. Handlers capture `liveGen`. `dropLive` and `Chart.Close` bump that generation and unsubscribe. A handler that runs after `Close` must not change the chart. A `SubscribeBar` error unsubscribes the symbols that already succeeded. The handler must not call `SetPeriod`, `SetSource`, or `View`.
- Do not raise the allocation ceilings. `go test` runs `TestCompareScalesAllocationBudget`, `TestStackPanAllocationBudget`, `TestOverlayPanAllocationBudget`, `TestMarkerPanAllocationBudget`, and `TestAppendBarAmortizedAlloc`. The byte limits are in those tests. `bench/baseline.txt` predates the #15/#16 fixes; check bytes/op against the test ceilings, not against that file.
- `PriceDomain` skips non-finite OHLC and extra series. A NaN must not become the axis domain.

## Conventions

- One PR per feature or fix. Subjects match `git log`: `feat:`, `fix:`, `docs:`, or `P10:`.
- A behavior change needs a test that fails on the old code.
- Replace `bench/baseline.txt` only when `benchstat` on the same OS, arch, and Go is neutral or better. Time can move 7–20% between runs; bytes/op should stay flat.
- Keep the exported API stable. Describe a behavior change in the README.
- Do not add a feature that was not asked for. Perf and host DX are the standing work. `docs/plans/` is a record, not a queue.
