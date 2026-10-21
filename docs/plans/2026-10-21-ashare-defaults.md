# A-share defaults

## Goal

Make the default chart, built-in style, synthetic source, demo, and examples
present mainland China A-share conventions. Keep `Generate` as a generic
deterministic test fixture; add a separate A-share fixture instead of changing
test geometry used across the package.

## Requirements

- Rising candles and buy markers are red; falling candles and sell markers are
  green. Limit-up is red and limit-down is green. The `US` toolbar preset swaps
  the pair.
- A chart with no explicit location formats dates in Asia/Shanghai. Explicit
  locations, including UTC, still win.
- A chart with no symbols uses `600519` and `000001`.
- `SyntheticSource` generates weekday A-share sessions in Asia/Shanghai:
  09:30-11:30 and 13:00-15:00. One-hour bars open at 09:30, 10:30, 13:00, and
  14:00. Longer 4h/1d bars use one 09:30 open per weekday. Exchange holidays
  are not guessed because no authoritative holiday calendar is bundled.
- The root demo and all runnable examples use A-share symbols or A-share
  session data. No example calls the generic `Generate` fixture for its visible
  market data.
- Existing `Generate`, `sampleWalk`, replay no-lookahead, allocation budgets,
  and exported APIs remain compatible.

## Verification

1. Focused tests for red/green defaults, default symbols/location, session
   boundaries, lunch break, weekends, forward paging, and examples.
2. `gofmt -l .`, `go build ./...`, `go vet ./...`, `go test ./... -count=1`,
   API examples, allocation budgets, and Windows race.
3. Launch the real Windows demo and `examples/live`; capture screenshots to
   confirm red-up/green-down candles, A-share codes, Asia/Shanghai labels, and
   session gaps.

## Result

- `StyleFrom`, `DarkDefault`, and `LightDefault` now use red-up / green-down.
  The Classic toolbar preset matches; US swaps it.
- `NewChart` defaults to `600519`, `000001`, and Asia/Shanghai. The explicit
  `Location` option still wins.
- `SyntheticSource` uses weekday A-share sessions. Hour opens are 09:30, 10:30,
  13:00, and 14:00; 4h and 1d use 09:30 per weekday. Generic `Generate` and
  `sampleWalk` remain available for non-calendar tests.
- Root demo, API examples, `examples/live`, `examples/replay`, and
  `examples/extend` now use A-share symbols or session data.
- Full tests, API examples, Windows race, and the real Windows demo passed.
  The real 4h and 1d windows showed `Last ... 09:30` under Asia/Shanghai.

## Rollback

The style, source, and example changes are separable by file. Do not modify or
replace `bench/baseline.txt`.
