# #87 回滚原因补填、修正与历史留痕

本地候选基于 `7daa8b50f002d98ca6b8d41228a9cae2bcfd2f1b`，只实现父规格 #81 的 AC-013。完整功能最终全套由后续 #88 统一运行；本记录只陈述 #87 的受影响验证。

## 真实 HTTP 与 MySQL

`admin/cmd/admin/release_rollback_reason_integration_test.go::TestRollbackReasonCanBeCorrectedByExecutorOrAdministrator` 使用公开 HTTP、会话和隔离 MySQL 8.4 完成申请、独立审批、正向发布及原单快速回滚。初始行为 RED 为实际回滚执行人降为 VIEWER 后详情没有 `edit-rollback-reason`；实现后同一纵向用例通过，并验证：

- 原申请人、正向发布人和无关 VIEWER 均为 403；实际回滚执行人降为 VIEWER 后仍可补填，当前 ADMIN 可修正。
- 补填、修正和清空三次真实修改分别保留操作者、数据库时间、完整内容、不变版本和同一 `ROLLBACK` execution ID。2,001 UTF-8 字节被拒绝，空原因可成功保存。剥离新增 `ROLLBACK_REASON` 历史后，整张 `domain.ReleaseOrder` 与回滚完成响应完全相同，已有审批与生命周期历史也未被改写。
- 同键同正文返回逐字相同响应，同键异正文为 409。强制主单 JSON 更新失败返回 503，不留下原因历史或占用请求键；去除故障后由用户以原键原正文手动提交成功。
- 业务行与记录版本、明细三段实际结果、成功执行、表版本／游标、Command、通知和目标在成功、失败及重推后均未变化。

最终专项日志为 `/private/tmp/rcc-issue-87-evidence/mysql-integration-green.log`：1 个顶层用例及 3 个拒绝子例通过，耗时 9.81 秒。新增 execution ID 对既有原单与快速回滚的影响由 `/private/tmp/rcc-issue-87-evidence/affected-rollback-integration.log` 验证，3 个顶层用例通过：原单保留两次实际执行、混合发布恢复与原键结果重放、当前角色及已审阅版本／摘要门禁。更早的行为 RED 和首次较小范围 GREEN 保留在 `/private/tmp/rcc-issue-87-evidence/red-green.md`，最终 9.81 秒日志以更完整的清空及字节边界断言取代中间 9.89 秒通过记录。

## Web 与真实浏览器

`ReleaseOrdersPage.test.tsx` 和 `release-journal.test.ts` 共 69 项通过。新增场景覆盖可选原因的补填／修正／清空入口、每次历史、UTF-8 字节计数、未知响应不自动重试、原输入及同键正文手动恢复、另一窗口原请求晚到后不丢本窗口输入，以及编辑窗口打开后服务端撤销动作时禁止提交已有请求。生产构建通过。

`RCC_E2E_SUITE=rollback-reason ./scripts/browser-acceptance.sh` 使用真实 Chromium、同源 Web 代理、Admin 和一个隔离 MySQL 完成 3 个具名检查：[机器结果](2026-09-10-rollback-reason-history/browser-result.json)。实际执行人降为 VIEWER 后仍通过键盘打开窗口、输入并保存；390 px 文档无横向溢出，取消按钮初始聚焦，保存按钮可由 Tab 到达。管理员修正时丢弃已提交的成功响应，页面保留原输入且没有自动重试；再次点击以同一 key/body 找回结果。申请人、正向发布人和无关 VIEWER 在真实页面均没有编辑入口。

[桌面截图](2026-09-10-rollback-reason-history/rollback-reason-editor-desktop.png)、[桌面布局数据](2026-09-10-rollback-reason-history/rollback-reason-editor-desktop-layout.json)、[390 px 截图](2026-09-10-rollback-reason-history/rollback-reason-editor-390.png)和[390 px 布局数据](2026-09-10-rollback-reason-history/rollback-reason-editor-390-layout.json)已归档。两个视口均验证文档不横向溢出、长原因换行及保存操作可达。专项运行的数据库后置检查通过，临时容器、卷和端口清理均验证成功；最终原始日志位于 `/private/tmp/rcc-issue-87-evidence/browser-final-runner.log`。该脚本同时接入默认 `release-workflow/all`，排在已有发布／回滚脚本之后，并从真实查询读取当前记录版本，避免共享夹具的顺序假设。

首次浏览器脚本已读取到修正后的主单内容，但在等待并确认第二次 POST 响应前提前检查请求监听数组，以 `1 !== 2` 失败；脚本随后改为明确等待第二次 POST 响应。同一场景先在全新隔离库通过，再按 Standards 审查意见加入桌面布局／操作可达性断言并于另一全新隔离库通过。失败、第一次修复和最终复验分别保留在 `/private/tmp/rcc-issue-87-evidence/browser-runner.log`、`browser-green-runner.log` 与 `browser-final-runner.log`。

## 受影响工程检查

[证据清单](2026-09-10-rollback-reason-history/evidence-manifest.json)记录固定基点、实际命令、结果和外部原始日志 SHA-256。

- Admin 非集成 `go test ./... -count=1 -p 1`：6 个有测试包通过，4 个包没有测试文件；日志 `/private/tmp/rcc-issue-87-evidence/admin-unit.log`。
- Web 受影响测试：2 个文件、69/69 通过；日志 `/private/tmp/rcc-issue-87-evidence/web-tests.log`。
- Web `pnpm build`：TypeScript 检查及 Vite 生产构建通过；日志 `/private/tmp/rcc-issue-87-evidence/web-build.log`。
- `git diff --check`、浏览器脚本 `node --check` 与运行器 `bash -n` 通过。

实现没有迁移、旧接口、兼容适配、双写、Feature Flag、期限、提醒、必填原因、流程重开、再次执行、目标占用或独立未知结果确认。原因修改只追加主单审计 JSON 与幂等请求结果；执行人员、执行类型／时间／版本／结果及业务事实继续来自原成功执行。
