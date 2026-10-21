# 最终复审与示例补齐方案（2026-10-21）

## 目标

在已有两轮复审和修复的基础上，只处理当前 HEAD 仍可复现的问题。过滤掉已经修复或仅属架构取舍的旧建议，并在修复后补齐宿主可直接运行的示例，最后用真实 Windows 原生窗口做交互验收。

当前基线：`main`、Go 1.27.1、Windows amd64、`CGO_ENABLED=0`。导出 API 保持兼容；不新增行情连接；不提高分配预算；不使用 WebView。

## 本轮确认的问题

### 契约正确性

1. **回放标记边界读取隐藏 bar**
   - `Replay.markerView` 用 `base[cursor].Time` 作为上限。下一根未揭晓 bar 的时间变化会改变当前标记可见性。
   - 标记过滤没有下界，聚合 bucket 向下取整后可能显示第一根基础 bar 之前的交易。
   - 修复方向：边界只由已揭晓前缀和基础周期计算，使用半开区间 `[first revealed open, last revealed open + basePeriod)`。

2. **订阅回滚时旧 handler 仍可写入**
   - `followLive` 在一个 symbol 订阅失败后，先逐个退订、最后才递增 `liveGen`。
   - 退订回调中触发的旧 handler 仍通过 generation 检查并改写图表。
   - 修复方向：进入回滚退订前先失效本轮 generation。

3. **回放回调中的无操作 `SetPeriod` 截断当前批次**
   - `Replay.setPeriod` 在校验和同周期判断前调用 `nextOp`。
   - `OnBar` 中调用同周期、非法或更细周期会静默停止外层 `Step`。
   - 修复方向：只有真正改变周期时才开启新的操作代际。

### 绘制与公开 API

4. **Overlay 指数基准接受 `+Inf`**
   - 正无穷 close 被当作合法基准，导致 raw scale 无穷或 compare line 全部归零。
   - 修复方向：基准选择、规范化、绘制统一要求有限且为正。

5. **价格轴上的指针仍报告一根 K 线**
   - `View` 把整个 chart 元素的 X 直接传给 `Viewport.IndexAt`，包括右侧价格轴。
   - 修复方向：横向十字线只接受价格绘图区宽度，纵向仍允许音量/副图带。

6. **Overlay 第二个标的买卖标记可能完全不可见**
   - 第二标的只画 close 线，但标记使用未绘制的 low/high 作为锚点，常落在裁剪区外。
   - 修复方向：在 overlay 中，第二标的标记以已绘制的 close 线为锚点；第一标的仍使用 OHLC 语义。

7. **`PaintCompare` 的尺度窗口短于绘制窗口**
   - 尺度按两条序列的公共长度计算，蜡烛按 primary 全长度绘制，primary 尾部会因域缺失被裁掉。
   - 修复方向：用 primary 的可见窗口计算尺度；compare 缺失的 bar 视为断点。

8. **时间轴标签不受 `ChartOptions.Location` 控制**
   - tick 在源时间戳的 location 中生成，默认上海读数与时间轴可能相差 8 小时。
   - 修复方向：刻度生成使用图表标签时区；时间到 bar 的 X 映射仍用绝对时间。

9. **极小宿主矩形会画出边界外**
   - 收起布局保留固定 gap，`bandsBottom` 可超过传入矩形。
   - 修复方向：空间不足时优先压缩 gap/可选副图，保证所有非空矩形位于宿主框内。

### 文档与示例

10. README 的嵌入示例仍用 `600519` 创建 chart、却用 `ALPHA` 查标记，示例块会静默跳过。
11. `SetSource` 的短注释没有说明 `SubscribeBar` 失败时新源与新 bars 已经安装。
12. 现有三个可运行示例覆盖面偏窄：缺少最小宿主、历史分页、交易标记、自定义副图、宿主自绘。
13. `examples/extend` 把 `High-Low` 作为主图价格线，会压缩价格域；应展示与价格同单位的主图指标。

