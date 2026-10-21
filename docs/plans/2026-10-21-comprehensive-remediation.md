# KLineChart-go 全库复审修复实施方案

## 目标与约束

本方案修复 [全库复审](../reviews/2026-10-21-comprehensive-reassessment.md)中确认的 2 项 P1、8 项 P2，并统一 1 项 Close 后恢复订阅的文档冲突。目标是恢复“回放不泄露未来”和“扩展条目按文档可见”两个端到端契约，同时保持导出 API 兼容、原生绘制路径不变，不新增行情连接。

每个 PR 都先合入能在旧行为上失败的测试，再改实现。禁止只增加 happy path 断言，禁止提高现有 allocation ceiling 来掩盖回归，禁止顺手加入未请求的功能。每个 PR 单独成分支、单独提交，沿用仓库的单功能 PR 规则。

工作区当前已有大量未提交改动。实施前先从用户确认的基线创建分支；不要回退、覆盖或重新排版这些改动。若准备修复的文件在开工后又发生变化，应先停止并重新核对差异。本方案写于当前工作区，不表示已完成任何修复。

## 验收门槛

每项缺陷的最小复现必须先失败，修复后必须通过；最终 PR 还要通过：

```powershell
gofmt -l .
go build ./...
go vet ./...
go test ./... -count=1
go test ./kline -run Example -v
go test ./... -race -count=1
```

`-race` 需要 cgo 和 C 编译器。当前验收基线是 Windows amd64，本机使用临时 MinGW-w64 工具链运行，不改系统环境。涉及绘制帧的改动还要运行对应 pan/append/marker 基准以及 `go test` 内的分配预算；不得替换 `bench/baseline.txt`，除非同 OS、arch、Go 的 benchstat 为中性或更好。

建议的排查命令为：

```powershell
go test ./kline -run '^TestReview' -count=1
go test ./kline -run '^$' -bench 'Benchmark(PanFrames|OverlayPanFrames|MarkerPanFrames|AppendBar)$' -benchmem -count=3
go test ./kline -run Example -v
```

旧复现文件名为 `TestReview*`，只适合作为临时回归来源；正式修复应把断言移入命名表达行为的永久测试，例如 `TestReplayCallbackReplacementStopsOldAdvance`。不要把 `docs/reviews/evidence/*.go.txt` 直接改为 `*_test.go`。上述 `TestReview` 命令只在隔离复现目录中运行，不属于仓库最终测试集。

## PR 1：回放操作代际与所有权栅栏

修复 **B1**。建议标题：`fix: fence replay advances after owner changes`。

### 修改

在 `Replay` 内增加私有操作序号，例如 `opGen uint64`，每次新的顶层 `Step`、`StepBack`、`SeekIndex`、`SeekTo`、`Tick` 或 `setPeriod` 开始执行时递增，并让本次调用保存看到的值。另用 `chart.replay == r && !r.closed` 继续作为所有权判断。不要给 `Chart` 增加公开代际 API。

不要让 public `Step` 调 public `SeekIndex` 时重复递增，否则合法的嵌套实现会把本批次误判为失效。可把入口统一为私有 `seekIndex(i, gen)`，public `Step`/`StepBack`/`SeekTo` 每次只开一个操作代际。

改写 `advance`：循环中每一根 bar 开始前核对所有权；`revealOne` 写入完成后、调用 `OnBar` 前登记本次已经推进的 cursor；`OnBar` 返回后立即再次核对所有权与 `opGen`。如果回调执行了 `Close`、启动替代 replay、触发新的 Seek/Step 或结束当前播放，本次批次必须立即退出，返回时不得继续 reveal。回调里的 `StartReplay` 可能关闭旧 replay；`Close` 不能从 `started=false` 改成失败状态，已经提交的当前 cursor/bar 保留，但余下待推进数量丢弃。

`Tick` 在 `advance` 后仍要复核 `r.playing`、所有权和 cursor，避免回调 `Pause` 后继续消费同一批 due bars；公开语义是“暂停停在当前已经揭露的那一根”，`Tick` 返回 nil 状态且不再请求帧。不要通过禁止 `StartReplay`/`Close`/`Pause` 来回避问题，README 已经允许这些生命周期操作。

