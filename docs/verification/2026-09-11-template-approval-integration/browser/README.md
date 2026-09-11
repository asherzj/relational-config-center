# 模板／审批依赖集成：真实浏览器验证

2026-09-11，在 `/private/tmp/rcc-release-template-approval-integration` 串行运行三个既有系统路径。每条都通过真实Chromium、同源Vite代理、Admin独立进程和新的MySQL 8.4容器。三条全部exit0，日志均包含容器完整终止；没有访问其他任务数据库，完成后已向root释放数据库时段。

| 测试 | checks | 用例 / package耗时 | 原日志 | 独立输出 |
| --- | --- | --- | --- | --- |
| TestApprovalRoleBrowserSystemPath | 12 | 48.08s / 50.287s | 01-roles.log | roles/ |
| TestTableApprovalBrowserSystemPath | 7 | 36.22s / 37.379s | 02-table-approvals.log | table-approvals/ |
| TestTableReleaseTemplateBrowserSystemPath | 9 | 21.98s / 22.898s | 03-table-templates.log | table-templates/ |

28项实际检查逐项来自运行产物JSON，汇总为 `runs.json`，没有把静态审阅算作系统检查。角色管理与表审批脚本的浏览器错误数组均为空；模板脚本的所有操作断言通过。

## 实际命令

从工作树根目录分别执行下列命令，stdout/stderr重定向至表中log，真实shell退出码存同名`.exit`。Docker socket和本地监听使用已授权的升级权限；三条之间没有重叠容器。

```sh
TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache RCC_E2E_OUTPUT=/private/tmp/rcc-release-template-approval-integration/docs/verification/2026-09-11-template-approval-integration/browser/roles go test -tags=integration,browser ./admin/cmd/admin -run '^TestApprovalRoleBrowserSystemPath$' -count=1 -timeout=10m -v
TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache RCC_E2E_OUTPUT=/private/tmp/rcc-release-template-approval-integration/docs/verification/2026-09-11-template-approval-integration/browser/table-approvals go test -tags=integration,browser ./admin/cmd/admin -run '^TestTableApprovalBrowserSystemPath$' -count=1 -timeout=10m -v
TESTCONTAINERS_RYUK_DISABLED=true DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock GOCACHE=/private/tmp/rcc-go-cache RCC_E2E_OUTPUT=/private/tmp/rcc-release-template-approval-integration/docs/verification/2026-09-11-template-approval-integration/browser/table-templates go test -tags=integration,browser ./admin/cmd/admin -run '^TestTableReleaseTemplateBrowserSystemPath$' -count=1 -timeout=10m -v
```

## 人工视觉核对

已实际查看角色桌面成员选择与390px目录操作；逐表审批桌面1/2表进度与390px范围冲突审阅；模板设置桌面目录及390px底部动作区。长内容在对应区域换行/局部滚动，操作可达。`scope-review-geometry-390.json` 的documentWidth为390，冲突区left16/right374、scrollWidth=clientWidth356。

`table-templates/integrated-management-directory.png` 明确显示“发布流程模板”“角色管理”“账号角色”导航，以及同一表的“查看”“字段配置”“发布流程”“审批角色”四入口。第三条脚本亦实际等待这些DOM入口可见；随后执行创建/启用、常规和应急关联管理、真实提交后中断响应、注销后同账号重新登录、原包手动重推、并发冲突输入保留、390px键盘/Escape和整表停用。

表审批检查保留真实VIEWER成员只审批自己的表、ADMIN无通用旁路、两表分别通过才能执行、成员变化后确认范围冲突、空集合独立ADMIN默认及合法历史资格来源。没有接入#103实例或新增应急执行。

## 源码与证据边界

本轮唯一源码修改是 `web/e2e/table-release-templates.cjs` 增加导航/四入口DOM断言与目录截图（`navigation-check.diff`）；编辑前已通知root，随后第三条整体运行验证。生产Go/Web没有修改。此前Web188项验证的快照保持在 `../web/source-sha256.json`；仅此脚本条目的最新值以本目录 `browser-inputs.sha256.json` 为准，不回写之前的运行快照。

`browser-inputs.sha256.json` 记录3个Go harness、脚本/helper及关键页面/构建输入共19项当前哈希，完整生产源码归整体集成记录。`evidence-sha256.json` 包含此目录全部产物且排除自身。所有产物均在本轮新目录，未覆盖历史#93/#94/#101/#102证据；原始日志/diff空白不修剪。本代理只按明确路径stage脚本及本目录证据，不commit/push；整体独立复核与merge由root完成。
