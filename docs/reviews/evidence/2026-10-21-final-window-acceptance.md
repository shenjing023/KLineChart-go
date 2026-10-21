# Final Window Acceptance

<details>
<summary>历史候选与当前工作树验收状态（2026-10-22）</summary>

当前工作树的十份二进制已于 2026-10-22 重新构建，并完成了本机 Windows 11 的启动、
标题、原生窗口句柄、响应和正常关闭验证，转录见
[final-launch.txt](2026-10-21-final-launch.txt)。随后使用新版 Computer Use
（`node_repl` + `@oai/sky`）完成了十份程序的真实窗口交互抽查。控制级结果与逐项
观察见 [final-interaction.txt](2026-10-22-final-interaction.txt)。本轮仍未穷尽每个
鼠标动作、屏幕 DPI、GPU 驱动和平台组合，因此不是跨环境全覆盖声明。

</details>

This record covers the final local worktree, not an earlier commit or the
`HEAD` report. Run it on Windows 11 with a native MyGo window. Keep the normal
`go build` portability target; this file does not claim macOS or Linux GUI
coverage.

## Build

From `D:\Tools\KLineChart-go`:

```powershell
New-Item -ItemType Directory -Force build/acceptance
go build -o build/acceptance/k-line.exe .
go build -o build/acceptance/host.exe ./examples/host
go build -o build/acceptance/history.exe ./examples/history
go build -o build/acceptance/trades.exe ./examples/trades
go build -o build/acceptance/indicator.exe ./examples/indicator
go build -o build/acceptance/paint.exe ./examples/paint
go build -o build/acceptance/live.exe ./examples/live
go build -o build/acceptance/extend.exe ./examples/extend
go build -o build/acceptance/replay.exe ./examples/replay
go build -o build/acceptance/updates.exe ./examples/updates
```

Before the matrix, run:

```powershell
gofmt -l .
go test ./... -count=1
go test ./kline -run Example -v
$env:PATH = "<MinGW-bin>;$env:PATH"
$env:CGO_ENABLED = "1"
go test ./... -race -count=1
Remove-Item Env:PATH, Env:CGO_ENABLED
```

Replace `<MinGW-bin>` with the `bin` directory of a portable MinGW toolchain.
The race command needs cgo and a C compiler. The two environment changes last
for this PowerShell session only. Close the session afterwards, or restore the
old values, and record that no machine-wide environment changed.

## Matrix

For each row, record `PASS`, `FAIL`, or `BLOCKED`, the observed state, and
whether the window exited normally. Do not treat a launched window as proof
that its controls work.

| Binary | Required interaction |
| --- | --- |
| `k-line.exe` | Change 1h/4h/1d, toggle MA/MACD/BOLL/KDJ, Stack/Overlay, drag pan, wheel zoom, crosshair readout, H-Line place/delete, Escape cancel, replay Step/Back/Play/Pause/Speed |
| `host.exe` | Move over the plot; host status changes from Last to Hover; date, price precision, and Asia/Shanghai time remain correct |
| `history.exe` | Retry the intentional first error, page to older bars, run Fail refresh, confirm existing bars remain and the error appears, Reload succeeds |
| `trades.exe` | Three buys stack on one bar, a separate sell, limit-up/down colors, Palette/Hollow/Limit controls, 1h/4h remap, Clear/Restore |
| `indicator.exe` | HOSTBAND appears in its own band, toggle off/on, Append and Revise update its line/readout, Register fast adds HOSTFAST |
| `paint.exe` | Paint/PaintCompare toggle, drag pan, wheel zoom both directions, crosshair clears over axes, Layer line visible and hit, status/readout match |
| `live.exe` | The last bar changes while running, 1h/4h/1d toolbar resubscribes, closing stops pushes |
| `extend.exe` | Toggle MID, select Mark, place/drag/delete custom diamond, right-click deletion, return to Pan |
| `replay.exe` | Step, Back, Play, Pause, index/time seek, 1h/4h/1d, Restart, Return to source; future marker stays hidden |
| `updates.exe` | Append A then B at the same open, revise A, force skewed Appends and see AlignError, Reset |