`revealOne` 多 symbol 的逐 symbol 写入需要额外防御：当前 `appendBar` 的私有路径一次只写一个 symbol，若第二个 symbol 写入失败，第一个 symbol 已变化。此 PR 不需要重构该失败恢复，但要在 `OnBar` 前确认所有 symbol 均已提交，并把 reveal failure 保持为“停止播放”而不是 owner fence 错误。

更新 `OnBar`、`Close`、`Playing` 文档：回放只保证“已经 return 的 callback 对应的 bar 可见”；callback 结束后的批次是否继续由当前 owner 决定。README 的 replay 段补一句：切换回放或关闭后，旧回放的当前批次立即停止，不会写入替代回放。

### 测试

在 `kline/replay_lifecycle_test.go` 增加：

- `TestReplayCallbackReplacementStopsOldAdvance`：旧 replay `Step(4)`；第一个 `OnBar` 调 `StartReplay`；断言 old `ErrReplayClosed` 或当前批次被取消、new `Cursor()==0`、`chart.Bars()==0`、callback 只执行一次。
- `TestReplayCallbackCloseStopsBatch`：callback 调 `Close`；断言 `Step` 不继续 reveal，callback 次数为 1，cursor 恰好停在已揭露的 1。
- `TestReplayCallbackPauseStopsTick`：`Play(10)` 后推进 500ms，callback 调 `Pause`；断言只推进 1 根，`Playing()==false`，下一次 `Tick` 不再推进。
- `TestReplayCallbackCloseAndReplacementAreTerminal`：callback 内 Close 或 StartReplay 后，当前批次不得继续写；不要再用嵌套 Seek 作为验收路径。现有文档禁止 callback 调 `Step`、`Seek`、`Tick`，而 `Close`、`StartReplay`、`Pause` 是可测试的合法生命周期动作。
- 现有 `TestReplayPlayPauseUsesClock`、`TestReplayTickStopsWhenRevealFails`、`TestReplayRevealsThroughPrivateAppend` 必须继续通过。

### 完成标准与回滚

`go test ./kline -run 'Replay' -count=1` 全通过；全量测试和 race 通过；replay benchmark 的 bytes/op 不因新增 epoch 字段增长。若实现需要改变公开回调限制，先改 README 加新测试，不要暗中改变语义。回滚只回滚本 PR 的操作代际与测试，不回滚工作区中其他 replay 防护。

## PR 2：统一标记的基周期可见窗口

修复 **B2、B3**。建议标题：`fix: clip markers to revealed base bars`。

### 修改

不要继续用 `Chart.period` 作为标记截止时间。增加一个统一的 reindex 输入结构，例如：

```go
type markerView struct {
    candles []Candle
    period  Period
    until   time.Time // zero means no replay cutoff
}
```

或等价参数 `func (a *Chart) reindexMarks(s *series, period Period, revealedUntil time.Time)`。`barContaining` 只负责半开区间，不读取 chart 的全局周期。把该参数显式传到 `mount`、`appendBar`、`updateLast` 和 `SetMarkers` 的 reindex 路径，不要通过临时修改 `Chart.period` 或在每个调用点重新推导。

正常加载和周期切换：`install(p, next)` 先把目标周期传给每个 pane 的 mount/reindex，再替换 `a.period` 或在函数参数中显式使用 `p`。不要为了这一个 bug 让 mount 临时改 `a.period`，因为后续 recalc/回调可能观察中间状态。`SetPeriod(1h→4h)` 时末根 00:00 的 03:00 标记必须可见；`4h→1h` 时必须再次按 1h 的末根边界隐藏。

Replay：在每种显示模式下，使用 `base` 的已揭晓前缀得到 `revealedUntil`。当 cursor 小于 base 长度时，边界是 `base[0][cursor].Time`，即下一根未揭晓 bar 的 open，恰好复用 `[bar open, next bar open)` 语义；没有隐藏后缀时才是 `base[0][cursor-1].Time + basePeriod`；cursor 为 0 时为零值。不要简单使用“最后一根已揭晓 open + 周期”，否则周末、午休或停牌会造成误裁。

