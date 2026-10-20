# K-line

A small candlestick chart drawn with [MyGo](https://mygo.egoist.dev) Native UI. The window is `ui.View`: Go paints it, and the process does not open a WebView or WebView2.

It is a demo of the interaction in [KLineChart](https://github.com/klinecharts/KLineChart) (candles, volume, pan, zoom, crosshair), not a port of that library. MyGo's [`ui/chart`](https://mygo.egoist.dev/docs/ui/chart) package has scales, axes, and bar/line/area marks. It has no OHLC geometry and no pan or zoom, so this repo draws the candles itself and keeps the viewport in Go. Price and time ticks come from `chart.NewLinear` and `chart.NewTime`.

## What you can do

- Candles with wicks. Green when close is at or above open, red otherwise (KLineChart's default).
- Volume bars in a pane under the candles, colored the same way.
- MA5 and MA20 over the closes.
- Drag horizontally to pan. The wheel zooms the bar width around the pointer. A sideways scroll pans.
- A crosshair and a readout of the hovered bar: time (UTC), open, high, low, close, volume, and the moving averages when they exist. Away from the plot, the readout stays on the latest bar.
- The price scale follows the visible bars. Grid lines and tick labels track that range.
- **Reset view** returns to the latest bars at the default width.
- 600 hourly bars from a fixed random walk, so the app runs with no network and no API key.

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

- `main.go` — window, header readout, drag, wheel, and the first-frame fit.
- `kline/` — `Candle`, the synthetic series, SMA, the viewport (index, bar width, pan, zoom, hit testing), and `Paint` using `ui.Painter`.

Bar `i` starts `(i - offset) * barWidth` DIPs from the left of the price plot. Dragging right decreases `offset`. The wheel keeps the bar under the pointer fixed while the width changes. The Y axis is `chart.Linear` over the visible high/low, padded, including MA points that fall in that window.

## Out of scope

- Feature parity with KLineChart: drawings, a full indicator set, log scale, sessions, or a data loader.
- A WebView, or embedding the JavaScript chart.
- Live market data.
- Installer polish beyond `go build` and `mygo build`.

## Limitations

- Bars are evenly spaced by index. The sample is hourly, so the time axis matches the bars. A real series with missing sessions would still be drawn one slot per bar.
- The price scale always refits the visible range. There is no locked or manual scale.
- Zoom changes bar width between 3 and 28 DIPs. It is not KLineChart's gesture model.
- The cloud build compiles and tests the view headlessly. It does not open the window.
