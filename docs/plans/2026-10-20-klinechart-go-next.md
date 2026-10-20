# KLineChart-go 下一阶段 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在已合入的 P0–P4 基线上，按打谱优先级补齐：真行情**只留接口**、BOLL/KDJ、磁吸+斐波那契、可嵌入 Chart API；仍保持 MyGo Native、无 WebView。

**Architecture:** 继续以 `kline.DataSource` + `Indicator` + `Layer` 为扩展点；实时流用可选接口挂在源上，本仓不接 HTTP；指标与画线按现有注册/绘制路径增量扩展；最后把窗体逻辑收成可被 dapu 导入的 `Chart` 组件，`main` 退成薄 demo。

**Tech Stack:** Go 1.27+、MyGo v0.5.5 Native UI、现有 `kline/`；语义对齐 [KLineChart](https://klinecharts.com)（不移植 JS）。

## Global Constraints

- 全程 Native `ui.View` + `ui.Painter`；禁止 WebView。
- 每阶段单独 PR，合入后再开下一阶段；`go test ./...`、`go build`、有帧/截图、README「能做/不做」更新。
- **真实行情：只定接口与文档 + 可测 stub；禁止在本仓实现任何股票/券商 HTTP 或 WebSocket 客户端。**
- 不追求 KLineChart 全量指标/全 overlay；只做本清单。
- 云端自行验证并开 PR；合入由协调者完成。

## 现状基线（main @ 99b510f）

已有：`DataSource`/`SyntheticSource`、周期与 forward、MA+MACD、H-Line/Segment、Stack/Overlay、`Style`+Hollow、双窗十字线联动。`main.go` 仍是胖 demo 窗体。

## 分期总览

| 阶段 | 主题 | 交付物 |
|------|------|--------|
| **P5** | 真行情接口 | `LiveSource`（subscribe/unsubscribe）+ stub；README 接入说明；无网络实现 |
| **P6** | 打谱指标 | BOLL 主图、KDJ 副图；工具条开关 |
| **P7** | 磁吸 + 斐波那契 | Magnet off/weak/strong；Fibonacci 画线工具 |
| **P8** | 可嵌入 API | `kline.Chart`（或等价）可被外部 `import`；`main` 变薄 demo |

---

## File map（增量）

```
kline/
  data.go            # P5：可选 LiveSource
  live_stub.go       # P5：内存 stub，供测试
  ind_boll.go        # P6
  ind_kdj.go         # P6
  magnet.go          # P7：价格磁吸
  ov_fibonacci.go    # P7
  chart.go           # P8：可嵌入组件
  chart_opts.go      # P8：选项
main.go              # 逐步变薄；P8 只组装 Chart
```

---

## P5 — 真行情接口（无网络实现）

**Goal:** 打谱/实盘以后能插真实源；本仓只定义契约与 stub。

### Task P5.1：LiveSource 接口

**Files:** Modify `kline/data.go`；Create `kline/live_stub.go`, `kline/live_test.go`；Modify `README.md`

- [ ] 在 `DataSource` 旁增加可选实时接口（名称可微调，语义对齐 KLineChart）：

```go
// BarHandler receives one bar update (new or revise last).
type BarHandler func(bar Candle)

// LiveSource is optional. A feed that can push bars implements it.
// GetBars remains on DataSource for history.
type LiveSource interface {
	DataSource
	SubscribeBar(symbol string, period Period, onBar BarHandler) error
	UnsubscribeBar(symbol string, period Period) error
}
```

- [ ] `SyntheticSource` **不**实现 `LiveSource`（或实现为 no-op 返回明确 error），避免假实时。
- [ ] `StubLiveSource`：内存队列；`SubscribeBar` 后测试可 `Push` 一根；断言 handler 收到；`UnsubscribeBar` 后不再投递。
- [ ] README：说明真实源 = 实现 `DataSource` + 可选 `LiveSource`；本仓不提供券商客户端；dapu 主仓负责接行情。
- [ ] 可选：`main` 用 type assert 检测 `LiveSource`，有则订阅、无则忽略——**不接网络**。
- [ ] PR：`P5: LiveSource interface + stub (no network feed)`。

**P5 不做：** 任何 HTTP/WS 行情、A 股/港股具体协议、鉴权。

---

## P6 — BOLL + KDJ

**Goal:** 打谱常用的两条：布林带主图、KDJ 副图。

### Task P6.1：BOLL

**Files:** Create `kline/ind_boll.go`, `kline/ind_boll_test.go`；Modify registry / `main.go` / layout if needed；README

- [x] BOLL(20, 2)：中轨 SMA20，上下轨 = 中轨 ± 2×标准差；`PaneMain`；三条线。
- [x] 工具条 **BOLL** 开关；默认关（避免主图过挤）。
- [x] 测试：短 fixture 断言中轨与已知 SMA 一致；上下轨对称。
- [x] 十字线 `ValueAt` 含 MID/UP/LOW。

### Task P6.2：KDJ

**Files:** Create `kline/ind_kdj.go`, `kline/ind_kdj_test.go`；Modify layout（第二副图或与 MACD 互斥/并存——**推荐并存：price | vol | MACD | KDJ**，KDJ 默认可关）

- [x] KDJ(9, 3, 3)：RSV → K/D/J；副图自有 Y（通常 0–100）。
- [x] 工具条 **KDJ**；默认关。
- [x] 测试：fixture 断言 K/D/J 公式。
- [x] 截图：BOLL+KDJ 打开时 Stack 布局。
- [x] PR：`P6: BOLL main + KDJ sub-pane`。

**P6 不做：** RSI/SAR 等其余指标；自定义脚本指标。

---

## P7 — 磁吸 + 斐波那契

**Goal:** 画线点能吸到 OHLC；斐波那契回撤线可画。

### Task P7.1：Magnet

**Files:** Create `kline/magnet.go`, `kline/magnet_test.go`；Modify overlay 放置路径 / `main.go`

- [x] `MagnetMode`: `off` | `weak` | `strong`。
- [x] 弱磁：指针价距最近 OHLC（或收盘）小于阈值（DIP 或价格比例）则吸附；强磁：强制吸到最近 OHLC。
- [x] 工具条 **Magnet: Off / Weak / Strong**；仅影响新点与拖端点，不改已有线批量重算。
- [x] 测试：给定 bar OHLC 与指针价，断言吸附结果。

### Task P7.2：Fibonacci

**Files:** Create `kline/ov_fibonacci.go`；Modify `overlay.go` Hit/Draw；`main.go` 工具

- [ ] 两点定高低；画标准回撤水平线：0 / 0.236 / 0.382 / 0.5 / 0.618 / 0.786 / 1（可配置 ratios 常量）。
- [ ] 工具 **Fib**：两击完成；选中可拖端点；Delete/右键删除同现有。
- [ ] Hit：命中任一条水平线或端点。
- [ ] 截图 + PR：`P7: magnet snaps + fibonacci overlay`。

**P7 不做：** 全套射线/平行通道/笔刷；跨 pane 共享同一 overlay。

---

## P8 — 可嵌入 Chart API

**Goal:** dapu（或其他 Go 宿主）能 `import` 图表组件，不再只能跑本仓 `main`。

### Task P8.1：抽出 Chart

**Files:** Create `kline/chart.go`（及必要拆分）；Slim `main.go`；Create `kline/chart_test.go`；README「嵌入」小节

- [ ] 公开类型大致形状（实现时可微调命名，保持可测）：

```go
type ChartOptions struct {
	Source     DataSource
	Symbols    []string // e.g. ALPHA, BETA
	Period     Period
	Layout     string   // "stack" | "overlay"
	Style      Style
	Hollow     bool
	Indicators []string // names from Registry, e.g. "MA","MACD","BOLL","KDJ"
	Magnet     MagnetMode
}

type Chart struct { /* opaque */ }

func NewChart(opts ChartOptions) *Chart
func (c *Chart) View() ui.View           // or embeddable widget factory MyGo expects
func (c *Chart) SetPeriod(p Period) error
func (c *Chart) SetLayout(layout string)
func (c *Chart) SetSource(src DataSource) error
```

- [ ] 现有工具条行为迁入 `Chart` 或由宿主自绘工具条 + 调上述 API；**最低要求**：无工具条也能显示 Stack K 线+量+已选指标，平移缩放十字线可用。
- [ ] `main` 只：`NewChart` + 窗口 chrome + 工具条绑定。
- [ ] 测试：`NewChart` + Tester 出帧非空；换 Period 不 panic。
- [ ] README：一段「在其他模块中嵌入」示例代码。
- [ ] PR：`P8: embeddable Chart API`。

**P8 不做：** 打谱双盲训练流程（属 dapu）；打包成 C ABI / 动态库。

---

## 验证清单（每阶段）

- [ ] `go test ./... -count=1`
- [ ] `go build -o /tmp/klinechart-go .`
- [ ] 无 WebView
- [ ] Tester 帧或实窗截图
- [ ] README 更新
- [ ] Squash 合入 `main` 后再开下一阶段

## Backlog（本方案仍不做）

- 券商/行情 HTTP·WS 客户端
- 全量 30 指标、脚本指标
- 射线/通道/笔刷等其余 overlay
- 对数轴、锁定 Y、多 Y 轴、导出图片 API
- dapu 双盲训练 UI

## 实施顺序

1. 本文档确认（或用户说按这个做）→ P5 PR。
2. P5 → P6 → P7 → P8，逐段合入。
3. 布局/手势要破相容时先停并更新本文档。

## 文档位置

- 工作区：`/workspace/klinechart-go-next-plan.md`
- 合入仓库建议：`docs/plans/2026-10-20-klinechart-go-next.md`（P5 PR 一并提交）