`reindexMarks` 先排除 `m.Time >= revealedUntil` 的标记，再按当前显示周期 `period` 映射到聚合蜡烛。这样只揭晓 08:00/09:00 时，4h 部分桶不会显示 10:00 的交易；09:59 的交易仍会正确落到 08:00 桶。等价地，可以先找到 marker 所在的 base bar index，仅当该 index < cursor 时才映射到显示桶；实现时选一种并让 stack、paint、Readout 共用，避免边界规则分叉。

`SetMarkers`、`AppendMarker`、Step、Seek、StepBack、SetPeriod、SetSource 和 load 都必须走这一个视图函数；不要在 `drawMarkers` 或 `readout` 用不同条件二次过滤，否则标记索引和堆叠编号会再次不一致。`tradeMark` 不需要新增公开字段；内部可以缓存可见性，但不把截止时间暴露给宿主。

当 replay 使用粗周期并继续 reveal 到 10:00 后，该标记应出现在 08:00 的聚合桶中；StepBack 到 09:00 后重新隐藏并重算同侧 stack。`SetSource` 的普通加载没有 replay，使用全量已加载边界。

文档同步 `README.md` 第 27/142 行和 `StartReplay` 注释，说明“marks are clipped by the revealed base interval before aggregation”。不要只写“future markers hidden”而不解释聚合桶。

### 测试

在 `kline/marker_test.go` 或新增 `marker_visibility_test.go` 增加：

- `TestMarkerReindexUsesTargetPeriodOnLoad`：单根 00:00 末根 + 03:00 marker，1h→4h 后可见且 Readout 包含 label；4h→1h 后隐藏。
- `TestReplayAggregateMarkerStaysHiddenUntilBaseReveal`：08、09 后切 4h，10:00 marker 仍在图外；`Step` 到 10:00 后出现在 08:00 桶；`StepBack` 隐藏并恢复 stack。
- `TestReplayAggregateMarkerStackUsesVisibleOnly`：10:00 与 11:00 的同类 marker，在只揭晓 9:00 时 stack 不占用可见位置，逐步揭晓后 stack 顺次变化。
- `TestMarkerBoundaryUsesHalfOpenInterval`：`t == bar open` 属于该 bar；`t == next open` 属于下一根；replay 截止时间使用同一条规则。
- 普通 `SetSource`、forward prepend、周期切换测试继续通过，确保无 replay 时没有额外隐藏。

### 完成标准与回滚

`go test ./kline -run 'Marker|Replay' -count=1` 通过；全量测试、API examples、race 通过；Marker pan 分配预算仍通过。不要在 `drawMarkers` 里按时间裁剪来“修补”索引错误；如果实现只能做到绘制时不显示，Readout 或 stack 仍可能泄露，视为未完成。回滚本 PR 不会影响 PR 1 的所有权修复。

## PR 3：真实交易数据的时间轴映射

修复 **B4**。建议标题：`fix: place time ticks on actual bars`。

### 修改

`drawGrid` 当前用 `chart.NewTime(left,right)` 得到自然时间比例，再把 `tick.At` 当 X 比例。桌面时间在 bar index 上均匀，真实数据会有周末、午休和停牌，这两个坐标系不一致。

保留 `chart.NewTime` 生成“应标哪些时刻与标签”，但把 tick 时间映射到 candle-index 横轴，而不是全局自然时间比例。推荐实现一个分段映射：用 `sort.Search` 找到 `tick` 落在 `cs[i].Time` 与 `cs[i+1].Time` 之间，计算 `i + (tick-cs[i])/(cs[i+1]-cs[i])`，再乘 `vp.BarWidth` 并减去 `vp.Offset`。正好等于 `cs[i+1].Time` 时落在下一根 bar；周末等缺口只拉伸对应相邻 bar 的间隙，不会把整月/整周的时长线性压到所有蜡烛上。首尾外推复用现有相邻周期规则。

将 `timeTickX(cs, vp, tick)` 或等价的 `timeToBarPosition` 独立出来，Stack 和 Overlay 共用。不要在找不到“精确等于 open time”的 tick 时直接全部跳过：09:30 开盘或整点 candle 会让 `chart.NewTime` 的多数 tick 无法精确匹配，从而产生空时间轴。若产品选择只标 candle 自身时间，应走另一套“从 candles 生成标签”的算法，并先更新规格。