For each window, use the title bar close control. Confirm the process exits.
For `live.exe`, wait for at least two pushes before closing and confirm no
background goroutine failure is printed. `CloseMainWindow` in the launch
transcript is a normal close request to the process's main window; it is not
proof that the displayed controls were exercised.

## Record

The final ten binaries were launched from `build/acceptance` on 2026-10-22.
Every title matched, every process reported `Responding=True`, accepted
`CloseMainWindow`, and exited with code 0. The full transcript is
[final-launch.txt](2026-10-21-final-launch.txt). This proves startup, a native
window, process responsiveness, and normal close; the control-level
observations are separately recorded in
[final-interaction.txt](2026-10-22-final-interaction.txt). The host was Windows 11 build 26300 with Go
1.27.1. WMI listed only `GameViewer Virtual Display Adapter`, so this record
does not claim a physical-GPU Direct3D path.

| Binary | Result | Observed | Window exit | Notes |
| --- | --- | --- | --- | --- |
| `k-line.exe` | PASS | `1h/4h/1d` labels updated; MA/MACD toggled off/on; BOLL/KDJ and sub-panes appeared; Stack/Overlay changed the plot; drag, wheel, crosshair, H-Line placement/drag/Delete, Segment+Escape, replay Step/Play/Pause observed | Normal, exit 0 | Pause required coordinate input after an accessibility-index click was unreliable |
| `host.exe` | PASS | Host status changed `Last → Hover`; date and price precision remained visible | Normal, exit 0 | Crosshair was exercised in the plot |
| `history.exe` | PASS | Initial error visible; Retry loaded 600 bars; drag paged history; Fail refresh kept bars and showed error; Retry recovered | Normal, exit 0 | Requests count advanced from 1 to 4 |
| `trades.exe` | PASS | Buy stack, sell marker, 1h/4h remap, Clear/Restore, Palette, Hollow, Limit colors observed | Normal, exit 0 | Restored 1h markers before close |
| `indicator.exe` | PASS | HOSTBAND sub-pane toggled off/on; Append reached 121 bars; Revise changed sub-pane; Register fast added HOSTFAST | Normal, exit 0 | HOSTFAST rendered in a second sub-pane |
| `paint.exe` | PASS | Paint/PaintCompare changed indexed view; drag pan and wheel zoom in/out changed width/offset; crosshair cleared on axis; hover/readout matched | Normal, exit 0 | The status line reports a successful Layer hit; sampled pointer positions did not select the layer |
| `live.exe` | PASS | Background pushes changed bar count `243 → 247`; 4h/1d toolbar changed periods and bars | Normal, exit 0 | Normal close requested after changing 1d |
| `extend.exe` | PASS | MID toggled off; Mark selected; custom diamond placed/dragged/deleted; right-click deletion observed; returned to Pan | Normal, exit 0 | Visual pixel checks used captured native window |
| `replay.exe` | PASS | Step, Back, Seek 40, 4h, Play/Pause, 1d, Restart, Return to source observed; source ended `Source \| 1h \| 600 bars` | Normal, exit 0 | Future marker stayed outside the revealed prefix |
| `updates.exe` | PASS | Append A held, Revise held A, Append B paired, Revise A, Skew AlignError, Reset to 32 bars observed | Normal, exit 0 | Held button disabled/enabled with edge state |

Completion requires every row's interaction column to be `PASS`. The startup
and close evidence alone does not fill that column. The rows above record native
interactions observed in the current binaries; code tests and headless render
checks remain separate regression evidence.

## Evidence

For this round, the live accessibility-tree observations and screenshot-backed
Computer Use session are summarized in `final-interaction.txt`. Earlier PNGs
remain historical evidence from prior builds, not substitutes for the current
interaction record. The finalized file set is recorded in
`2026-10-22-file-cleanup.md`. If a row fails, record the exact input, visible
result, and command used to start the window.

Screenshots from an earlier build must not be reused for this record. The
window title, visible status, and final binary name are part of the evidence;
a headless frame or a process that starts without interaction is not a PASS.
