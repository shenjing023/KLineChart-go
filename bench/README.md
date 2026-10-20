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

`TestCompareScalesAllocationBudget` is a normal test, not a row in `baseline.txt`. It fails if the overlay scale copies the whole series (48 KB/op).
