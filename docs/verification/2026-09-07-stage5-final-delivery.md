# 阶段 5：最终回归与 Issue #22 交付核对

- 日期：2026-09-07
- 工作分支：`codex/management-work-package-20260907`
- 本阶段起点：`f9eb752a52c50ff7c0f7d7bbb0e48b4833bbc988`
- 规格来源：[GitHub Issue #22](https://github.com/asherzj/relational-config-center/issues/22)，本阶段只读

## 交付结论

Issue #22 的 43 条用户故事均有正式 Web 实现、自动化验证或真实 Admin + MySQL + Chromium 验收支撑，本阶段复核后没有未完成的规格项。

最终组合回归在阶段 3 的内容、按钮和布局上重新执行：未保存编辑与失败保留脚本 14/14 通过，规则效果脚本 6/6 通过，Chromium 均无 `pageerror`。验收后临时 Query Draft 为 0，`stage1_acceptance_items` 仍为 5 条种子数据。

本阶段发现并修复一处交付文案缺口：规则类型未知、注册表未确认或能力不完整时，列表原来统一显示“仅可查看”，但 Active / Deprecated 规则实际仍允许修改名称和描述。现在提示由和操作按钮相同的生命周期规则计算：Draft 显示“仅可查看”，Active / Deprecated 显示“仅可修改名称和描述”，执行规则修改、激活、弃用和删除仍保持 fail-closed。针对测试 2 个文件 23 项、Web 全量 17 个文件 132 项、typecheck 和 production build 均通过。

## 本阶段实际执行与复用结果

| 检查 | 本阶段结果 | 说明 |
| --- | --- | --- |
| 环境健康 | Admin live 200、ready 200；Web 200 | 父代理从隔离环境实际读回；MySQL 为 `rcc-stage1-20260907-mysql-1`，宿主端口 `32768` |
| `web/e2e/unsaved-changes.cjs` | 14/14 通过，0 页面错误 | 全新 headless Chromium；真实创建并清理临时 Query Draft；真实 MySQL 拒绝非法 ENUM 后保留草稿与错误 |
| `web/e2e/rule-clarity.cjs` | 6/6 通过，0 页面错误 | 规则效果、名称描述编辑边界、Deprecated 既有分配、表分配预览、实时 Schema、390px |
| 验收后只读回查 | 临时 Query Draft 0；fixture 5 行 | API 返回 Query Codes 仅 `notification_page_query_v1`、`stage1_query_v1`；fixture ids 为 1–5 |
| 列表能力提示针对测试 | 2 文件、23 项通过 | Query / Mutation 未确认类型的提示与按钮一致 |
| `pnpm test:run` | 17 文件、132 项通过 | 最终 Web 源码全量回归 |
| `pnpm typecheck` | 通过 | TypeScript no-emit |
| `pnpm build` | 通过 | Vite production build，1976 modules；500 kB chunk 提示不影响构建通过 |
| `git diff --check` | 通过 | 最终执行见交付前检查 |

没有重复阶段 1 的约 10 分钟真实 MySQL integration。`git diff 4fcc8c8..f9eb752 -- admin` 为空，本阶段也没有 Admin 修改，因此复用阶段 1 的 `make test`、`make build` 和完整 MySQL 8.4 integration 通过结果。阶段 1 完整 Admin integration 中 `admin/cmd/admin` 耗时 587.672 秒。

两组浏览器回归实际运行时的 HEAD 为 `89ea077`。随后同步最新 `main` 的 merge commit `f9eb752` 处理了 #41 视觉基线的重叠历史；父代理核对 `89ea077..f9eb752` 的 `web admin client server shared` 产品树差异为空，因此机器结果另记 `f9eb752` 为产品树等价 merge HEAD。同步后父代理另跑 Web 130 项、typecheck 与 build 通过；本阶段文案修复后再得到上述 132 项最终结果。

## 证据索引

| 代号 | 证据 |
| --- | --- |
| M | [`MutationPoliciesPage.test.tsx`](../../web/src/features/mutation-policies/MutationPoliciesPage.test.tsx)、[`model.test.ts`](../../web/src/features/mutation-policies/model.test.ts)、[`mutation-policies.test.ts`](../../web/src/api/mutation-policies.test.ts) |
| T | [`TablePoliciesPage.test.tsx`](../../web/src/features/table-policies/TablePoliciesPage.test.tsx)、[`table-policies.test.ts`](../../web/src/api/table-policies.test.ts) |
| Q | [`ManagedDataPage.test.tsx`](../../web/src/features/managed-data/ManagedDataPage.test.tsx)、[`managed-data.test.ts`](../../web/src/api/managed-data.test.ts) |
| W | [`ManagedDataMutationPage.test.tsx`](../../web/src/features/managed-data/ManagedDataMutationPage.test.tsx)、[`model.test.ts`](../../web/src/features/managed-data/model.test.ts)、[`mutation-workflow.test.tsx`](../../web/src/features/managed-data/mutation-workflow.test.tsx) |
| L | [`lifecycle.test.ts`](../../web/src/features/policies/lifecycle.test.ts)、[`UnsavedChanges.test.tsx`](../../web/src/test/UnsavedChanges.test.tsx)、[`ModalFocus.test.tsx`](../../web/src/components/ui/ModalFocus.test.tsx) |
| F | [`local_fixture_integration_test.go`](../../admin/cmd/admin/local_fixture_integration_test.go)、[`002-notification-templates.sql`](../../deploy/mysql/local-fixture/002-notification-templates.sql) |
| E1 | [阶段 1 真实全流程验收](2026-09-07-stage1-acceptance.md) |
| E2 | [阶段 2 未保存保护验收](2026-09-07-stage2-unsaved-changes.md) |
| E3 | [阶段 3 规则解释验收](2026-09-07-stage3-rule-clarity.md) |
| E5 | [阶段 5 机器结果](2026-09-07-stage5-browser.json)、[未保存确认 390px](2026-09-07-stage5-discard-390.png)、[失败行草稿保留](2026-09-07-stage5-failed-row-retained.png)、[规则效果 390px](2026-09-07-stage5-rule-clarity-390.png) |

## 43 条用户故事核对

| # | 验收点 | 实现与自动化证据 | 真实验收 | 结论 |
| ---: | --- | --- | --- | --- |
| 1 | 主导航进入 Mutation Policy | `AppShell` 正式路由；M 路由测试 | E1 | 通过 |
| 2 | Mutation 目录含编码、名称、类型、权限、Auto Fill、状态、审计 | 列表与详情；M 目录/详情测试 | E1、E3 | 通过 |
| 3 | 查看 Mutation Type 与操作能力 | Type Registry；M 契约与页面测试 | E1 创建流程使用；完整目录能力由 M 验证 | 通过 |
| 4 | 创建 Mutation Draft | 完整创建 DTO；M | E1 | 通过 |
| 5 | 编辑 Draft 完整执行规则 | Draft replace；M | 未单独实跑；完整行为由 M 验证 | 通过 |
| 6 | 激活 Draft | 生命周期确认；M、L | E1 | 通过 |
| 7 | 弃用 Active 并保留既有分配 | 生命周期门禁与确认；M、L | E1、E3 | 通过 |
| 8 | Active / Deprecated 只改名称描述 | 元数据专用模式；M、L | E3、E5 实跑编辑边界；更新请求由 M 验证 | 通过 |
| 9 | 确认后删除 Draft | 共享生命周期确认与 DELETE client；L | 未单独实跑 Mutation 删除；E2 仅实跑 Query 临时 Draft 的 API 清理 | 通过 |
| 10 | 配置 ADD / MODIFY / DELETE | Mutation 表单与 DTO；M | E1、E3 | 通过 |
| 11 | 四个标准 Auto Fill 目标 | 表单、规则效果、快照执行；M | E1、E3 | 通过 |
| 12 | 提前拒绝重复、非法、权限冲突目标 | Web `validateDraft`；M | 未单独实跑前端全边界；完整行为由 M 验证 | 通过 |
| 13 | 主导航进入 Table Policy | `AppShell` 正式路由；T | E1 | 通过 |
| 14 | Database Table Discovery 分类 | Discovery 页面与契约；T、Admin integration | E1 实跑兼容未分配表；完整分类由 T 与 integration 验证 | 通过 |
| 15 | 仅兼容未分配真实表可创建 | 创建候选过滤；T | E1 | 通过 |
| 16 | 新分配只选 Active Query / Mutation | 两类 Catalog 过滤；T | E1 实际选择 Active；状态过滤由 T 验证 | 通过 |
| 17 | 查看引用、启用状态、审计 | 目录与详情；T | E1、E3 实跑引用和启用状态；完整审计字段由 T 验证 | 通过 |
| 18 | 原子替换两个 Policy Code | 单次 PUT 完整引用；T、Admin integration | E1 | 通过 |
| 19 | enabled 替换前提示下次请求立即生效 | 顶层确认框；T | E1、E3 | 通过 |
| 20 | 启停并呈现实时 Schema 错误 | enable/disable 与稳定错误；T | E1 实跑启停；启用时 Schema 错误由 T 与 Admin integration 验证 | 通过 |
| 21 | 客户端筛选完整 Table Policy Catalog | 浏览器内 filter；T | 未单独实跑筛选；完整行为由 T 验证 | 通过 |
| 22 | 主导航进入 Managed Data | `AppShell` 正式路由；Q | E1 | 通过 |
| 23 | 只列 enabled Table Policy | Managed Table 候选过滤；Q | E1 | 通过 |
| 24 | 八种 Query Operator | API 契约与页面组合；Q | E1 | 通过 |
| 25 | 最多 20 个 AND 条件与单字段排序 | 浏览器边界验证；Q | E1 实跑条件与排序；20 条上限由 Q 验证 | 通过 |
| 26 | 服务端分页、总数、前后页、每页数量 | Query Spec 与分页 UI；Q | E1 | 通过 |
| 27 | 动态列原名、类型、NULL 状态 | response contract 与动态表头；Q | E1、E3 | 通过 |
| 28 | 按 Mutation Policy 显示操作按钮 | 实时快照能力计算；W | E1、E3 | 通过 |
| 29 | 未授权操作可见、禁用并解释 | `capabilityReasons` 与 aria/tooltip；W | 未单独实跑未授权快照；完整行为由 W 验证 | 通过 |
| 30 | 通用字段编辑器表达省略、NULL、空串、字符串 | editor、API DTO、Change Set；Q、W | E1、E5 | 通过 |
| 31 | id 不进入写入表单 | editor 排除 id / Auto Fill；W；Admin 报告非自增 id 失败 | E1 实跑隐藏 id 的自增表；非自增失败由 Admin integration 验证 | 通过 |
| 32 | ADD 全字段 Change Set | 完整列模型与状态文字；W | E1 | 通过 |
| 33 | MODIFY 全字段且只突出变化 | diff 模型与渲染；W | E1 | 通过 |
| 34 | DELETE 全原值标红到不存在 | DELETE 模型与渲染；W | E1 | 通过 |
| 35 | 三类写入共用一次最终确认 | 共享 `ChangeSetDialog`；W | E1 | 通过 |
| 36 | 文字与配色共同表达四种状态 | 表头、变化标记、NULL / `""` / 未提交 / 不存在；W | E1 | 通过 |
| 37 | 失败保留编辑器与 Change Set、稳定错误和 Request ID | 失败状态不清空；W、L | E1、E2、E5 | 通过 |
| 38 | ADD / MODIFY 后 exact-id 回查最终值 | 写一次、独立回查与可重试回查；W | E1 | 通过 |
| 39 | DELETE 后显示摘要 | 成功结果组件；W | E1 | 通过 |
| 40 | 本地环境含示例表、数据、Active Policies、enabled 分配 | Compose local fixture；F | E1 | 通过 |
| 41 | fixture 幂等且仅本地使用 | 二次应用、冲突 fail-closed、生产路径隔离、`rcc_*` 不泄露；F | 阶段 1 integration（非浏览器） | 通过 |
| 42 | 抽屉、确认框、禁用原因、Change Set 的语义与焦点 | modal focus trap、初始/恢复焦点、aria；L | E1、E5 实跑焦点路径；禁用原因语义由 W 验证 | 通过 |
| 43 | 窄屏导航、表格、筛选器、抽屉、Change Set 可操作 | 响应式 CSS 与组件布局测试 | E1、E3、E5 分路径实跑；完整组合由页面组件验证 | 通过 |

## GitHub、Git 与项目记录状态

本阶段读取 #22 时，Issue 为 `OPEN`，标签为 `ready-for-agent`，没有评论。当前继续保持 OPEN 是合理的：完整 Web 基线已在 `main`，但本工作包中的非法 ENUM 稳定错误修复、焦点与未保存保护、规则解释和本阶段列表文案修复尚未全部合入 `main`。待该工作分支经维护者评审并合入后，应在 #22 留下交付与验证链接并关闭 Issue；本阶段没有评论、改标签、关闭 Issue 或创建 PR。

工作包提交如下：

| Commit | 内容 | 远端状态 |
| --- | --- | --- |
| `4fcc8c8` | 导入并完成管理流程；修复叠层焦点和非法 ENUM 错误分类；真实全流程验收 | 已推送工作分支 |
| `3f61e7e` | 未保存保护、提交态防重与失败保留；14 项真实浏览器验收 | 已推送工作分支 |
| `2e25685` | 规则效果、名称描述/执行规则边界、表分配与实时 Schema 说明 | 已推送工作分支 |
| `89ea077` | PM-006 运行与验收文档同步 | 已推送工作分支 |
| `f9eb752` | 合并最新 `main` 的 #41 视觉基线历史，保留相同最终产品树 | 已推送工作分支 |
| 本报告所在提交 | 列表能力提示、132 项最终回归、机器结果、截图与本报告 | 提交记录见远端工作分支 |

验收快照中的远端 `main` 为 `e032e53`（合并 #41）。`f9eb752` 及前四阶段提交位于远端工作分支，尚未合入 `main`；不能表述为已部署或 CI 已通过。Notion 的 PM-054、PM-055、PM-056、PM-006 与 PM-053 已由父代理更新。父代理在最终验收后提交、推送本报告并更新项目记录；最终远端状态可在[工作分支](https://github.com/asherzj/relational-config-center/tree/codex/management-work-package-20260907)与[Notion 项目页面](https://app.notion.com/p/3ca8544cc98980559f27e17f2c94cbaf)核对。

## 限制

- 真实浏览器验收使用隔离的 auth-disabled Admin 和 Chromium 151；生产 Bearer Token、精确 CORS 与同源代理边界由代码测试和阶段 4 preview probe 支撑，没有声称完成生产部署验证。
- 草稿只保存在当前页面内存；浏览器或系统强制终止、用户确认原生离开后不会恢复。提交前并发刷新、冲突合并和版本控制仍按 #22 明确排除，写入维持 last-write-wins。
- Web build 有现存的单 chunk 超过 500 kB 提示；production build 成功，这不构成 #22 的功能缺口。
