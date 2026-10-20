# K-line

A small candlestick chart drawn with [MyGo](https://mygo.egoist.dev) Native UI. The window is `ui.View`: Go paints it, and the process does not open a WebView or WebView2.

It is a demo of the interaction in [KLineChart](https://github.com/klinecharts/KLineChart) (candles, volume, pan, zoom, crosshair), not a port of that library. MyGo's [`ui/chart`](https://mygo.egoist.dev/docs/ui/chart) package has scales, axes, and bar/line/area marks. It has no OHLC geometry and no pan or zoom, so this repo draws the candles itself and keeps the viewport in Go. Price and time ticks come from `chart.NewLinear` and `chart.NewTime`.

## What you can do

- **Stack / Overlay.** Stack is the default: ALPHA and BETA each get a price plot, a volume strip, and a MACD sub-pane, on their own price scales. Turning **KDJ** on adds a KDJ sub-pane under MACD. Overlay puts both symbols on one price plot.
- In Overlay, ALPHA stays candles and BETA is a close line. Both are divided by their own close at the first visible bar where both closes are positive, so that bar is 100 on the shared Y axis. Panning or zooming moves that base with the window. The header shows each symbol's own OHLCV at the crosshair index, plus that index (`idx`).
- Overlay keeps one volume strip and the open sub-panes (MACD, and KDJ when that toggle is on), all for ALPHA. BETA's volume, MACD, and KDJ are not drawn. BETA's header is the bar's OHLCV and index, not BETA's MA, BOLL, MACD, or KDJ.
- The panes share one horizontal viewport in both layouts. Dragging or scrolling pans and zooms that viewport, so the same bar indexes stay aligned.
- One crosshair. In Stack, moving the pointer in either pane selects one bar index and draws that vertical line through that symbol's price plot, volume strip, and open sub-panes, and through the other symbol as well. The symbol under the pointer also draws the horizontal line. Each header shows that symbol's own OHLCV, and the MA, BOLL, MACD, and KDJ values when those indicators are on. The time axis is drawn once, under the lower symbol. In Overlay the same index drives both headers, and the vertical line runs through the one price plot, the ALPHA volume strip, and ALPHA's open sub-panes.
- Candles with wicks. Green when close is at or above open, red otherwise (KLineChart's default). **Hollow** draws a rising body as an outline and keeps a falling body filled. Off, both bodies are filled. Volume bars and the MACD histogram stay filled either way.
- MA5 and MA20 on each price plot. The **MA** toggle turns both lines off together. It starts on.
- MACD(12, 26, 9) in a sub-pane under each volume strip: DIF, DEA, and a histogram of `2*(DIF-DEA)`, on that pane's own Y scale. The **MACD** toggle starts on. Turning it off removes the sub-pane and the MACD readout.
- **BOLL(20, 2)** on the price plot: the middle line is SMA20, and the upper and lower lines are that average ± 2 population standard deviations. The **BOLL** toggle starts off. The crosshair readout shows `MID`, `UP`, and `LOW`.
- **KDJ(9, 3, 3)** in its own sub-pane under MACD (or under volume when MACD is off). The Y scale covers 0–100 and widens when K, D, or J leaves that range. The **KDJ** toggle starts off. The readout shows `K`, `D`, and `J`.
- Volume is the built-in VOL strip between price and the sub-panes. It is not a separate indicator and has no toggle.
- Colors come from `kline.Style`, built from the window theme by `StyleFrom`. Light and dark each get their own plot, grid, crosshair, and MA / MACD colors. Rising stays green and falling stays red on both. `DarkDefault` and `LightDefault` are those two styles.
- In Stack, the price scale of each pane follows that symbol's visible bars, including BOLL when that toggle is on. In Overlay, the one price scale follows the indexed range of both series, including ALPHA's moving averages and Bollinger bands when those toggles are on.
- **Reset view** returns both panes to the latest bars at the default width.
- **1h / 4h / 1d** reload both panes from the data source and fit the latest bars. The default **1h** series is the same walk as before (ALPHA seed 42, BETA seed 99).
- Dragging to the left edge loads older bars, prepends them, and shifts the shared offset by that count so the bars on screen stay put. A load already in flight does not start another.
- **Pan / H-Line / Segment.** Pan drags the chart. H-Line places a horizontal price line with one click; dragging it changes the price. Segment takes two clicks, with a rubber band between them. A draw tool turns pan off. The selected line is highlighted. Delete removes it, and a right-click removes the line under the pointer. Drawings belong to the pane that was clicked. In Overlay, new drawings are stored on ALPHA in raw price, and BETA's existing lines stay hidden until you return to Stack.

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

- `main.go` — window, one readout per symbol, the shared viewport and crosshair index, and the period, layout, and indicator toggles.
- `kline/` — `Candle`, `Generate`, SMA, the viewport (index, bar width, pan, zoom, hit testing), `Paint` using `ui.Painter`, `Style` (`style.go`: `StyleFrom`, `DarkDefault`, `LightDefault`), `DataSource` (`Period`, `BarRequest`, `SyntheticSource`), optional `LiveSource` and the in-memory `StubLiveSource`, the indicator list (`MA`, `MACD`, `BOLL`, `KDJ`), overlays (`Layer`, horizontal price line, segment), and overlay compare (`NormalizeClose`, `IndexBase`, `PaintCompare`). Volume is drawn by `Paint`, not by an indicator. Each symbol keeps its own `Layer`.

Bar `i` starts `(i - offset) * barWidth` DIPs from the left of every price plot. Dragging right decreases `offset` for all panes. The wheel keeps the bar under the pointer fixed while the width changes, and the other pane uses that same width. In Stack, each pane's Y axis is its own `chart.Linear` over that symbol's visible high/low. In Overlay, both series share one index scale: `close[i] / close[base] * 100`, and ALPHA's OHLC uses that same divisor. `Paint` takes the shared bar index so a pane draws the vertical line even when the pointer is on the other one.

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
- `kline.Layer` 是**一个 pane 自己的线**。ALPHA 上的线不会出现在 BETA 上，反过来也一样。两个 pane 各有各的选中。
- **H-Line**：在价格区点一下放下一条水平价线。拖动改价格。选中时加粗，并在左端画一个方块。
- **Segment**：点两下。第一下之后，指针和第一点之间有一条橡皮筋；第二下完成。选中时两端有方块。在 Segment 工具下拖端点，可以改那一端的 bar 和价格。
- 工具条 **Pan / H-Line / Segment**。选了画线工具时，拖动不再平移。滚轮缩放仍然可用。切回 Pan 之后，拖动重新平移；在线上点一下（没有拖开）会选中它。
- **Delete** 删掉当前 pane 里选中的那条。**右键**删掉指针下面的那条，只影响这个 pane。
- 切换周期会重新加载 K 线，两个 pane 上的线一起清掉。

明确不做：

- 斐波那契、笔刷，以及磁吸的弱档 / 强档
- 一条线同时画在两个 pane 上
- P3 同轴叠加（见下文）、P4 样式外置

## P3

本阶段加了同轴比较，没有改 Stack 的画法和手势。默认仍是 Stack。

能做：

- 工具条 **Stack / Overlay**。Stack 是原来的上下两栏：各自的价格、量柱、MACD。Overlay 收成一栏。
- Overlay 里主序列（ALPHA）仍是蜡烛。第二序列（BETA）画收盘折线，不用第二套蜡烛。两套蜡烛叠在一起时实体会互相挡住，折线能看清相对走势。
- 归一化是 `close[i] / close[base] * 100`。`base` 是当前可见区间里，两列收盘都为正的第一根；可见区间里没有这样的 bar 时，退到整段序列里的第一根。ALPHA 的开高低收都除以 ALPHA 在 `base` 的收盘，所以这根蜡烛的收盘是 100。BETA 除以 BETA 自己在同一根的收盘。Y 轴跟着两条归一化序列（以及打开时的 ALPHA 均线）的范围走。平移或缩放会改 `base`，两条都从新的左端重新回到 100。
- 视窗和十字线仍然共用。两个抬头都显示这一根的 OHLCV，并带上 `idx`（归一化后的收盘）。ALPHA 的抬头继续带 MA 和 MACD。BETA 的抬头不带它自己的 MA / MACD。
- 量柱和 MACD 只留 ALPHA 的一条。BETA 的量柱和 MACD 不画，避免一栏里再塞第二套副图。
- 画线仍按原始价格存在 ALPHA 的 `Layer` 上。切到 Overlay 时用同一条价格到像素的映射，线还贴在 ALPHA 的蜡烛上。BETA 上已有的线先藏起来，回到 Stack 再出现。

明确不做：

- 双价格轴、对数轴、锁定 Y
- BETA 的量柱或 MACD 副图
- 两条都画成蜡烛
- P4 样式外置、明暗主题、空心阳线

## P4

本阶段把颜色收到 `kline.Style`，并加了空心阳线。画线、指标算法、Stack / Overlay 的布局都没有改。

能做：

- `kline.Style` 在 `kline/style.go`。里面有涨跌色、网格、十字线、MA5 / MA20，以及 MACD 的 DIF、DEA 和零轴。柱状图（量柱和 MACD 柱）用同一套涨跌色。
- `StyleFrom(theme)` 按当前 MyGo 主题填这些颜色。`DarkDefault` 和 `LightDefault` 分别是暗色和亮色主题的结果。传入 nil 等于 `DarkDefault`。
- 明暗两套的绘图区、网格、十字线、坐标字，以及 MA / MACD 线，跟着主题的背景、边框、弱化文字、Accent、Warning 走。涨跌色两套都是绿涨红跌，不随明暗对调。
- 工具条 **Hollow**。打开后，收盘不低于开盘的 K 线只画框，收盘更低的仍是实心。关掉后两种实体都填满。Overlay 里的 ALPHA 蜡烛走同一条绘制，所以这个开关在两种布局里都生效。
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
- `kline.StubLiveSource` 把历史和订阅都放在内存里。`SetBars` 写入历史，`Push` 在已订阅时调用 handler，`UnsubscribeBar` 之后不再投递。测试用它，不需要网络。

真实源由宿主实现，不在本仓库里。历史实现 `GetBars`；能推送时再实现 `LiveSource`。`Push` 的调用方决定在哪一个 goroutine 上进入 handler，stub 是同步调用。宿主若从别的 goroutine 推送，要自己串到 UI 线程。下面是形状，不是可运行的客户端：

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
- 工具条 **BOLL**。默认关。打开后十字线读数带上 `MID` / `UP` / `LOW`。价格轴把上下轨算进去，带不会被裁掉。Overlay 里这三条跟着 ALPHA 的归一化价格走。
- **KDJ(9, 3, 3)** 单独占一条副图。RSV 用 9 根里的最高价和最低价；最高价等于最低价时分母按 1。K、D 的递推是 `((周期−1)×前值 + 当日值) / 周期`，没有前值时用 50。通常参数下就是 `(2×前值 + 当日值) / 3`。J = `3K − 2D`。和 KLineChart 的算法一致。
- 副图 Y 轴通常覆盖 0–100。K、D 或 J 超出这个范围时轴跟着拉开。
- 工具条 **KDJ**。默认关。Stack 里每个 symbol 自上而下是：价格、量柱、MACD（开着时）、KDJ（开着时）。只开 KDJ 时，它紧挨在量柱下面。Overlay 里副图仍只画 ALPHA 的。关掉后这一行消失，时间轴回到上面一条。
- 十字线的竖线穿过 KDJ。指针停在 KDJ 上时，右侧标出该副图的 Y 值。两个 symbol 的抬头都显示这一根的 `K` / `D` / `J`。

明确不做：

- RSI、SAR，以及其余指标
- 自定义脚本指标
- P7 磁吸和斐波那契

## Out of scope

- Feature parity with KLineChart beyond the list above: the rest of the overlay set, a full indicator set, log scale, or session gaps.
- A WebView, or embedding the JavaScript chart.
- A market-data client. A host implements `DataSource` and, when it can push bars, `LiveSource`. This repo only defines that interface and an in-memory `StubLiveSource`. It does not open an HTTP or WebSocket connection.
- Installer polish beyond `go build` and `mygo build`.

## Limitations

- Bars are evenly spaced by index. The sample is hourly, so the time axis matches the bars. A real series with missing sessions would still be drawn one slot per bar.
- Each price scale refits the visible range. Stack gives each symbol its own scale. Overlay indexes both to 100 at the first visible common bar and refits that index range. There is no locked or manual scale.
- Zoom changes bar width between 3 and 28 DIPs. It is not KLineChart's gesture model.
- The cloud build compiles and tests the view headlessly. It does not open the window.