## 实施顺序

### 阶段 1：失败测试与契约修复

- 回放标记：增加隐藏后缀时间无关、第一根前缀之前不可见、聚合桶上下界测试。
- 订阅回滚：增加退订期间旧 handler 不能写入的测试。
- 回放周期：增加回调内同周期、非法周期、更细周期不截断 `Step` 的测试。
- 指标有限性：增加 `+Inf` 基准、全零序列回退、规范化输出测试。
- 十字线：增加价格轴和绘图区左右边界的测试。
- Overlay 标记：增加第二标的买卖箭头像素非零、价格区裁剪测试。
- PaintCompare：增加 primary 比 compare 长时的尾部像素测试。
- 时间轴：增加同一瞬间使用 UTC/UTC+8 存储时标签一致、显式 `Location: UTC` 的测试。
- 布局：增加多个极小矩形所有非空 band 都位于宿主框内的测试。

每项先证明旧实现失败，再改最小实现。完成后运行 `go test ./kline -count=1`。本轮回归集中在 `kline/contract_fixes_test.go` 与 `kline/synthetic_paging_test.go`。

### 阶段 2：示例

新增可运行命令，每个都有一个 headless 测试驱动同一套 view/构建函数：

- `examples/host`：最小嵌入、`HideHeader`、宿主自己的表头、`Crosshair`/`Bar`/`Readout`、精度和时区。
- `examples/history`：固定缓存 `DataSource`、超过 600 根历史、左边界分页、`SetSource` 刷新、加载失败重试。
- `examples/trades`：买卖标记、堆叠、限价颜色、周期重映射、清空与恢复。
- `examples/indicator`：宿主副图指标、`SubPlot`、`BarTail`、开关与 `SyncIndicators`。
- `examples/paint`：宿主自行调用 `Viewport`、`Split`、`Paint`/`PaintCompare` 和 `Layer`，保留自己的 chrome。

同时修正 `examples/extend` 的主图指标语义，并更新 README 示例表。为 `StartReplay`、`SeekTo`、`Replay.Close`、`Chart.SetSource`、`Chart.AppendBar` 等关键路径补充 `Example` 函数或示例内可执行代码。

### 阶段 3：验证

```powershell
gofmt -l .
go build ./...
go vet ./...
go test ./... -count=1
go test ./kline -run Example -v
go test ./... -race -count=1
```

真实窗口验收至少覆盖：

- 根 demo：周期、指标、Stack/Overlay、拖拽、滚轮、十字线、画线、删除、Escape、回放。
- `examples/host`：宿主表头随十字线更新，价格精度与时区正确。
- `examples/history`：左边界加载、刷新和失败重试提示。
- `examples/trades`：标记可见、周期切换后仍落在正确 bar。
- `examples/indicator`：副图出现、开关、增量追加。
- `examples/paint`：宿主自绘、平移缩放、坐标一致。
- `examples/live`：后台推送、UI 线程投递、关闭退订。
- `examples/updates`：A/B 分步 AppendBar、共同时间揭晓、已显示与 held 两种 UpdateLast、修订值在配对揭晓时可核对、Skew 后的 AlignError、修订源后的 Reset。
- `examples/replay`：窗口内 Step/Back/Play/Pause/Seek。

所有窗口必须真实启动、交互、并通过标题栏正常关闭；记录进程退出和关键观测。无法实测的平台或 GPU 组合单独标注，不把本机结果外推。

## 完成标准

- 本轮 11 项代码缺陷都有永久回归测试。
- 新增示例可 `go run`，并有 headless 测试。
- 全部测试、API examples、race、分配预算通过。
- 真实窗口验收完成，文档记录实际场景与结果。
- 导出 API 无非必要破坏，README 与当前默认值一致。

## 进度

