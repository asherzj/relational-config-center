# 阶段 2：防止编辑内容意外丢失

- 日期：2026-09-07。
- 工作目录：`/private/tmp/rcc-management-work-package-20260907`。
- 分支：`codex/management-work-package-20260907`，阶段起点 `4fcc8c8d250e769d3b45b51b07ab46afcfa01370`。
- 本阶段只改 Web 与验证材料；没有实现账号、自动保存、浏览器草稿持久化、自动重放写入或并发版本控制。

## 已实现的行为

Query / Mutation 的新建、编辑 Draft、只更新名称描述，以及 Table Policy 的创建与替换分配，均保护关闭按钮、取消、Escape、遮罩、页内资源或模式切换、主导航、浏览器返回与前进。用户选择继续编辑时保留原界面；确认放弃时执行原来的离开目标。

Managed Data 的编辑器与 Change Set 作为同一份内存草稿保留。查看 Change Set、返回修改不提示放弃；关闭或“放弃本次编辑”才触发离开保护。切换 Managed Table、打开另一条记录或改用其他操作也经过保护。DELETE 预览没有可编辑输入，明确显示“尚未执行删除”，提供“取消删除”直接退出；提交中不能退出。

提交中的规则输入及提交按钮被禁用，同步请求锁防止同一时刻重复写入。提交中触发的站内离开请求直接取消，不在写入成功后补执行旧导航。提示在请求完成后消失：成功进入详情或结果，失败保留草稿和带 Request ID 的错误。成功后的精确回查失败仍沿用阶段 1 的“已执行、回查未完成”结果，不自动重复写入。

刷新、关闭标签页及跨文档离开通过原生 `beforeunload` 提醒。配置字段值不写入 `localStorage`、`sessionStorage` 或 URL。

## Dirty 与初始化边界

- 规则表单比较本次进入编辑时的初始值与当前值；显示名称、描述及会在提交时裁剪的文本按裁剪后的值比较。Mutation Auto Fill 的空文本与 `null` 作为同一未填写状态。
- Table Policy 比较表名与两条规则引用；选择后恢复原分配即恢复干净状态。异步目录刷新使选项消失时保留当前选择的编码展示，不悄悄换成其他规则。
- 行编辑器比较字段输入、是否包含和 NULL 开关的完整初始状态。字段选择也代表写入意图，因此仅勾选一个字段也受保护；取消包含后仍保留已输入的值，直到明确放弃或恢复全部原状态。NULL、空字符串和省略保持不同语义。
- 编辑资源或模式变化才开启新的编辑会话。详情、状态或 Mutation Type Registry 后台刷新不会覆盖正在编辑的规则草稿；保存依然由 Admin 使用当前状态校验，可能拒绝过时的编辑。只读详情继续显示最新服务器数据。
- Managed Data 在开始编辑时固定表、Schema 和原行；查询刷新或表规则目录变化不会卸载编辑器与 Change Set。

## 实现与焦点

正式入口改用 React Router 的 `createBrowserRouter` / `RouterProvider`；现有页面路由仍由 `AppRoutes` 管理。共享 `LeaveProtectionProvider` 只有一个公共 `useBlocker`，覆盖 PUSH、REPLACE、POP，并接受页内状态切换的延后动作。没有访问路由私有 API，也没有改写浏览器 history。

离开确认框通过 portal 放在 DOM 与视觉最上层，复用阶段 1 的 `useModalFocus`；初始焦点为“继续编辑”，Tab / Shift+Tab 不离开顶层，Escape 只关闭提示，取消后恢复原焦点。`useModalFocus` 改用 `:disabled` 语义识别被整个 fieldset 禁用的输入。ConfirmDialog 在 pending 时同时禁用遮罩，保持阶段 1 的叠层焦点修复。

## 自动化验证

`web/src/test/UnsavedChanges.test.tsx` 新增 22 个行为用例，覆盖：

- Query / Mutation 六种表单模式的关闭、取消、Escape、遮罩、恢复原值与 `beforeunload`；
- 真正 data router 的 history 返回/前进、拒绝与确认导航、资源/模式切换及主导航；
- 六种规则模式的同步重复提交、pending 离开拒绝、失败保留、成功清除和忙碌提示自动退出；
- 只读刷新显示新值，编辑时刷新详情、状态与类型目录保留原稿；
- Table 创建/替换的选项保留、恢复原值、刷新、失败、成功，以及叠层确认的 pending 遮罩/Escape；
- 行编辑器与 Change Set 往返，NULL、省略及临时未包含输入的保留，表目录刷新和切表确认；
- 行写入 pending 防重复、防取消，失败保留错误和输入，成功显示结果且取消的导航不再执行；
- 恢复行控件初始值不提示，DELETE 预览明确取消且不发 DELETE。

`ModalFocus.test.tsx` 另增加 1 个 disabled fieldset 的 Tab 回归用例。原 Managed Data 回查测试补上切表时的“继续编辑”，避免测试绕过新确认框点击底层按钮。

| 命令 | 结果 |
| --- | --- |
| `pnpm test:run`（web） | 17 个测试文件、129 个测试全部通过。 |
| `pnpm typecheck`（web） | 通过。 |
| `pnpm build`（web） | TypeScript no-emit 与 Vite production build 通过，1975 个模块。 |
| `git diff --check` | 通过。 |

