# K-line

A small candlestick chart drawn with [MyGo](https://mygo.egoist.dev) Native UI. The window is `ui.View`: Go paints it, and the process does not open a WebView or WebView2.

It is a demo of the interaction in [KLineChart](https://github.com/klinecharts/KLineChart) (candles, volume, pan, zoom, crosshair), not a port of that library. MyGo's [`ui/chart`](https://mygo.egoist.dev/docs/ui/chart) package has scales, axes, and bar/line/area marks. It has no OHLC geometry and no pan or zoom, so this repo draws the candles itself and keeps the viewport in Go. Price and time ticks come from `chart.NewLinear` and `chart.NewTime`.

## What you can do

- Two symbols, ALPHA and BETA, stacked in one window. Each symbol is price, then a volume strip, then a MACD sub-pane. They are not drawn on one price scale.
- The panes share one horizontal viewport. Dragging or scrolling either pane pans and zooms both, so the same bar indexes stay aligned.
- One crosshair. Moving the pointer in either pane selects one bar index and draws that vertical line through that symbol's price plot, volume strip, and MACD sub-pane, and through the other symbol as well. The symbol under the pointer also draws the horizontal line. Each header shows that symbol's own OHLCV, and the MA and MACD values when those indicators are on. The time axis is drawn once, under the lower symbol.
- Candles with wicks. Green when close is at or above open, red otherwise (KLineChart's default).
- MA5 and MA20 on each price plot. The **MA** toggle turns both lines off together. It starts on.
- MACD(12, 26, 9) in a sub-pane under each volume strip: DIF, DEA, and a histogram of `2*(DIF-DEA)`, on that pane's own Y scale. The **MACD** toggle starts on. Turning it off removes the sub-pane and the MACD readout.
- Volume is the built-in VOL strip between price and MACD. It is not a separate indicator and has no toggle.
- The price scale of each pane follows that symbol's visible bars.
- **Reset view** returns both panes to the latest bars at the default width.
- **1h / 4h / 1d** reload both panes from the data source and fit the latest bars. The default **1h** series is the same walk as before (ALPHA seed 42, BETA seed 99).
- Dragging to the left edge loads older bars, prepends them, and shifts the shared offset by that count so the bars on screen stay put. A load already in flight does not start another.

## Run

MyGo needs **Go 1.27.1 or later**. This module pins `github.com/egoist/mygo` v0.5.5 and the `mygo` CLI as a tool, so the CLI matches the library.

```bash
go test ./...
go run .
```

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

`go build -o kline .` also produces a binary. Packaging (`mygo build`) is optional.

### Windows, macOS, Linux

Native UI draws with Direct3D 11 on Windows, Metal on macOS, and OpenGL on Linux. A native window does not use WebView2, Edge, or WebKit.

- **Windows.** Go is enough to build and run. The WebView2 runtime is not required.
- **macOS.** Go is enough. `mygo dev` builds a development app bundle so its window position stays separate from a packaged app.
- **Linux.** Building needs no cgo. Running the window needs GTK 3, which the desktop already has on current GNOME and KDE sessions (`libgtk-3-0` on Debian and Ubuntu). WebKitGTK is only for MyGo windows that load a web page; this demo does not.

`go test` drives the view with `ui.NewTester`, which renders frames in memory and does not open a window. That is the check used in CI and in this repo's cloud build. A display is still required to look at the chart.

## Layout

- `main.go` — window, one readout per symbol, the shared viewport and crosshair index, and the period and indicator toggles.
- `kline/` — `Candle`, `Generate`, SMA, the viewport (index, bar width, pan, zoom, hit testing), `Paint` using `ui.Painter`, `DataSource` (`Period`, `BarRequest`, `SyntheticSource`), and the indicator list (`MA`, `MACD`). Volume is drawn by `Paint`, not by an indicator.

Bar `i` starts `(i - offset) * barWidth` DIPs from the left of every price plot. Dragging right decreases `offset` for all panes. The wheel keeps the bar under the pointer fixed while the width changes, and the other pane uses that same width. Each pane's Y axis is its own `chart.Linear` over that symbol's visible high/low. `Paint` takes the shared bar index so a pane draws the vertical line even when the pointer is on the other one.

## P0

本阶段在原有双窗联动图上补了数据层，没有改画布和手势模型。

能做：

- 通过 `kline.DataSource.GetBars` 取数。当前只有 `SyntheticSource`（本地合成，不访问网络）。
- `init` 取最新一段（默认 600 根）。`forward` 取锚点之前的更早 K 线，时间不重叠。`backward` 只留在接口里，合成源返回错误。
- 工具条 `1h` / `4h` / `1d`。切换时 ALPHA 和 BETA 一起重新加载，再 `FitRight`。
- 共享 `Offset < 5` 且还有更早数据时，prepend 一批并加上同样的根数，画面不跳。加载过程中不重复请求。

明确不做（也不在本阶段开始）：

- 真实行情 HTTP、WebSocket、`subscribeBar`
- 缺会话的空隙压缩
- P2 画线
- P3 同轴叠加
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
- P2 画线、P3 同轴叠加、P4 样式外置

## Out of scope

- Feature parity with KLineChart beyond the list above: drawings, a full indicator set, log scale, or session gaps.
- A WebView, or embedding the JavaScript chart.
- Live market data. A real feed implements `DataSource`; this repo does not open a socket.
- Installer polish beyond `go build` and `mygo build`.

## Limitations

- Bars are evenly spaced by index. The sample is hourly, so the time axis matches the bars. A real series with missing sessions would still be drawn one slot per bar.
- Each price scale always refits that pane's visible range. There is no locked or manual scale, and the two symbols are not overlaid on one scale.
- Zoom changes bar width between 3 and 28 DIPs. It is not KLineChart's gesture model.
- The cloud build compiles and tests the view headlessly. It does not open the window.
