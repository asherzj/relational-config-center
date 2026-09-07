# 可靠性工作包阶段 3：真实页面操作覆盖

- 日期：2026-09-07
- 分支：`codex/management-reliability-20260907`
- 阶段起点：`93e6436d776d324f148f6472c6e2584bc2db7cb6`
- 最终工件：`../../../.records-reliability-20260907/stage3-final-operation-coverage-2/`

## 结论

新增 `operation-coverage` 浏览器套件，以真实 Chromium 页面驱动生产构建后的 Web，同源代理携带 Bearer Token 请求 Admin，并由 Admin 读写每次运行独占的 MySQL 8.4。最终 20 项业务断言全部通过，覆盖旧 43 故事矩阵中原来只有组件或 API 清理证据的 5、8、9、12、16、21、25、29。

本阶段发现并修复两个同源的真实页面缺陷：Web 原先只按大小写敏感字符串检查重复 Auto Fill 目标和主键 `id`。用户输入 `created_by`/`CREATED_BY` 或 `ID` 时，页面会发出 PUT，再由 Admin 返回 422；这与 MySQL 字段及 Admin 的大小写无关校验不一致。Web 现在先把受限 ASCII 字段名转成小写，再检查主键和重复目标，在页面内保留输入并阻止请求。新增单元回归后当前共享 Web 树完整测试 20 个文件、162 项通过。

## 覆盖结果

| 故事 | 真实页面动作 | HTTP 与 SQL 结果 |
| --- | --- | --- |
| 5 | 页面创建 Mutation Draft；进入执行规则编辑，改变 ADD/MODIFY/DELETE 和四个 Auto Fill；保存后刷新重开 | POST 201、PUT 200；SQL 完整值为 `1/1/1 + created_by/created_at/updated_by/updated_at + DRAFT` |
| 8 | Query、Mutation 分别在 Active 和 Deprecated 状态编辑名称与描述，保存后刷新重开 | 四次 PATCH 均为 200；每次 SQL 对比都确认 Type、排序/分页、授权、Auto Fill、状态未被元数据写入改变 |
| 9 | 从 Mutation Draft 详情点击删除并在可见确认框确认 | DELETE 204；SQL count 为 0；成功后返回目录 |
| 12 | 实际输入权限冲突、非法列名、大小写不同的 `ID` 主键、完全重复和大小写不同的重复 Auto Fill | 五种错误都保留输入；修复后只产生一次成功 PUT。`ID` 和大小写重复的修复前证据均为 PUT 422，修复后不发 PUT |
| 16 | 页面实际弃用已分配 Query/Mutation，再打开新分配抽屉 | 新候选包含 Active `stage3_query_full_v1`、`stage3_mutation_denied_v1`、`stage3_mutation_full_v1`；不含对应 Draft 和 Deprecated 编码 |
| 21 | 对完整 Table Policy Catalog 按表名、Query Code、Mutation Code、修改人筛选 | 覆盖首尾空白、大小写、无匹配和清空；筛选期间写请求为 0 |
| 25 | 页面逐个添加并填写 20 个相同 exact 条件后查询 | POST 200，request body 恰有 20 个 AND 条件，MySQL 对应 AND 查询 count 为 1；第 20 个时添加按钮禁用，删除后可再添加，未出现 21 条请求 |
| 29 | 使用三项均拒绝的 Mutation Policy 打开已有行页面 | ADD/MODIFY/DELETE 按钮全部可见、禁用，`title` 和页面说明均给出对应未授权原因；页面实际替换为全授权 Policy 后，下一页面三项启用且 ADD 201、SQL count 为 1 |

额外验证了生命周期两侧的业务边界：通过页面把既有分配引用的 Query 与 Mutation 从 Active 弃用后，下一次真实查询仍为 200，下一次 ADD 仍为 201，数据库 Auto Fill 的 Operator 为 `browser-acceptance`。这与“Deprecated 对既有分配继续执行、不可用于新分配”的约定一致。

## Runner、隔离与证据

`scripts/browser-acceptance.sh` 现在显式接受 `all`、`write-recovery`、`operation-coverage`。默认 `all` 依次纳入原有套件和新套件；指定 `operation-coverage` 时只运行本阶段套件；未知值仍以状态 2 在创建 Docker 资源前拒绝。

最终命令：

```bash
RCC_E2E_SUITE=operation-coverage \
RCC_E2E_ARTIFACTS=/Users/asher/Projects/relational-config-center/.worktrees/.records-reliability-20260907/stage3-final-operation-coverage-2 \
scripts/browser-acceptance.sh
```

结果：

- Chromium `151.0.7922.34`，20 项通过，`pageErrors=[]`；每个新页面都注册独立 `pageerror` 捕获。
- Admin 直连无 Token 为 401，Web 同源代理为 200。
- `fixture-before.tsv` 与 `fixture-after.tsv` 逐字节相同，覆盖 `stage1_acceptance_items` 的全部字段和 NULL/空值表达。
- `stage3_%` Table Policy、Query Policy、Mutation Policy 和物理表最终 count 全为 0。
- 独占容器、volume、Admin 监听和 Web 监听清理通过；`run.txt` 为 `exit status: 0`、`cleanup verified: true`。
- 工件密钥扫描通过；持久工件没有保存本次生成的 Admin Token、MySQL 用户密码或 root 密码。
- 关键工件：`operation-coverage/result.json`、`operation-coverage/http-evidence.json`、`operation-coverage/runner.log`、`operation-coverage/mutation-draft-reopened.png`、`operation-coverage/twenty-and-conditions.png`、`operation-coverage/unauthorized-controls.png`、`auth-boundary.txt`、`database-postcheck.txt`、`fixture-before.tsv`、`fixture-after.tsv`、`run.txt`。

