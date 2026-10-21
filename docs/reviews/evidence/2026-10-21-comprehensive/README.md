# 全库复审证据

对应 [完整复审报告](../../2026-10-21-comprehensive-reassessment.md)。`*_test.go.txt` 是隔离副本运行过的复现代码，使用文本扩展名避免污染正常 `go test ./...`。

## 重现

在 PowerShell 中提供需要审查的实际仓库路径：

```powershell
& ./docs/reviews/evidence/2026-10-21-comprehensive/reproduce.ps1 -RepositoryRoot (Get-Location).Path
```

脚本只复制源码到新建临时目录，在那里加入复现测试并运行；不修改、删除原仓库文件。它会输出目录和 Go 退出码。当前已审查状态应失败，修复后相关断言应通过。

默认运行确认的 10 项缺陷（12 个顶层失败断言，因为 replay 生命周期有三个场景）及指标一致性对照。不包含 D1 的同周期重开争议，单独检查可在脚本输出的临时目录运行：

```powershell
go test ./kline -run '^TestReview(CloseThenSamePeriodResubscribes|ReopenSamePeriod)$' -count=1 -v
```

## 文件

- `source-manifest.json`：审查开始时 72 个 Go/模块文件的 SHA-256。
- `baseline-tests.log`、`api-examples.log`、`coverage-functions.txt`：既有测试、公开示例和覆盖率。
- `modules-verify.log`、`format.log`、`race.log`：模块校验、格式检查、race 未能执行的原因。
- `bench-review.txt`：同一 Windows 主机的三轮定向基准，未替换仓库基线。
- `confirmed-probes.log`：主审在同一快照再次运行确认缺陷和指标对照的结果。
- `replay-controls.log`：五个既有 replay/lifecycle 测试通过，说明新的复现补充了组合边界。
- `reopen-contract.log`：D1 实际行为，解释冲突见报告，不计入确认缺陷数。
- `marker-spill.png`：MyGo headless 原生绘制的实际像素图，紫色为复现时指定的 marker 专用色。

这些日志的失败表示复现成功，不是对库进行了修复后的验证失败。完整测试结果只适用于清单对应的源码；修复或其他工作区更改后应重新运行。