如果产品需要“非交易日不标、交易日标”，不要用时间比例近似；保留相同标签格式，只改变 X 映射。不要让 `TimeAt` 的自然时间外推影响这里。Overlay 和 Stack 共用 `drawGrid`，一次修复两条路径。

分段映射仍有一个产品取舍：周末刻度会出现在两根相邻交易 bar 之间的空隙中，而不是强行贴到某一根 bar。这保持了 tick 的真实时间含义；日期标签的正确交易日由 candle 的 X 坐标决定。对需要“只标交易日”的产品，可以再过滤掉没有 candle 的完整自然日，但不能把过滤写成跨日近似映射。

### 测试

- `TestTimeTicksFollowCandleIndexesAcrossSessionGap`：周五 + 周一 + 周二的 fixture，周一 tick 的 X 必须落在周一 bar 的 `[BarLeft, BarLeft+BarWidth)`，不得进入 Tuesday。
- `TestTimeTicksSpanSessionGapsWithoutGlobalDistortion`：周末/停牌期间的 tick 位于相邻 bar 之间的分段区间，不能偏移到之后的 Tuesday candle。
- `TestTimeTicksMatchBarWidthZoom`：同一 fixture 在缩放后 tick 的 X 跟随 `BarLeft`，不被自然时间插值拉伸。
- `TestTimeTicksCoverOffGridSessionBars`：09:30/13:00 等非整点 fixture 仍能产生可读的时间标签。
- Stack 与 Overlay 各渲染一帧，确保两种布局都改到。
- 现有 `TestViewport...` 与 `TestPaint...` 继续通过；不要修改 `TimeAt` 的既有外推语义，除非同时有单独行为测试和文档说明。

### 完成标准与回滚

全量测试通过；Stack/Overlay pan 预算不恶化，时间轴相关 headless 图像没有新增越界文本。若某实现需要改动 `TimeAt` 公共行为，拆成单独 PR，先更新其注释和兼容性评估。回滚本 PR 只恢复 tick 位置逻辑。

## PR 4：扩展绘制的 Slot 绑定

修复 **S1、B5、S2**。建议标题：`fix: paint host indicator slots exactly once`。

### 修改

先修 S1。把 `layoutPanes` 与 `subLayers` 的对应关系从“只按具体类型选第一个”改为稳定顺序的实例分配。`enabledSubs` 返回 `[]subIndicator`，至少包含 `slotIndex int`、`ind Indicator`，按 `a.slots` 顺序生成。保持公开 `Panes` 结构不变：第一个 MACD 和第一个 KDJ 继续使用专用 rect，其他所有 sub 实例（包括第二个 MACD/KDJ）依次使用 `Extra` rect；`Panes.Extra` 的容量要覆盖全部剩余 sub 实例。内部可以再保存 slot→rect 映射以保证 `subLayers`、hit test、clip 和轴完全一致。

明确以下规则：

- 第一个启用的 `MACD` 使用 `Panes.MACD`，第一个启用的 `KDJ` 使用 `Panes.KDJ`。
- 同类型的第二个及以后实例使用额外 band，使用自己的 `SubPlot.Scale`。
- 额外 band 的轴、绘制、clip 和 hit 使用同一 rect。
- 不依赖 `ind.Name()` 猜测槽位；slot index 才是身份。
- `orderedSpecs` 仍决定工具栏顺序，但绘制必须按实际 slot 对应，不要用 map 迭代顺序。

再修 B5。主图线条路径只在 `LinePlot.Series()` 非 nil 时使用；实现接口但返回 nil 的指标回到 `DrawMain`。`Series()` 返回空但非 nil 的切片按“有 LinePlot 但无线条”处理，避免再次调用 DrawMain；把这个区分写入注释和测试。Overlay 的既有限制保持：主图自定义 DrawMain 仍不在 overlay 中补画，除非另立需求。

最后修 S2。为内置 MA/MACD/BOLL/KDJ 名字建立不可变保留集合；`Register` 在注册锁内/外都拒绝保留名，`Unregister` 对保留名直接 no-op，不能删除内置 spec。测试清理只使用宿主自己的临时 name，不能为了测试恢复 registry 而绕过保护。README 把“built-in names and tool names stay reserved”落实到 `Unregister`，不再只依赖注册时的重复检查。

### 测试

