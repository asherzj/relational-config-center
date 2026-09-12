## Standards

审查对象：基点 `ea36801375a9e888708c6e9c51caea45cff66be4`；源码快照 manifest SHA-256 `75757e74bc6bf8ed516bce3a259c9e01a3014310a66a08090284f47d5392bd1e`；路径 `web/e2e/browser-accessibility.cjs`。

状态：通过本地固定快照 Standards gate；正式 post-push CI 尚待运行。

- **Hard violations：0。** 差异在 `reload()` 与 `repeatDraftSave(page)` 的下一次导航之间，等待精确可访问名称为 `Managed Table` 的 combobox。该等待以用户可见的工作区控件确认 React 会话恢复完成，符合 `web/DESIGN.md` 对真实控件语义、认证中断恢复及浏览器验收的要求。变更未增加超时，且保留最终 `assert.deepEqual(pageErrors, [])`，没有掩盖页面错误。
- **Subjective smells：0。** 两行注释准确解释浏览器 document load 与 React 恢复的时序差异；单一语义等待没有引入重复、神秘命名、无需求抽象、消息链或其余基线坏味道。
- **Unresolved code findings：0。** 等待点范围窄，复用 Playwright locator 的既有默认超时；未改变业务行为、写入断言或错误断言。
- **必需待补证：0。** 证据 manifest SHA-256 为 `5b4a8de5dcc8b2f20c742b0b553665e36f201f74d4ade85f6922a6445443020d`；53/53 文件的字节数与 SHA-256 匹配，内部 `SHA256SUMS` 全通过。冻结的独立 Linux WebKit 专项退出 0、7 项 PASS、`pageErrors: []`、残留行 0，且 fixture 前后相同。
- **Post-push CI：1（后续，非本地 gate 阻塞）。** 三引擎尝试中 Chromium/Firefox 各 7 项通过；WebKit 在到达修复点前因未查明的审批 403 停止，后续 26 个原 CI case 未执行。README 准确保留该边界；正式 Linux amd64 CI 尚未运行，不得声称三引擎或整套 CI 已绿。

结论：hard 0，subjective 0，unresolved 0，必需待补证 0；post-push CI 待补 1。