- [x] 阶段 1：11 项契约与绘制问题有永久回归测试，`kline` 全部测试通过。
- [x] 阶段 2：原有 3 个示例加新增 6 个示例；9 个示例都有 headless 测试。host/history/trades/indicator/paint/updates 共用生产构造器与 view，live 共用队列帧门与 view，extend/replay 共用构造与 view。README 示例表、`SetSource` 失败边界和 API Example 已同步。
- [x] 阶段 3 最终窗口抽查：全量测试、API examples、Windows race、分配预算、随机顺序测试和 8k 基准已通过；当前根 demo 与 9 个示例完成真实原生窗口控制级抽查，证据见 `docs/reviews/evidence/2026-10-22-final-interaction.txt`。
- [x] 交付前复审：修复价格轴十字线残留、live 单槽丢 bar/数据竞态、updates 部分 Skew 状态、replay 订阅失败后的失效指针；补相应回归并按 23:00 工作树重新生成 12 包测试、随机顺序、50 Examples、race、覆盖率和 8k 基准证据。
- [x] 真实窗口：当前工作树的十个程序已重新构建并通过启动、标题、响应和关闭检查；2026-10-22 使用新版原生 Computer Use 完成逐窗口控制级抽查，记录见 `docs/reviews/evidence/2026-10-22-final-interaction.txt`。

### 2026-10-21 复核补充修订

本轮再次只读复核后，剔除三项已被当前代码和回归测试覆盖的旧结论：

- `SyntheticSource` 对合法的 `3h` 请求不会永久扫描；`ashareOpens` 在首个工作日没有开盘时返回 `ErrInvalidPeriod`，`data_test.go` 覆盖该路径。
- `Replay.Close` 后的同周期 `SetPeriod` 会重新向数据源加载隐藏后缀；`TestReplayCloseThenSamePeriodReloads` 覆盖该行为。
- 回放标记已同时使用已揭晓前缀的下界和上界，且包含聚合周期测试。

以下问题仍成立，按顺序处理；每项先补失败测试或可核对证据，再改实现/文档：

1. **示例测试复用不足（已修复）**
   - `host` 提取 `newHostChart`，生产程序和测试使用同一构造器。
   - `live` 的 `attach` 是 `window.Update` 的有序队列帧门；生产路径与测试共用 `view`、`newLiveChart`、`attach` 和 `dispatch`。队列按 FIFO 保留每次 revision/new-open，不能用一个 pending 槽覆盖先前回调；测试覆盖突发新开盘、慢 UI 顺序、周期工具栏和关闭围栏。测试只替换“请求下一帧”的副作用，因为 `ui.NewTester` 没有真实 `window.Update`；被入队的 chart 回调仍是生产实现。
2. **示例回归覆盖边界不足（已确认已有覆盖）**
   - `TestNativeViewMarkPlacementDragAndDelete` 已实际覆盖放置、拖动、Delete、右键删除和 Pan 恢复。
   - `TestNativeReplayControls` 已覆盖 `StepBack`、`SeekTo`、时间前后边界、周期切换、Restart、Return to source 和末尾停止。
3. **绘制边界断言不足（已确认已有覆盖）**
   - `TestCrosshairIgnoresPriceAxis` 已覆盖画布左右边界、绘图区内部和平移后的价格轴。
   - Overlay 第二标的的 buy 和 sell 分别用独立颜色断言非零像素，并断言没有落到成交量区。
4. **文档精确度（已修复）**
   - 修正 `ExamplePeriod` 注释，不声称解析；说明它只验证和格式化。
   - 修正 `FormatPriceDigits(..., 0)` 的注释，说明固定精度、四舍五入到整数。
   - 审查后确认 README 没有 `SplitNoTime` 的调用；API 清单中的 `Split` 与 `examples/paint` 一致，无需改写代码。
   - README 继续明确区分“共用工厂/view”和“仅测试数据路径”的示例，不能把二者都写成同一构造保证。