- `TestSecondSameKindSubPaneGetsOwnBand`：注册 `FASTMACD`，与 `MACD` 同时启用，断言 `len(subLayers)==2`，两个 rect 不重叠，两个 `DrawSub` 都被调用。
- `TestSecondKDJGetsOwnBand` 与自定义 `PaneSub` 的额外 band 测试。
- `TestNilSeriesLinePlotFallsBackToDrawMain`：`Series()==nil` 时 DrawMain 恰好调用一次；非 nil 空 series 不会调用 DrawMain。
- `TestBuiltInIndicatorNamesStayReserved`：`Unregister("MA")` 后 `Registry` 仍有 MA，`Register` MA 仍返回错误，`NewChart` 仍得到默认 MA。
- `TestHostIndicatorSurvivesSync`：已有 `IndicatorEnabled` 指针在 `SyncIndicators` 后仍可切换；新增 slot 按稳定顺序。
- `TestOverlayStillSkipsDrawMain`：确认不在 overlay 中悄悄调用主图 DrawMain。

### 完成标准与回滚

全量测试、API examples、race 通过；Stack/Overlay pan 的 bytes/op 不得超过现有预算，尤其检查 `TestStackPanAllocationBudget` 和 `TestOverlayPanAllocationBudget`。S1 是正确性修复，不允许用“只保留第一个”的文档修改来验收。S2 的保护必须跨 chart 共享，不能只阻止当前 chart 的重复名。

若 S1 需要改变 `Panes.Extra` 的公开含义，改为发布前单独讨论并补兼容性测试；优先保持现有 struct 字段和公开构造行为。回滚本 PR 不影响标记、时间轴的修复。

## PR 5：Overlay 基准与像素裁剪

修复 **B6、B7**。建议标题：`fix: align overlay fallback and clip markers`。

### 修改

B7：把“能否指数化”统一成一个判断，输入必须包含 primary 和 compare 两条序列以及 base。不能只看 `indexable(primary, base)`。建议：

```go
func canIndexCompare(primary, compare []Candle, base int) bool {
    return indexable(primary, base) && indexable(compare, base)
}
```

但要注意：`compare == nil` 可能是合法的“单标的 overlay”路径（代码、文档和测试已允许把单标的当指数化）。因此判断必须显式区分“没有 compare 序列”与“有 compare 但没有共同正 close”。新增一个内部 `overlayMode` 或 `(hasCompare bool, canIndex bool)` 结果，供 `CompareScales`、`drawCompare`、`indexValue`、`readout` 共用。有 compare 且任一系列在 base 不可指数化时返回原始主尺度；无 compare 的单标的 overlay 保持现有行为，除非文档同时决定改为 stack。

`drawCompare` 返回的 `bool`、`CompareScales` 的 `base` 和 `indexValue` 必须使用同一结果；不要让设置 header 显示 100 但绘制回退原价。`compareBase` 仍返回 base，但调用者要同时拿 `indexed`，避免只用 base 推断成功。

B6：在 `drawMarkers` 内部调用 `p.Clip(plot, 0, func(){...})`，将整个循环包住；`FillPath`、`Text`、tick line 都不能在价格区外产生像素。若标签本身被设计为可以在价格区边缘外延伸，先改产品契约并增加明确的 label clip 选择；当前 README 只承诺箭头在 bar 外，不承诺覆盖量区。不要只裁三角形而漏掉文字/ tick。

把图像测试改成可重复的像素断言：记录价格区与成交量区的专用 marker 颜色计数，允许抗锯齿误差；使用测试临时目录写 PNG，不写死个人机器路径。确保 `ui.NewTester` 的 headless 图像与真实应用绘制约束一致。

### 测试

- `TestCompareFallbackRequiresBothSeries`：ALPHA 正常、BETA 全零，断言 `indexed=false`，轴等于 `MainScale(primary)`，header 不显示虚假 `idx`。
- `TestCompareSingleSeriesKeepsDocumentedBehavior`：无 compare 时走当前单标的 overlay 语义；若选择改变，应先更新 README。
- `TestComparePaintAndHitUseSameFallbackScale`：headless 点击/绘制/hit 都使用同一 raw scale。
- `TestMarkersStayInsidePricePlot`：12 个同 bar 买标记，价格区有颜色，成交量区 marker 专用颜色为 0；检查文字和 tick 也已裁剪。
- `TestMarkerPanAllocationBudget` 继续低于现有上限；如果 clip 改变分配，先优化循环复用，不能抬上限。

