# 阶段 3：让规则更容易理解

- 日期：2026-09-07。
- 工作目录：`/private/tmp/rcc-management-work-package-20260907`。
- 分支：`codex/management-work-package-20260907`，阶段起点 `3f61e7e0bf8601bce29b248b94d35f42be0e4120`。
- 本阶段只修改 Web 与验证材料；没有修改 Admin、权限模型、账号/会话、并发版本或数据库数据。

## 已实现的行为

规则详情不再只陈列字段，而是直接说明运行结果：查询请求未指定排序或每页数量时怎样处理、最大每页数量是多少；变更规则分别说明新增、修改、删除是否允许，以及每个操作会由服务器填写哪些列。新增会填写已配置的创建列和修改列，修改只填写修改列，删除不自动填写。界面明确说明 Operator 来自部署配置，不是当前登录用户。

名称和描述与执行规则使用两个独立区块和操作名称。Draft 可进入“修改执行规则”；Active 和 Deprecated 只能进入“修改名称和描述”，保存按钮也明确写出保存对象。修改名称和描述不会改变执行内容。Active 或 Deprecated 若要改变排序、分页、操作授权或自动填写列，界面指引新建带新版本编码的草稿、激活，再替换表分配。Deprecated 详情说明它对已有分配仍有效，但不能用于新分配。

表规则创建和替换会在选择项下方预览两条规则的实际效果。创建分配明确说明创建后保持未启用；替换已启用分配明确说明成功后下一次数据请求立即使用新选择。选择项从刷新后的候选中消失时仍保留当前编码，不自动改选。只读详情始终使用最新表规则引用，编辑会话继续使用进入编辑后保存的候选选择。

配置内容管理页显示当前表规则能力，并列出本次请求由实时 Schema 确认的列。查询规则没有字段白名单；可查询、排序和编辑的列由实时 Schema、`id`、服务器自动填写列及现有值转换能力共同决定。原有按钮禁用原因和服务器校验继续生效。

未知规则类型、类型目录尚在加载或加载失败、规则详情或目录无法取得时，界面统一显示“无法确认”，不推测允许哪些操作。原有 fail-closed 行为和 Admin 最终验证保持不变。

## 自动化验证

共享 `PolicyEffect` 组件集中表达 Query 与 Mutation 效果，页面行为测试覆盖：

- Query 默认 `字段 + 方向`、默认与最大每页数量，以及没有字段白名单的边界；
- Mutation 三种操作授权、ADD / MODIFY / DELETE 的不同自动填写结果与 Operator 来源；
- 未知或未完整注册类型不显示“规则允许”，只显示无法确认；
- 新建和修改 Draft 显示执行效果预览；
- 表规则选择显示候选效果，创建后未启用的后果清晰；
- 表规则只读详情在引用刷新后跟随新编码的效果，编辑中的选择与效果保持不被刷新覆盖；
- Managed Data 同时展示规则效果与实时 Schema，并继续按实时不兼容原因禁用变更操作。

| 命令 | 结果 |
| --- | --- |
| `pnpm test:run`（web） | 17 个测试文件、130 个测试全部通过。 |
| `pnpm typecheck`（web） | 通过。 |
| `pnpm build`（web） | 通过。 |
| Impeccable detector | 对本阶段 UI 改动目标检查，0 条发现。 |
| `git diff --check` | 通过。 |

后端源码未修改，复用[阶段 1 验收](2026-09-07-stage1-acceptance.md)中 Go 全模块 test/build 与真实 MySQL integration 全部通过的结果，不重复执行昂贵的后端套件。

## 真实 Chromium + Admin + MySQL 验收

使用全新隔离的 headless Chromium 151.0.7922.34，连接本工作包的 Web、Admin 和 MySQL。验收脚本只读取既有规则和 fixture，不提交表单、不改变数据。六项检查全部通过，浏览器无 `pageerror`：

| 实测路径 | 结果 |
| --- | --- |
| Query 详情 | `notification_page_query_v1` 显示 `id` 降序、默认每页 20、最大 200，以及无字段白名单的实时 Schema 边界。 |
| 名称描述编辑 | Active Query 的显示名称可编辑，排序字段保持锁定，并明确说明执行内容不会改变。 |
| Deprecated Mutation | `stage1_mutation_v1` 的新增、修改、删除和四个自动填写列完整显示；说明 Operator 来自部署配置，且已有分配继续有效、不能用于新分配。 |
| 表规则详情与替换 | `stage1_acceptance_items` 显示两条已选规则的效果；进入替换后继续保留已 Deprecated 的当前编码，不自动改选，并预览当前选择。 |
| 配置内容管理 | 切换到 `stage1_acceptance_items` 后显示默认查询、三种变更能力、自动填写列和实时 Schema 列清单。 |
| 390×844 | 表规则详情无页面级横向溢出，抽屉内容、滚动和固定操作区可读可用。 |

机器结果为 [2026-09-07-stage3-browser.json](2026-09-07-stage3-browser.json)，窄屏截图为 [2026-09-07-stage3-rule-clarity-390.png](2026-09-07-stage3-rule-clarity-390.png)。可复跑脚本为 `web/e2e/rule-clarity.cjs`：

```sh
RCC_PLAYWRIGHT_MODULE=/absolute/path/to/node_modules/playwright \
  node web/e2e/rule-clarity.cjs
```

可覆盖 `RCC_WEB_URL` 与 `RCC_E2E_OUTPUT`。默认连接 `127.0.0.1:15173`，且全程只读。

## 验证范围与环境

- 组件和 data-router 测试验证刷新、编辑会话冻结、未知/失败状态和各页面组合；真实浏览器验证现有 Active Query、Deprecated Mutation、已启用表分配和真实 Schema，没有为制造错误状态写入规则目录。
- 真实环境仍使用 MySQL Compose project `rcc-stage1-20260907`，容器 `rcc-stage1-20260907-mysql-1`，宿主端口 `127.0.0.1:32768`。`stage1_acceptance_items` 沿用阶段 1 的 5 行 fixture，本阶段未执行写操作。
- Admin 已用现有 ignored `deploy/.env.stage1` 重启在 `127.0.0.1:18081`，unified exec session 为 `75606`；环境内容未输出、未修改、未提交。
- Web 继续使用原 `127.0.0.1:15173` 热更新会话 `46016`。曾因误判 sandbox 内 localhost 不可达而启动的 15174 临时会话已立即停止。
- 原 CUA/IAB 标签页没有操作；系统锁屏不影响独立 headless Chromium。

## 剩余限制

- 规则效果是对当前规则定义、类型目录和实时 Schema 的解释，不替代 Admin 在提交和每次数据请求上的最终校验。
- 390px 验收覆盖当前 fixture 的真实长编码和完整效果内容，没有穷举所有语言扩展或真实屏幕阅读器。
- commit、push、Issue 与 Notion 更新由父代理统一处理；本阶段没有执行这些操作。
