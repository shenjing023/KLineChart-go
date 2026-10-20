# KLineChart-go 能力补齐 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在保持 MyGo Native `ui.View`（无 WebView）的前提下，按打谱/交易优先级，把 [KLineChart-go](https://github.com/shenjing023/KLineChart-go) 从「看图骨架」补到可用的数据加载、副图指标、画线、同轴叠加与基础样式层。

**Architecture:** 以现有 `kline.Viewport` + 多 pane 共享十字线为核；新增可替换的 `DataSource`（先本地合成，接口对齐 KLineChart DataLoader 语义）；指标与画线作为独立层挂到 pane，不塞进 `Paint` 巨石函数；每一阶段合入 `main` 后可独立演示与测试。

**Tech Stack:** Go 1.27+、MyGo v0.5.5 Native UI、现有 `kline/` 包；对照参考 [KLineChart](https://klinecharts.com) API 语义（不移植 JS）。

## Global Constraints

- 全程 Native `ui.View` + `ui.Painter`；禁止 `ui.WebView` / HTML / Vite / WebView2。
- 云端负责实现与验证：`go test ./...`、`go build`、能开窗则截图；不把「请用户本地跑」当作完成条件。
- 每阶段单独 PR（或可合并的小 PR 串），合入后再开下一阶段；阶段间 API 保持可演进，避免一次性大重构。
- 不追求与 KLineChart 30+ 指标 / 全 overlay 字面 1:1；每阶段只交付清单内最小可演示集。
- 合成数据可继续用于无网演示；真实行情接入留到打谱主仓，本仓只定接口。
- 中文 README 补充「本阶段能做什么 / 明确不做」。

## 现状基线（main @ 8ee3616）

| 已有 | 说明 |
|------|------|
| 蜡烛 + 影线 + 量柱 | 绿涨红跌 |
| MA5 / MA20 | 硬编码在主图 |
| 平移 / 滚轮缩放 / Reset | 共享 Viewport |
| 十字线 + OHLCV 读数 | 双 pane 联动 |
| ALPHA / BETA 双图 | 各价轴，非同轴叠加 |
| 数据 | `kline.Generate` 固定 600 根小时线 |

关键文件：`main.go`（窗体与联动）、`kline/candle.go`、`kline/viewport.go`、`kline/paint.go`。

## 目标分期总览

| 阶段 | 主题 | 交付物 | 预估 |
|------|------|--------|------|
| **P0** | 数据层 + 周期 | `DataSource`、Period 切换、init/forward 语义、UI 切周期 | 3–5 天 |
| **P1** | 副图指标 | Indicator 接口、VOL 可配、MACD 副图、主图 MA 可开关 | 4–7 天 |
| **P2** | 画线 Overlay | segment / horizontal / priceLine，创建-选中-删除 | 5–8 天 |
| **P3** | 同轴叠加 | 第二序列归一或双价轴叠加模式 | 3–5 天 |
| **P4** | 样式与主题 | Style 结构外置、明暗、蜡烛变体（实心/空心） | 2–4 天 |

以下按阶段拆任务。完成标准均为：测试绿、构建绿、有帧或实窗证据、README 更新。

---

## File map（目标结构）

```
kline/
  candle.go          # Candle、Generate（保留作 SyntheticSource）
  viewport.go        # 视窗（基本不动，P0 可加按 timestamp 定位）
  paint.go           # 拆：主图蜡烛/量；指标与 overlay 另文件绘制
  paint_price.go     # P1 起：价区绘制
  paint_volume.go    # P1：量区（可被 Indicator VOL 替代或复用）
  layout.go          # Split / SplitNoTime / 多副图垂直分割
  data.go            # P0：Period、BarRequest、DataSource
  synthetic.go       # P0：SyntheticSource（包装 Generate + 周期重采样或 regenerated）
  indicator.go       # P1：Indicator 接口与注册表
  ind_ma.go          # P1：MA / EMA（先 MA）
  ind_macd.go        # P1：MACD
  ind_vol.go         # P1：VOL（或把现有量柱迁入）
  overlay.go         # P2：Overlay 模型与命中
  ov_segment.go      # P2：线段
  ov_hline.go        # P2：水平线 / 价线
  style.go           # P4（P0 可先抽出 StyleFrom）
main.go              # 窗体、工具条（周期/指标开关）、联动状态
```

P0 不必一次建齐空文件；按阶段新增。

---

## P0 — 数据层与周期

**Goal:** 图表数据来自可替换 `DataSource`；UI 可切换 Period；支持「向左拖近边缘加载更早数据」（forward）的最小实现。

**Architecture:** `DataSource` 提供 `GetBars(req) ([]Candle, more)`；`app` 持有当前 `Symbol`+`Period`+缓存切片；视窗仍按 **index**；周期切换时重置缓存与 `FitRight`。合成源按 period 生成不同粒度时间戳（不必真聚合 tick）。

### Task P0.1：数据接口与合成源

**Files:**
- Create: `kline/data.go`
- Create: `kline/synthetic.go`
- Create: `kline/data_test.go`
- Modify: `kline/candle.go`（Generate 可被 synthetic 调用，或迁入 synthetic）

- [ ] 定义：

```go
type Period struct {
    Span int    // e.g. 1, 5, 15, 60
    Unit string // "minute" | "hour" | "day"
}

type BarRequest struct {
    Symbol    string
    Period    Period
    // Kind: "init" | "forward" | "backward"
    Kind      string
    // Anchor: 已有最早/最晚一根的时间；init 时可零值
    Anchor    time.Time
    Limit     int
}

type DataSource interface {
    GetBars(req BarRequest) (bars []Candle, more bool, err error)
}
```

- [ ] 实现 `SyntheticSource`：`init` 返回 `Limit` 根（默认 600）；`forward` 在更早时间续生成；两 symbol 不同 seed。
- [ ] 测试：init 长度与时间单调；forward 接在现有最早 bar 之前且不重叠；非法 Period 返回 error。
- [ ] `go test ./kline -count=1` 通过后提交（云端 PR 内 commit）。

### Task P0.2：app 接入 DataSource + 周期切换 UI

**Files:**
- Modify: `main.go`
- Modify: `main_test.go`
- Modify: `README.md`

- [ ] `app` 增加 `src DataSource`、`period Period`、`symbol` 仍用 ALPHA/BETA 两路（可共享同一 Period）。
- [ ] 启动时 `Kind=init` 填两路 candles，再算 MA（暂留硬编码，P1 再指标化）。
- [ ] 工具条增加周期按钮：`1h` / `4h` / `1d`（最少三个）；点击后两 pane 一起 reload + `fitted=false` + `FitRight`。
- [ ] 测试：切换 period 后 bars 时间步长变化；Reset 仍可用。
- [ ] Tester 或实窗截图：工具条可见、切换后轴标签变化。
- [ ] README：写清 DataSource 与当前仅 Synthetic。

### Task P0.3：边缘加载 forward（最小）

**Files:**
- Modify: `main.go` / `kline/viewport.go`（可选 hook）
- Modify: `main_test.go`

- [ ] 当 `Offset` 接近左边界（例如 `Offset < 5`）且 `more==true`，调用 `forward`，将更早 bars **prepend**，并调整 `Offset += prependedCount` 保持画面不跳。
- [ ] 测试：prepend 后同一根时间的屏幕位置不变（允许 1px 误差用 index 断言）。
- [ ] 节流：加载中不加重复请求。
- [ ] PR 标题建议：`P0: DataSource, period switch, forward load`；合入后进 P1。

**P0 不做：** 真实行情 HTTP、websocket、`subscribeBar` 推送、缺会话空隙压缩。

---

## P1 — 副图指标

**Goal:** 可挂载指标；主图 MA 可开关；至少 **MACD 副图** + **VOL**（量柱指标化或保留专用量区）。

**Architecture:**

```go
type Indicator interface {
    Name() string
    // Pane: "main" | "sub"
    PaneKind() string
    Calc(cs []Candle) // 填充内部序列
    // DrawSub 仅副图；主图叠加走 DrawMain
    DrawMain(p *ui.Painter, priceRect ui.Rect, vp Viewport, y chart.Linear)
    DrawSub(p *ui.Painter, rect ui.Rect, vp Viewport)
    ValueAt(i int) string // 读数
}
```

副图高度：在 `layout` 里把每个 symbol 列切成 `price | vol/macd...`；MACD 与 VOL 可并存时垂直再分（先做 **price + one sub**，VOL 默认作 sub 或保持现量区 + MACD 再下一行）。

推荐首版布局（每 symbol）：

```
[ header ]
[ price + MA ]
[ volume ]      // 可视为内置 VOL
[ MACD ]        // 可开关
[ time axis ]   // 仅最底 pane
```

### Task P1.1：Indicator 骨架 + MA 迁入

**Files:**
- Create: `kline/indicator.go`, `kline/ind_ma.go`
- Modify: `kline/paint.go`（调用 MA 绘制）
- Modify: `main.go`

- [ ] `MA` 实现 Calc/DrawMain；`app` 用指标列表替代裸 `ma5/ma20` 切片字段（或两者并存一版后删旧字段）。
- [ ] UI：勾选/按钮「MA」开关。
- [ ] 测试：Calc 与旧 `SMA` 数值一致。

### Task P1.2：MACD 副图

**Files:**
- Create: `kline/ind_macd.go`, `kline/ind_macd_test.go`
- Modify: `kline/layout.go`（新建或从 paint 抽出 Split）
- Modify: `main.go`, `README.md`

- [ ] MACD(12,26,9)：DIF、DEA、柱；副图自有 Y 域。
- [ ] 十字线读数增加 MACD 值。
- [ ] 开关「MACD」；默认开。
- [ ] 测试：已知短序列 fixture 断言 DIF/DEA；UI 测试副图矩形非空。
- [ ] 截图：双图 + MACD 副图 + 联动竖线穿过副图。

### Task P1.3：VOL 指标化（可选收敛）

- [ ] 若量柱仍独立绘制，本任务可只文档声明「VOL = 内置量区」；或抽 `ind_vol.go` 统一 DrawSub。
- [ ] PR：`P1: indicators MA + MACD sub-pane`。

**P1 不做：** 完整 30 指标、自定义 `registerIndicator` 热加载、BOLL/SAR 等（可列后续 backlog）。

---

## P2 — 画线 Overlay

**Goal:** 用户能画 **线段、水平价线**；可选中、拖端点、Delete/右键删除；坐标存 `timestamp` 或 `barIndex` + `price`。

**Architecture:** `Overlay` 存逻辑点；绘制时用 `Viewport` + 价轴转为像素；命中测试用端点/线段距离。模式：`idle | drawing | selected`。

### Task P2.1：模型与水平价线

**Files:**
- Create: `kline/overlay.go`, `kline/ov_hline.go`
- Modify: `main.go`（工具条 Draw 模式）

- [ ] 点击价区放置水平线；拖动改价；选中高亮。
- [ ] 测试：像素↔价格往返；命中半径。

### Task P2.2：线段 segment

**Files:**
- Create: `kline/ov_segment.go`
- Modify: `main.go`

- [ ] 两击完成线段；未完成时橡皮筋；与 pan 手势冲突时：**Draw 模式下禁用 pan** 或要求按住空格/工具切换（选工具切换，简单明确）。
- [ ] 截图：两 pane 上各有线（overlay 默认只画在触发 pane；文档写明「先 per-pane」）。
- [ ] PR：`P2: overlay hline + segment`。

**P2 不做：** 斐波那契全集、brush、磁吸弱/强全档、跨 pane 复用同一 overlay（可 backlog）。

---

## P3 — 同轴叠加

**Goal:** 提供模式「Compare / Overlay」：两序列画在同一价区（建议 **归一化到 100** 或百分比涨跌），共用一价轴；保留现有「Stack 双 pane」为默认。

### Task P3.1：叠加模式

**Files:**
- Modify: `main.go`, `kline/paint*.go`, `README.md`

- [ ] 工具条：`Layout: Stack | Overlay`。
- [ ] Overlay：主图两色蜡烛或第二序列用线（线更清晰，推荐 **主序列蜡烛 + 第二序列收盘折线**）。
- [ ] 归一：`close[i]/close[0]*100`；Y 域跟归一后范围。
- [ ] 十字线读数显示两 symbol 值。
- [ ] 测试 + 截图。
- [ ] PR：`P3: same-pane overlay compare mode`。

---

## P4 — 样式与主题

**Goal:** 样式可配置；跟随 MyGo Theme；支持空心涨蜡烛（可选）。

### Task P4.1：Style 外置

**Files:**
- Create/Move: `kline/style.go`
- Modify: paint / README

- [ ] `Style` 含涨跌色、网格、十字线、MA/MACD 色；`StyleFrom(theme)` + `DarkDefault`/`LightDefault`。
- [ ] 空心阳线开关（涨：框、跌：实心）——A 股常见习惯可配置。
- [ ] 截图明暗或空心对比。
- [ ] PR：`P4: chart styles and hollow candles`。

---

## 验证清单（每阶段通用）

- [ ] `go test ./... -count=1`
- [ ] `go build -o /tmp/klinechart-go .`
- [ ] 无 WebView：源码检索 +（可选）`ldd` 无 WebKit
- [ ] Tester 帧或 GTK 实窗截图进 artifacts
- [ ] README「能做 / 不做」更新
- [ ] Squash 合入 `main` 后再开下一阶段

## Backlog（本方案明确延期）

- 真实行情 / websocket `subscribeBar`
- 全量指标与自定义脚本指标
- 斐波那契等其余 overlay、磁吸档位
- 对数轴、锁定 Y、多 Y 轴
- `getConvertPictureUrl` 级导出 API
- 打谱双盲训练流程（属 dapu 主仓，本仓只提供图组件能力）

## 实施顺序与门禁

1. 本文档评审通过（或用户说「按这个做」）→ 开 P0 PR。
2. P0 合入且验证截图齐 → P1 → P2 → P3 → P4。
3. 任一阶段若发现布局/手势模型要破相容，先停并更新本文档再改代码。

## 文档位置

- 工作区副本：`/workspace/klinechart-go-parity-plan.md`（本文件）
- 合入仓库时建议路径：`docs/plans/2026-10-20-klinechart-go-parity.md`（P0 PR 一并提交）