### 完成标准与回滚

全量测试、示例和 race 通过；Stack/Overlay/Marker pan 预算通过；图像检查在有/无 marker、缩放和平移时都通过。回滚本 PR 不应影响 PR 1–4 的回放、标记和扩展修复。

## PR 6：初始加载与 Close 生命周期收口

修复 **B8**，并决定 **D1** 的统一契约。建议标题：`fix: bound initial pending bars and define reopen`。

### B8 修改

把 `held` 的写入统一到一个 helper，例如 `retainHeld(s, bars) (dropped int)`，初始加载、reload、live `hold`、promote 后排空都调用它。`alignInit` 目前直接 `append([]Candle(nil), ...)`，必须改为 `retainHeld`：

- 最多保留最新 `maxHeldBars` 根。
- 被丢弃的数量和最早时间通过新 `alignErr`/丢弃集合返回，而不是静默。
- `install` 的 `a.alignErr = nil` 只能在加载没有 pending 丢弃时执行；有丢弃时保留 `ErrMisaligned`，不能让成功 install 清掉刚产生的报告。
- 初始 A 600/B 1 的 fixture 断言 `len(held)<=512` 且 `AlignError` 非 nil；被保留窗口的最新时间仍可与 B 对齐。
- `SetPeriod`/`SetSource` 失败仍保持原数据；成功加载的丢弃报告需要明确生命周期（下一次成功 load 清掉），测试该重置边界。

`hold` 仍负责 live 路径；不要复制两套截断算术。检查 `append` 的 `held` slice 复用是否有别名风险，如果 helper 使用 `append(s.held[:0], ...)` 要保持容量和后续数据隔离。

### D1 决策

当前 README 同时说：

- `SetPeriod` 同周期且无 LoadError 是 no-op；
- Close 后可通过后续 `SetPeriod`/`SetSource` 重载并恢复订阅。

选择 **明确支持隐藏后再显示** 作为主方案，因为它符合已有 `TestCloseThenSetPeriodResubscribes` 和宿主示例。给 `Chart` 增加私有 `closed bool`（或 `liveDetached bool`），`Close` 标记、`dropLive` 后保持；`SetPeriod` 同周期的快速返回增加 `!a.closed` 条件，`SetSource` 成功后清除 closed，`install`/`followLive` 成功后清除该状态。另一个可选方案是保留 no-op，并把 README 改成“同周期不会重开”，同时删除/改写与该文档冲突的宿主承诺；但这样同周期恢复要强制用户换周期，接口体验较差，推荐前一种。

不要把 Chart 的 closed 状态和 Replay 的 closed 混用一个字段：`Replay.Close` 仍只结束回放，Chart 仍可按文档恢复 live。Close 后 `AppendBar`/`SetMarkers` 是否允许应保持现状并单独测试，避免扩大到无关 API。

### 测试

- `TestInitialHeldBarsUseCap`：初始加载 600 vs 1，`held<=maxHeldBars`、`AlignError` wraps `ErrMisaligned`。
- `TestInitialHeldDropSurvivesInstall`：成功 install 不静默清除初始 pending discard，下一次成功 reload 清掉旧报告。
- `TestSetPeriodFailureKeepsInitialPendingReport`：失败 reload 保持当前 bars、period、held 和 AlignError。
- `TestCloseThenSamePeriodReopensLive`：Close 后 `SetPeriod(Period())` 返回 nil、`liveSubs` 恢复、后续 push 可见；`SetSource` 路径仍有现有测试。
- `TestCloseAfterReopenAgainStopsUpdates`：再次 Close 不接收回调，`Step` 等 replay API 的行为不变。
- README 第 146/148 行用一致措辞描述“同周期 no-op 仅在 open chart；Close 后同周期会 reload/resubscribe”。

### 完成标准与回滚

全量测试、API examples、race 通过；加载相关分配和 append 预算不恶化。若产品决定不采用 reopen 方案，必须同时删除或修正冲突文档和测试，交付说明中明确选择。B8 不能只通过调整 README 来验收；它是已复现的实际上限违约。回滚本 PR 时，PR 1–5 的回归仍应保持通过。