5. **真实窗口矩阵**
   - 当前构建重新跑完十份二进制（根 demo 加 9 个示例）的人类可见交互后，才填写验收表并勾选阶段 3。
   - 终端启动、`Responding` 和关闭消息只作为启动证据，不作为交互证据。

修订后的验证顺序：

1. 先跑聚焦测试：`go test ./examples/... ./kline -count=1`。
2. 再跑全套：`gofmt -l .`、`go build ./...`、`go vet ./...`、`go test ./... -count=1`、`go test ./kline -run Example -v`、`go test ./... -shuffle=on -count=1`、race。
3. 构建 `build/acceptance` 十份二进制；每一份在真实窗口执行清单动作并记录。
4. 最后更新验收表、README 边界和本计划；没有实际观察到的项保持 `BLOCKED`，不得勾选完成。

### 交付复核补充

- 订阅回滚测试原先从已删除的 handler 表取值，实际没有调用旧回调；现在保存旧 handler，在后续退订时调用它并断言图表不变。
- 价格轴十字线测试移到轴内部，不再以“轴外的无效索引”为证据；primary 尾部测试同时断言 `CompareScales` 的 raw 域包含尾部高点。
- 合成源价格已经跨窗口稳定，但历史边界原先随 `Limit` 变化。现在固定为 `DefaultLimit + Extra`（负 `Extra` 保留单次窗口、无历史分页），并验证分页大小 1、17、200、800。
- 真实窗口交互受原生命令名与设备缩放影响，不能从截图文件名推断成功。验收记录区分两类证据：启动、渲染和交互实际完成；无法从截图恢复的输入值或瞬时状态单独说明。
- 早期桌面控制曾被 Escape 中断；本轮在用户明确授权后，使用新版原生 Computer Use 完成十个当前程序的逐窗口抽查。进程启动、交互与正常关闭分别记录，未用启动证据替代控制观察。
- 最终人工验收使用 [最终窗口验收清单](../reviews/evidence/2026-10-21-final-window-acceptance.md)，逐项填写真实窗口结果；清单未全部 PASS 前，阶段 3 保持未完成。

### 最终验证结果

| 检查 | 结果 |
| --- | --- |
| `gofmt -l .` / `git diff --check` | 无输出 |
| `go build ./...` / `go vet ./...` | 通过 |
| `go test ./... -count=1` | 12 个包全部通过；输出见 `docs/reviews/evidence/2026-10-21-final-tests.txt` |
| `go test ./... -shuffle=on -count=1` | 12 个包全部通过；输出见 `docs/reviews/evidence/2026-10-21-final-shuffle.txt` |
| `go test ./kline -run Example -count=1` | 50 个公开 API Example 全部通过；输出见 `docs/reviews/evidence/2026-10-21-final-examples.txt` |
| `kline` statement coverage | 89.4%（本轮复测） |
| `go test ./... -race -count=1` | Windows amd64 通过；临时 MinGW-w64（命令进程 PATH，未改系统环境） |
| `examples/live` 队列/后台循环回归 | 有序队列、突发新开盘、慢 UI 与关闭围栏测试通过；Windows race 下全部通过 |
| 本轮 11 项契约回归 | 聚焦测试通过；输出见 `docs/reviews/evidence/2026-10-21-final-contract-tests.txt` |
| 5 个 allocation budget tests | 全部通过 |
| 8k benchmark | 最新复测 Stack 174,020–174,026 B/op；Overlay 84,668–84,673 B/op；Marker 174,013–174,024 B/op；AppendBar 899–945 B/op；转录见 `docs/reviews/evidence/2026-10-21-final-bench-refreshed.txt`，均低于测试预算 |
| `examples/updates` headless | 通过；两个标的的分步追加、共享开盘、shown/held 修订、部分 Skew 保留已提交边缘、历史 `AlignError` 不误判最新提交、reload reset 均由 `main_test.go` 驱动 |
| `go run ./examples/extend -headless` / `go run ./examples/replay -headless` | 本轮执行通过；输出可由命令重跑，见示例 headless 测试 |
| 当前工作树窗口检查（2026-10-22） | 10 个 exe 标题匹配、`Responding=True`、`CloseMainWindow` 后退出码 0；转录只记录启动与关闭；见 `docs/reviews/evidence/2026-10-21-final-launch.txt` |

