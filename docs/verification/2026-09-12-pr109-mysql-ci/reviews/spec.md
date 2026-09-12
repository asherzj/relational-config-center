# Spec

**状态：窄修复提交 gate 通过；Findings：0。**

#106 要求「适用完整 Go/Web/MySQL/浏览器回归与逐 AC 证据索引」且不能以构建代替功能验证。本变更只重排完整 MySQL 验证，不改产品。冻结 manifest SHA-256 `55481f45…9a9ae3` 与 6 个源码、15 个证据文件哈希全部一致。

`mysql_integration.py` 使用真实 `go test -tags=integration -list . -json ./...` 发现顶层 Test/Example/Fuzz，以 `(package,name)` 去重、排序并 round-robin 分四组；集合并集和总数同时校验，空组也失败。实际清单为 431 个唯一身份，独立结果与 runner 分配逐项一致：108/108/108/107；`cmd/admin` 为 94/94/93/93。

每组按包串行运行 `go test -p 1 -count=1 -timeout=60m -json -run <精确顶层正则>`，保留完整子测试。记账要求每个 selected 都有顶层 run/pass，拒绝顶层或子测试 skip/fail、漏跑、额外顶层项、非 JSON、包终态异常及进程非零；包失败后继续其他包，默认 `make test-integration` 继续其余组后统一非零。CI 才以四个独立 runner 并行，`fail-fast:false`，70 分钟 job 上限未放宽，且 `always()` 上传原始清单、分配、逐包 JSON 和结果。

真实 Go fixture 与 parser 5 项通过，覆盖跨包同名、Unicode、Example、Fuzz seed、完整子测试及失败后续跑；`make test` 通过。原 CI 确认 `cmd/admin` 累计 60 分钟超时；被中断测试单独对真实 MySQL 发出唯一 run/pass，10.23 秒通过。证据未把原非 `-v/-json` 日志推断成逐项 PASS，也未声称本地四组全跑。

**Unresolved：** 四组在 GitHub Linux runner 的实际耗时和全量结果尚未知。

**必需待补证：** post-push 四个 matrix job 均须以 artifact 证明各自 selected 全部 run/pass、无 skip/extra，四组并集仍为同一 431 项。该未来 full gate 不构成本次 runner 窄修复的本地验证缺失。