## 依赖顺序、PR 拆分与验证矩阵

| 顺序 | PR | 依赖 | 主要文件 | 关键测试 | 风险 |
| --- | --- | --- | --- | --- | --- |
| 1 | 回放所有权 | 无 | `replay.go`, `replay_lifecycle_test.go`, README replay | callback Close/StartReplay/Pause/Seek | 中；易引入递归或取消误报 |
| 2 | 标记可见窗口 | 1 | `marker.go`, `replay.go`, `chart_load.go`, marker/replay tests | 聚合桶/边界/周期切换 | 高；索引与 stack 必须同源 |
| 3 | 时间轴 | 无 | `paint.go`, viewport/paint tests | 周末缺口/缩放/两布局 | 低到中；只改 X 映射 |
| 4 | Slot 扩展 | 无 | `chart.go`, `paint.go`, `layout.go`, `indicator.go`, extend tests | 双 MACD/nil Series/保留名 | 中高；布局兼容 |
| 5 | Overlay fallback + marker clip | 2, 4 | `compare.go`, `marker.go`, `chart.go`, compare/marker tests | 无基准/像素/预算 | 中；公开 Panes 语义敏感 |
| 6 | 初始 pending + reopen | 1, 2 | `chart_load.go`, `chart_opts.go`, lifecycle tests, README | 512 cap/Close 同周期/失败 reload | 中；报告清理与订阅状态 |

PR 3 与 PR 1 可以并行准备，PR 2 依赖 PR 1 的回放边界语义，PR 4 可与 PR 2 并行开发但不能共享同一分支；PR 5 依赖 PR 2 和 PR 4，PR 6 最后收口跨模块状态。每个子代理只负责一个 PR 的研究与测试，主审单独做集成回归。若并行开发，使用独立 worktree；不能在当前用户脏工作区上多人同时改同一文件。

## 回归矩阵

### 功能回归

| 场景 | 断言 |
| --- | --- |
| Replay Step 在 OnBar 内 Close/StartReplay/Pause/Seek | 旧批次不越过 owner 变化，新 replay 从空 prefix 开始，帧循环停止 |
| Replay 1h→4h 部分桶 | 隐藏未来交易；到 base cursor 后才出现在桶；StepBack 再次隐藏 |
| 普通 1h↔4h | 末根 marker 使用目标周期边界，Readout 与绘制一致 |
| 周末/停牌 | tick 落在实际 candle 的 bar 区间；无 candle 的 tick 不画 |
| 单/多标的 overlay | 只有 primary + compare 都有正 base 才 index；无 compare 走已文档化路径 |
| 主图 LinePlot 返回 nil | 回退 DrawMain；非 nil 空 series 不误画 |
| 同类型副图 | 每个 enabled slot 都有独立 rect、scale、clip、DrawSub |
| marker 堆叠 | 仅在价格区绘制，量区无像素，stack 不以未来 marker 占位 |
| 初始化 pending | held 不超过 512，丢弃有 AlignError，下一次 load 正确清报告 |
| Close 同周期 | 按最终契约恢复订阅，或文档和测试明确不恢复 |

### 验证顺序

先为每项写失败的 focused test，再实现最小改动；每完成一个 PR 运行对应包测试和全量测试，避免把多个回归混在一起。涉及画布的 PR 在 headless 图像检查后，再手工运行 `go run ./examples/live` 和 `go run ./examples/replay`（若环境无显示，只记录未验收）。最后在干净基线上重跑 `go test ./... -count=1`、race、examples、分配预算和三轮基准，并对比同机中位数。

## 交付记录

每个 PR 的说明应包含：

- 旧行为的最小复现与失败测试名。
- 修改的内部不变量，以及为什么没有改变公开 API。
- 通过的命令、结果、未执行项及原因。
- 适用时的像素/基准证据和 OS/arch/Go 环境。
- 已知迁移或兼容性影响；没有则明确写“无导出 API 变更”。

审查报告中的 10 项缺陷不是实施完成标志。修复完成后，在新报告中按每项写 closure 证据；若某项因产品决策不修，必须改文档并把原复现保留为已知限制，不能保留“已修复”措辞。

