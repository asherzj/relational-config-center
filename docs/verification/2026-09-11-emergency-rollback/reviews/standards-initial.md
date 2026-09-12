# Standards 初审

状态：存在阻塞发现。仅审 Standards，不评价 Spec 或运行中验收。

输入：基点及 HEAD 均为 `d31eabd6f7947004d39fe872afee8aaca37bbad2`，commit 列表为空；审查完整待提交快照。开始时 43 个 manifest 路径与固定 diff 一致，源码哈希全部匹配。结束复核发现工作树的 `admin/cmd/admin/release_paged_reads_integration_test.go`、`admin/cmd/admin/release_quick_rollback_integration_test.go`、`web/e2e/release-rollback-flows.cjs` 已偏离 manifest；本报告仍仅绑定原固定 diff，其 SHA256 未变：`664dd688ec4a979a607c4472a203e29e561392c3c48904f28675423ab4eb007c`。

## 成文规范违规

- **[P2] 冲突面板的读取入口仍发送恢复预览写请求。** `web/src/features/release-orders/ReleaseRequestReview.tsx:49–55`（hunk `@@ -41,12 +43,17 @@`）在“查看最新状态与配置”（第 90 行）内调用 `previewWrite.send/retry`，第 51 行还可直接确认替换已拒绝预览。`web/DESIGN.md:154` 明确要求：“未知结果关闭或刷新后仅在快速回滚窗口通过‘保存恢复预览’手动重推原包”，确定冲突通过“读取最新状态并重新保存恢复预览”建立新请求。实际路径为：回滚执行被拒绝 → 冲突面板查看 → 新预览响应丢失 → 刷新后再次查看；此时读取按钮会重推写请求，用户无法从操作名称辨认正在保存或重建预览。应将该分支引导至共享快速回滚窗口，由明确保存按钮处理原包恢复及冲突重建，保留原原因与请求。

## 主观坏味道

独立发现 0。已对照全部指定基线；不重复计入上述入口分歧，不报告工具已强制项目。

本次只读审查；未运行 DB、浏览器或测试，未修改源码或提交。Standards：1 项成文规范违规（最高 P2），0 项独立主观建议。