### 示例覆盖后续（2026-10-21）

“尽可能多 examples”仍未封顶。本轮已把 API Example 从 24 个扩到 50 个；以下条目属于已完成集合：

- `ExampleViewport`：`FitRight`、`Pan`、`Zoom`、`Clamp`、`Visible`、`IndexAt`、`BarLeft`，展示共享视口的基本坐标契约。
- `ExamplePriceDomain` / `ExampleVolumeMax`：非有限值过滤、空区间回退，以及指标扩展价格域。
- `ExampleChart_SetTool` / `ExampleChart_SetMagnet`：内置工具、非法工具不改变当前状态、三种磁吸模式。
- `ExampleChart_SetIndicator` / `ExampleChart_IndicatorNames` / `ExampleChart_IndicatorEnabled`：单指标切换、工具栏绑定指针、未知名称失败不改状态。
- `ExampleChart_Health`：`LoadError`、`AlignError`、`Visible`、`ResetView`、`HollowEnabled` 等宿主状态读取。
- `ExampleNormalizeClose` / `ExampleIndexBase` / `ExampleNormalizeToIndex`：Overlay 归一化与共同基准。
- `ExampleStubLiveSource`：历史、订阅、推送、退订的完整内存源契约。

另补 `ExampleSnapPrice`、`Chart.SetHollow`、`Chart.SetLimitPct`、`Chart.ResetView`、`Chart.Symbols`、`Chart.AlignError`、`ExampleDataSource`、`ExampleRegister`、`Chart.SyncIndicators`、`Chart.Crosshair`、`Replay.Play`、`Replay.StepBack`、`Paint`、`PaintCompare`、`Chart.EditStyle` 与 `BarTail`。50 个 Example 的精确输出全部由 `go test` 校验，证据见 `docs/reviews/evidence/2026-10-21-final-examples.txt`。

下一批最高价值缺口：自定义 `DataSource`、`Register`/`SyncIndicators`、`Replay.Play`/`StepBack`、直接 `Paint`/`PaintCompare`、指针 `Crosshair`、`BarTail` 增量计算、`EditStyle` 主题语义。它们展示的是宿主必须自己实现的契约，优先级高于再补单字段 getter。

### 真实窗口验收边界

| 场景 | 结果 |
| --- | --- |
| 根 demo：首帧、周期、指标、Stack/Overlay、十字线、画线/Delete、Escape、回放 Step/Back/Play/Pause | 当前构建已在本机 Windows 11 真实窗口执行并观察；记录见 `docs/reviews/evidence/2026-10-22-final-interaction.txt` |
| 当前工作树 10 个程序：启动、标题、窗口响应、关闭退出 | 已验证；转录见 `docs/reviews/evidence/2026-10-21-final-launch.txt` |
| 当前工作树的控制级交互（根 demo 与 9 个示例） | 十个程序均已在真实原生窗口抽查关键控件与状态转换；逐项观察见 `docs/reviews/evidence/2026-10-22-final-interaction.txt`。未穷尽所有输入组合、DPI 和 GPU 环境 |
| 早先示例截图：history、trades、indicator、host、live、paint；extend/replay 命令行 smoke | 本机 Windows 11 执行并观察；截图中的界面可能早于最后的布局改动，只作为当时行为证据，不作为当前构建的桌面矩阵证明 |
| 9 个示例的每个鼠标动作与发布前完整矩阵 | 未完成；本轮覆盖各示例的关键控件与状态转换，但不宣称穷尽每个坐标、输入组合或瞬时帧 |
| macOS / Linux GUI；其他 Windows/GPU 驱动组合 | 未实测，不外推 |