## 结果复盘

当前状态：10 项缺陷与 D1 已在审查工作区完成修复；没有回退审查前已有的本地改动。由于该工作区包含用户既有修改，本轮没有拆分提交或 push，`main` 仍指向审查起点。六组变更按文件边界可直接回滚；若拆成独立 PR，应依托干净基线逐组迁移以下提交内容。

1. 回放所有权：私有 `opGen` 在回调前后复核 owner/closed/generation，Step、Seek、Tick、Pause、setPeriod 各只开启一次顶层操作。测试为 `TestReplayCallbackReplacementStopsOldAdvance`、`TestReplayCallbackCloseStopsBatch`、`TestReplayCallbackPauseStopsTick`。
2. 标记可见窗口：`markerView` 同时携带目标周期与 replay 的排他截止时间，所有 mount、append、Step、StepBack 和周期重索引共用。测试为四个 `Test*Marker*` 用例。
3. 时间轴：`timeBarPosition` 按相邻 candle 分段映射 tick；像素测试直接验证实际 grid 的 Monday x 落在 Monday bar。测试为四个 `TestTimeTick*` 用例。
4. 扩展绘制：MACD/KDJ 首次实例保留专用 rect，后续实例进入 Extra；nil Series 回退 DrawMain；内置名不可卸载/替换。相关测试位于 `extend_test.go`。
5. Overlay 与像素：nil compare 保持单标的路径；非 nil compare 必须两序列都可指数化，尺度、header、hit 一致；markers 全部裁剪到价格区。相关测试位于 `compare_test.go`、`marker_test.go`。
6. 加载与生命周期：初始 pending 与 live pending 共用 `trimHeld` 的 512 cap，丢弃作为成功 load 后的 `AlignError` 报告；Chart 私有 `closed` 使同周期 SetPeriod 可重开，SetSource 与周期重载也按同一契约恢复。相关测试位于 `held_cap_test.go`、`replay_lifecycle_test.go`。

验证结果：`go test ./... -count=1`（235 个顶层 Test、38 个子用例、10 个 Example）、`go build ./...`、`go vet ./...`、`gofmt -l .`、`go mod verify`、Windows `go test ./... -race -count=1` 和五项 allocation budget 全部通过；`kline` 覆盖率为 88.0%。Windows race 使用临时 MinGW-w64 GCC 16.2.0，`CGO_ENABLED=1`，不修改系统环境。8k 三轮基准为 Stack pan 172,973–172,979 B/op、Overlay pan 83,997–83,999 B/op、Marker pan 172,970–172,979 B/op、AppendBar 857–924 B/op，均低于既有上限，未替换 `bench/baseline.txt`。隔离复审探针全部翻绿；时间轴旧探针按当前 candle-index 映射在隔离副本复跑通过。

行为文档已同步 `README.md`：回放回调批次、聚合标记裁剪、marker 价格区裁剪、同周期重开、init pending 上限和 LinePlot 空/nil 语义。Windows amd64 已用临时 MinGW-w64 在 `CGO_ENABLED=1` 下通过 `go test ./... -race -count=1`；该工具链只存在于临时目录，没有修改系统 PATH 或用户环境变量。CI 主矩阵已改为 Windows，并在 race job 中安装 MinGW。

真实桌面窗口也已验收：主窗口验证 Stack、1h/4h、BOLL/KDJ、Overlay、拖拽、滚轮、十字线、H-Line/Delete、Segment/Escape 和 Replay；`examples/live` 验证后台推送、`Marshal` UI 线程投递和周期重订阅；临时窗口 runner 验证宿主 LinePlot 与自定义绘图工具。三个窗口都通过标题栏正常关闭；进程、runner 和构建产物已清理。详细操作与观测见复审报告的 Windows 真实窗口验收表。该结论限于本机 Windows 11/Direct3D 11 环境，不外推为所有 Windows 与 GPU 驱动组合。

计划中的核心判断是：回放必须由所有可能被回调改变的 owner 状态共同授权；标记必须以“已揭晓基周期截止点”而非聚合 candle 的完整区间判断；指标绘制必须以 slot 身份而不是具体类型身份绑定 rect。三个判断落实后，剩余修复才是局部尺寸和边界问题。
