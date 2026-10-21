# Benchmarks

`baseline.txt` is the benchmark matrix, six samples each, with allocations.
`./bench/run.sh` writes it. The script runs:

```bash
go test ./kline -run '^$' -bench . -benchmem -count=6
```

`-run '^$'` skips tests so `-count` applies to the matrix only. `TestCompareScalesAllocationBudget` calls `testing.Benchmark`, and that call consumes the `goos` / `cpu` header Go prints once per process.

Regenerate from the repo root with `./bench/run.sh`. Pass another path to keep the committed file:

```bash
./bench/run.sh bench/new.txt
go tool benchstat bench/baseline.txt bench/new.txt
```

`go tool benchstat` runs `golang.org/x/perf/cmd/benchstat` at the version in `go.mod`. The same command without `go tool` works after `go install golang.org/x/perf/cmd/benchstat@latest`.

Compare runs from the same machine class. A delta against this file is only meaningful for the OS, architecture, and Go version below. The P9 before/after table in the repo README is historical; this file is the ongoing source of truth.

On this machine a second run of the 8k rows moved time by about 7–20% while bytes/op stayed flat. Repeat a small time delta before treating it as a change.

## This baseline

Recorded when the matrix was added.

| | |
| --- | --- |
| OS | Linux 6.12.94+ |
| Arch | amd64 |
| Go | go1.27.1 |
| CPU | Intel(R) Xeon(R) Processor (`cpu:` in `baseline.txt`) |

### Stale rows

`baseline.txt` has been appended to by later PRs without a full regeneration, so its rows do not all come from one run:

- The original matrix is from P10, before the P10+ performance fixes in #15 and #16.
- `BenchmarkMarkerPanFrames/8k` was appended by #18 and `BenchmarkAppendBar/*` and `BenchmarkReplayStepFrames/*` by #19.

The row that matters is `BenchmarkOverlayPanFrames/8k`: the file records **99 154 B/op**, but the frame stopped computing `CompareScales` twice in #16 and measures about 84 KB now. `TestOverlayPanAllocationBudget` caps it at 92 KB, so a regeneration on this OS and arch would drop that row rather than match it. Treat the overlay-pan bytes above as superseded, and check bytes/op against the current test ceilings rather than against this file.

Regenerate on Linux amd64 with go1.27.1 before citing a time delta. A Windows run produces a different `goos` / `cpu` header and cannot be compared with this file.

## Matrix

All series come from `Generate` with fixed seeds. Frame benches use a 960×720 `ui.NewTester`. Stack paints MA, MACD, BOLL, and KDJ. Overlay paints MA, MACD, and BOLL.

| Benchmark | What one iteration does |
| --- | --- |
| `BenchmarkCompareScales/{1k,8k,20k}` | Overlay price scale for the visible window. No UI. |
| `BenchmarkIndicatorCalc/{1k,8k,20k}` | `Calc` for MA, BOLL, MACD, and KDJ. No paint. |
| `BenchmarkPanFrames/stack_{1k,8k,20k}` | Pan 12 DIPs, then one stack frame. Direction alternates. |
| `BenchmarkOverlayPanFrames/{1k,8k,20k}` | Same pan on the indexed overlay plot. |
| `BenchmarkZoomFrames/{stack,overlay}_{1k,8k,20k}` | One zoom notch at the plot center, then one frame. Direction alternates. |
| `BenchmarkCrosshairFrames/{stack,overlay}_{1k,8k,20k}` | Move the pointer between two bars. Includes the follow-up frame the crosshair asks for. |
| `BenchmarkLivePushFrames/{stack,overlay}_{1k,8k,20k}` | Revise ALPHA's last bar through `StubLiveSource`, then one frame. Length stays fixed. |
| `BenchmarkMarkerPanFrames/8k` | Stack pan of 8000 bars with 500 buy/sell markers on every 16th bar. |
| `BenchmarkAppendBar/{8k,20k}` | Append one bar. MA, MACD, BOLL, and KDJ update from that close. |
| `BenchmarkReplayStepFrames/{8k,20k}` | Reveal one replay bar and paint one stack frame. |

`TestCompareScalesAllocationBudget` is a normal test, not a row in `baseline.txt`. It fails if the overlay scale copies the whole series (48 KB/op). `TestStackPanAllocationBudget` fails if a stack pan frame of 8000 bars goes above 216 KB/op; stack is the default layout and the more expensive frame. `TestOverlayPanAllocationBudget` fails if an overlay pan frame of 8000 bars goes above 92 KB/op. `TestMarkerPanAllocationBudget` fails if 500 markers add more than 16 KB/op to a stack pan frame. `TestAppendBarAmortizedAlloc` fails if appending one bar on an 8000-bar series allocates like a full indicator recalculation (8 KB/op).
