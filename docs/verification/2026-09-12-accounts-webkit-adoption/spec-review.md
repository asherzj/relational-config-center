# Issue #110 adoption — Spec 轴审查

**对象**：base `4b45aa1b3f16a9ca8401cc7aa0c887c751d0aae4` → tree `c9ec3b4bf753508d835c6a2c1239179144564b17`；证据 `SHA256SUMS` SHA-256 `bb068439b05f6fab2d2bfb34968c4cb20ca7c2f3d954ec6dd5efefe97f79e8cb`。

## Findings

未发现阻塞本地提交的 Spec 偏差。

- **需求/AC**：AC-F01 要求“区分按钮稳定性等待与浏览器关闭原因”。旧 1.62.1 正式 run 的 16 PASS/1 accounts WebKit FAIL/17 未执行、page-crash→test-failure→cleanup 时序，以及另一次 2336 compositor NULL SIGSEGV 均保留且未合并归因。AC-F02/03 的正式候选是与已审版本 patch 字节一致的 Playwright/core 1.63.0（package/lock SHA 分别 `14ad8e40…`、`df1e4ed0…`）；同一候选在 Linux amd64 完成 12 个独立 Node/profile 的 accounts WebKit 轮次，每轮原 21 项检查，越过 25 条分页边界，正文/键恢复、权限、隔离、SQL/fixture、cleanup 均通过。36 份 native trace 仅见 SIGCHLD、无异常退出或信号。
- **证据限制**：这只是不同 GitHub VM、带 ptrace 的 12 轮整体版本效果；不识别具体 WebKit 修复，也不排除更低频复发。浏览器 cache 未上传，launcher/依赖恢复主要由前后 SHA/mode 与 supervisor 成功记录证明。1.63 同时升级 Chromium 1243、Firefox 1543、WebKit 2359，因此 WebKit-only 对照不能覆盖前两者。
- **范围/可疑实现**：持久产品/E2E 逻辑未改；`accounts.mjs` 仍为已审 SHA `27596293…`。依赖图只替换 Playwright/core 并删除旧 Playwright 唯一引用的 Darwin-only `fsevents@2.3.2`。未见 force、retry、timeout/断言/动画降级。首次单测 479/480 因沙箱 `listen EPERM` 超时，原日志保留；允许本地监听后同源 480/480，另有 typecheck/build、2 个 dev-origin 检查通过。
- **临时退出**：CI YAML 与原 main `a306c94f…` 字节一致，只余 8 个正式 job；临时 job、DEBUG/ptrace、3 个诊断脚本、generated runner 均删除。活动 version fixture 已逐字迁入无消费者的 `retired-fixture/` 历史证据。

## Gate

112/112 冻结条目哈希通过。**本地提交 gate：通过。最终交付 gate：须与 #112 集成后的无 ptrace Linux amd64 34-case Browser 及原 8 项 CI 全部有效通过；当前不能宣称原生故障已根治或整体交付完成。**
