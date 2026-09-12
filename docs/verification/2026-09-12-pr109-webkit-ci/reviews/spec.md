# Spec

**状态：窄修复提交 gate 通过；Findings：0。**

规格要求包括 #106「真实浏览器完整桌面/390px/键盘及会话路径」，以及用户确认的「保持已确认发布/会话恢复原键正文重试」。源码 manifest SHA-256 `75757e74…bd1e`、唯一源码哈希，以及 evidence manifest `5b4a8de5…020d` 内 53 个文件哈希均已核对。

`browser-accessibility.cjs:390-417` 仅在 `reload()` 后等待名称精确且可见的 “Managed Table”，再调用原 `repeatDraftSave`。该门槛证明认证后的 React 工作区已挂载；恢复失败时等待本身会失败。timeout、生产代码和最终 `pageErrors === []` 未改。两次 POST、原正文与 `Idempotency-Key` 深度相等、独立审批后发布完成、SQL 恰一行及 cleanup 为零等断言均保留；无 scope creep、兼容分支或错误吞噬。

**证据：** 原 Linux CI 的 WebKit 七项业务检查完成后因 auth/session access-control page error 失败。精确小回路中，macOS 20 轮 before/after 为 8/0 个 page error，Linux arm64 为 15/0；两边 native uncaught error 均为 0。实际后端专项由 Linux WebKit 连接 macOS Admin/MySQL，七项通过、`pageErrors: []`、原 body/key 重放为真、SQL=1、cleanup=0。此边界已在 README 明示，不伪称完整 Linux CI。

**Unresolved：** 三引擎尝试中 Chromium、Firefox 通过，WebKit 在到达本修复行之前因审批 403 停止；现有证据不能证明其原因或已解决。

**必需待补证：** post-push 正式 Linux amd64 三引擎 CI 须复核共享夹具审批路径，并运行原 CI 未执行的后续 26 cases。该 pending 不构成本次窄修复本地验证缺失，也不能在完成前宣称全套通过。