## 红灯与修复依据

大小写重复 Auto Fill 的修复前工件保存在 `../../../.records-reliability-20260907/stage3-red-auto-fill-4/`。其中 `http-evidence.json` 显示页面向 `/api/v1/mutation-policies/stage3_ui_mutation_v1` 发出包含 `created_by` 和 `CREATED_BY` 的 PUT，Admin 返回 422；`failure-body.txt` 显示服务端错误出现在仍保留输入的草稿中。紧凑单元回归在 `web/src/features/mutation-policies/model.test.ts`，最终真实页面回路证明同一输入不再发 PUT，修正为 `updated_by` 后 PUT 200 并完整持久化。

大小写不同主键的修复前工件保存在 `../../../.records-reliability-20260907/stage3-red-id-auto-fill/`。`http-evidence.json` 显示页面发出 `create_time_field: "ID"` 的 PUT，Admin 返回 422；`result.json` 同时确认 `pageErrors=[]`、fixture 全字段相同及所有 stage3 资源清零。最终页面回路等待 `#createTimeField-error` 本次出现后，确认输入仍为 `ID` 且 PUT 数未增加。大小写重复场景同样先确认旧字段错误已清除，再等待本次两个重复错误出现后断言 PUT 数，套件中不使用固定等待时间猜测请求是否发生。

另一个失败工件 `../../../.records-reliability-20260907/stage3-green-operation-coverage-2/` 留给阶段 4：当合成表使用下面的时间列并把它配置为 Auto Fill 目标时，真实 Managed Data query 返回 `invalid_policy_snapshot` 422：

```sql
created_at datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
```

这最小化到了已知的 `DEFAULT_GENERATED` 与真正 `STORED/VIRTUAL GENERATED` 列识别边界。阶段 3 没有越界修改它；最终阶段 3 fixture 改为无默认的可写 `datetime(6)`，seed 显式提供时间值。失败工件中的 HTTP request ID、Admin 日志、截图和清理结果均保留，可供阶段 4 直接复用。

阶段 2 的共享提交保护修复进行中时，本套件还捕获过 Mutation Draft DELETE 已返回 204、SQL 已删除，但成功导航被旧 pending 状态拦截的页面红灯，工件位于 `stage3-green-operation-coverage/`。共享生命周期修复增加成功提交结束信号后，最终套件证明页面删除和目录终点均正常；该修复及其回归归入阶段 2，不在本阶段重复归属。

## 其他验证

| 检查 | 结果 |
| --- | --- |
| `pnpm --dir web test -- --run` | 当前共享 Web 树 20 文件、162 项通过；需允许本地 loopback 供 TCP body 中断测试监听 |
| `pnpm --dir web build` | 通过；TypeScript no-emit 与 production Vite build 完成，1977 modules |
| Mutation Policy model 定向回归 | 5 项通过，含大小写不同重复 Auto Fill 及 `ID`/`Id` 主键 |
| `node --check web/e2e/operation-coverage.cjs` | 通过 |
| `bash -n scripts/browser-acceptance.sh` | 通过 |
| `git diff --check` | 通过 |
| 未知 suite | `definitely-unknown` 以状态 2 拒绝，未启动外部资源 |

完整 Web 测试第一次在文件沙箱内运行时，`client-stream.test.ts` 因监听 `127.0.0.1` 被系统返回 `EPERM`，当时其余 160 项通过；在已授权本地端口环境中重跑，并在独立审查及共享导航回归补入后，去除根代理排查 CI 所用的临时重复运行后，正式共享树最终 162/162 通过。因此第一次失败是执行权限限制，不是产品回归。

正式 162 项在父代理审查时曾复现已有 history back/forward 测试的异步 `act` 偶发失败（161 通过、1 失败），下一次带路由状态探针运行 162 项通过；该未定位问题继续由父代理处理，不以重跑通过宣称根因已修复。阶段 2 的 Linux run `34090889947` 已确认 Go、MySQL、48 项浏览器验收成功，Web job 因同一旧导航用例失败。阶段 3 的真实 20 项和本阶段 5 项模型回归均已独立通过。临时 20 次循环产生的 181 测试计数不作为正式用例数。

可在仓库直接查阅[最终浏览器结果](2026-09-07-reliability-stage3-browser.json)。

## 未验证边界

- 本阶段真实浏览器为本机 Chromium。Firefox/WebKit、全键盘操作和窄屏长 Change Set 由阶段 5 验证。
- 复杂文本、JSON/NULL/空字符串往返、极值与精度、`DEFAULT_GENERATED`、大 ID 等由阶段 4 验证。
- 跨数据库引擎不属于本阶段；当前真实数据源为 MySQL 8.4。
- 并发版本控制 #33、账号与审计身份 #34、Agent #21 未实现或扩展。
- 本阶段单独运行 `operation-coverage`；`all` 的组合路径由 runner 条件和后续 CI 验证，不把本次单套件结果表述为组合套件已实跑。
