# 模板／审批依赖集成：Web 合并验证

本子任务只合并 #102 与外部 #94（含 #93）的既有 Web 能力，不实现 #103 节点实例或应急执行。工作树 `/private/tmp/rcc-release-template-approval-integration`，分支 `codex/release-template-approval-integration`；ours=`7eb9dd36affe16739d8e8b178afdd9977d4976a3`，theirs=`2612bb354ef71d42e193648564b9f30bd9281eb4`，merge-base=`dafdde3a4a41a2b8afd03ae57fa51b9908fae08b`。已完整读取 #93/#94 正文、评论及标签、AGENTS、设计约定、当前 DESIGN 和集成审计，逐块解决冲突，没有整文件选单侧。

## 冲突决策

| 文件 | 合并结果 |
| --- | --- |
| `web/DESIGN.md` | 主导航同时保留发布流程模板、角色管理及账号角色；表审批、逐表资格与模板关联设计段落均保留。 |
| `TablePoliciesPage.tsx` | 行内同时保留查看、字段配置、发布流程、审批角色；`fields`、`templates`、`approvals` 各自独立抽屉，默认普通表规则抽屉。管理入口仍仅 ADMIN。 |
| `TablePoliciesPage.test.tsx` | 保留 #102 原请求重推及读取/写入互斥回归；保留 #94 的角色分页选择与审批冲突／原请求恢复；旧外部分支的只读确认测试按已交付 #102 契约改为手动原样重推后读取当前目录，继续断言真实停用显示与写次数。新增导航及四入口共存断言。 |
| `release-batches.cjs` | 同时保留启用时 `expected_version: "1"` 与 VIEWER 账号加入真实审批角色的 fixture。 |
| `table-approvals.cjs` | 适配新建表后启用的版本1；原 helper 已携 Idempotency-Key，不增加隐式业务重试。 |

自动合并审阅：`AppShell` 与 `app` 的角色/模板导航均存在；客户端保留所有模板、关联、表规则错误以及 `release_approver_unavailable` 的确定拒绝分类；错误文案合并。发布单审批沿服务端 `approval_context` 与 `allowed_actions`，原请求保持 `confirmed_tables` 和 `expected_approval_revision`，不恢复全局 APPROVER 或无条件 ADMIN 旁路。`release-multitable`、`write-recovery` 保留外部角色 fixture 与本任务表规则版本/原包重推。冲突审阅窄屏换行样式保留。

`preserved-source.json` 逐字节证明10个关键实现仍与对应交付父提交相同：本任务表规则/关联Drawer、API和查询，以及外部角色页、审批分配、审批窗口、逐表进度、请求journal和write hook。无需改写这些已交付行为来解决合并。

## 运行与原始结果

全部命令从新工作树根目录执行。未启动 MySQL 或浏览器后端，未改历史验收目录，未改业务Go/schema/其他docs。

| 运行 | 结果 |
| --- | --- |
| 01 `pnpm --dir web install --offline --frozen-lockfile` | exit1：缺少锁定的 lucide-react tarball，原始输出保留。 |
| 02 `pnpm --dir web install --frozen-lockfile` | 升级网络权限下载缺包，exit0；package/lockfile无变更。 |
| 03 `pnpm --dir web typecheck` | exit1：改写测试误用了仅Playwright支持的 `exact` 选项。 |
| 04 相关组件/API十文件 | 187通过，1失败：测试匹配两个关闭按钮；原失败未改记全绿。 |
| 05 `pnpm --dir web typecheck` | 修复测试定位后exit0。 |
| 06 单独重跑 TablePoliciesPage | 18项全部通过（5.98s）；其余9文件170项复用04且源码/测试未改变，总计188项有效通过。 |
| 07 `pnpm --dir web build` | 最终类型检查和生产构建exit0。 |

运行04的完整命令：

```sh
pnpm --dir web exec vitest run src/api/client.test.ts src/api/table-policies.test.ts src/api/release-orders.test.ts src/features/accounts/WorkspaceAccess.test.tsx src/features/account-roles/AccountRolesPage.test.tsx src/features/table-policies/TablePoliciesPage.test.tsx src/features/table-policies/TableReleaseTemplatesDrawer.test.tsx src/features/release-templates/ReleaseTemplatesPage.test.tsx src/features/release-orders/ReleaseOrdersPage.test.tsx src/features/release-orders/release-journal.test.ts
```

运行06：

```sh
pnpm --dir web exec vitest run src/features/table-policies/TablePoliciesPage.test.tsx
```

`script-syntax.json` 保存29个e2e `.cjs/.mjs` 的逐命令exit0和空输出；这只证明语法，不宣称运行浏览器。组件保留并实际通过 VIEWER 真资格、ADMIN/旧APPROVER无资格拒绝、审批范围冲突保留、IndexedDB原请求、同账号恢复，以及表关联版本/不确定结果保护。当前切片仍须由root验证整合后的后端/真实数据库和浏览器。

## 交接与清单

`final-ours.diff` / `final-theirs.diff` 为最终Web相对两侧父提交的完整差异。`source-sha256.json` 涵盖两侧Web差异涉及的全部文件与构建清单；`evidence-sha256.json` 涵盖此目录产物，自身除外。原始log/diff保留空白；源码单独执行空白检查。

本代理只按明确路径stage解决项及新证据，未commit/push；由root完成整个依赖合并、独立复核与后续交付。