后端源码未修改，复用[阶段 1 验收](2026-09-07-stage1-acceptance.md)中 Go 全模块 test/build 与真实 MySQL integration 全部通过的结果，没有重复昂贵的后端全套测试。

## 真实 Chromium + Admin + MySQL 验收

使用全新隔离的 headless Chromium 151.0.7922.34，连接阶段 1 的真实 Web、Admin 和 MySQL；不操作用户已有浏览器或系统锁屏。默认桌面视口 1440×1000，窄屏 390×844。脚本为 [`web/e2e/unsaved-changes.cjs`](../../web/e2e/unsaved-changes.cjs)，14 项检查全部通过，没有浏览器 `pageerror`。

| 实测路径 | 结果 |
| --- | --- |
| 空白 Query 新建 → Escape | 正常关闭，不提示。 |
| 输入后实际浏览器返回 → 继续编辑 / 放弃 | 取消保留输入和 URL；放弃回到原列表；再次前进是新的干净表单。 |
| 实际浏览器前进 → 继续编辑 / 放弃 | 取消保留原界面，确认进入原前进目标。 |
| 原生刷新与关闭标签页 | 均触发浏览器 `beforeunload` 对话框；取消后文档和输入保留。 |
| 关闭 → 确认框键盘与 390px | 初始焦点、正反向 Tab 循环、Escape 均通过；框体完整位于视口内，截图已人工检查。 |
| 输入恢复为空 → 取消 | 无未保存提示，正常回列表。 |
| 新建 Query 使用已有规则编码 | 真实 Admin 拒绝；编码与名称保留，可改后重试。 |
| 新建 Query 真实写入完成、仅延迟响应交付 | 测试拦截器先执行真实 `route.fetch()`，再延后响应到 UI；此时返回键被拒绝，无放弃按钮。放行响应后进入已创建详情，提示自动消失，只发出一次写入。临时 Query Draft 随后经 Admin DELETE 清理。 |
| ADD 编辑器 → Change Set → 放弃后继续编辑 → 返回修改 | 名称、NULL、空字符串与省略字段完整保留。 |
| ADD 使用非法 ENUM | 真实 MySQL 校验失败，经 Admin 返回字段 Schema 错误与 Request ID；Change Set 和返回后的编辑器均保留错误、非法值和选择。 |
| 确认放弃 ADD | 编辑器关闭，localStorage / sessionStorage 都为空。 |

测试后经真实 Admin 只读回查，`stage1_acceptance_items` 仍为 5 行，`stage2_unsaved_query_` 前缀的临时规则为 0 条。

机器结果保存在 [2026-09-07-stage2-browser.json](2026-09-07-stage2-browser.json)。窄屏截图：[2026-09-07-stage2-discard-390.png](2026-09-07-stage2-discard-390.png)。本机原始截图与结果目录为 `/tmp/rcc-stage2-browser`，包含 `failed-row-retained.png`。

复跑方式（先启动隔离环境；Playwright 可以使用已安装或官方 bundled runtime 的包）：

```sh
RCC_PLAYWRIGHT_MODULE=/absolute/path/to/node_modules/playwright \
  node web/e2e/unsaved-changes.cjs
```

可覆盖 `RCC_WEB_URL`、`RCC_E2E_OUTPUT`、`RCC_E2E_TABLE`。默认沿用本轮 `127.0.0.1:15173` 与 `stage1_acceptance_items`。脚本的写入目标是隔离 fixture 与自己创建的临时 Query Draft；不要对生产环境运行。

## 限制与复用环境

- 原生离开提示由浏览器决定文案与显示条件；浏览器/系统强制终止、移动系统回收进程等不能保证触发 `beforeunload`。用户在原生提示中明确允许离开后，内存草稿不恢复；正在执行的服务器请求也不会被自动重放或被当作已撤销。
- 实际浏览器验证使用一个 Chromium 版本；三类规则各模式、Table 写入失败/成功与 Managed Data pending 的完整组合由组件/data-router 行为测试覆盖，并非每一组合都在实际浏览器重复执行。
- 隔离 Admin 仍是本机 auth-disabled 模式，认证与部署边界沿用阶段 1 记录。
- 阶段 1 的 MySQL project `rcc-stage1-20260907`、数据库端口 `127.0.0.1:32768`、Admin 18081（session 74802）、Web 15173（session 46016）继续可用；ignored `deploy/.env.stage1` 未修改、未提交。原 CUA/IAB 标签页未操作。
- commit、push、Notion 更新由父代理在复核后统一执行；本阶段代理没有提交或更新远端状态。

## 改动文件

- 共享保护/路由：`web/src/components/ui/LeaveProtection.tsx`、`web/src/app.tsx`、`web/src/main.tsx`、`web/src/styles.css`。
- 焦点与叠层：`web/src/components/ui/ConfirmDialog.tsx`、`useModalFocus.ts`、`ModalFocus.test.tsx`。
- 规则表单：Query / Mutation 的 `PolicyDrawer.tsx`、`PolicyForm.tsx`，`web/src/features/policies/lifecycle.ts`，`web/src/features/table-policies/TablePolicyDrawer.tsx`。
- 数据草稿：`ManagedDataPage.tsx`、`ManagedRowEditor.tsx`、`ChangeSetDialog.tsx`、`mutation-workflow.ts`。
- 验证：`web/src/test/TestRouter.tsx`、`UnsavedChanges.test.tsx`、六个现有页面测试的 data router harness，`web/e2e/unsaved-changes.cjs`，本报告、机器结果与窄屏截图。
