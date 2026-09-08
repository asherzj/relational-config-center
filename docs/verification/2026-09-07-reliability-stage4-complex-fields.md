# 可靠性工作包阶段 4：复杂字段写入与再次编辑

日期：2026-09-07。分支：`codex/management-reliability-20260907`。本阶段在前一阶段的操作覆盖之后，验证真实 Chromium → production Web 同源代理 → Bearer Admin → 独占 MySQL 8.4 的完整写入回路。未使用 API 替身来代替复杂字段写入。

本文保留阶段交付 `49fc66f` 的实现与证据。后续主键补审改用本次 INSERT 结果处理自增 ID，补充规范 ID 回查、事务前提和不可用身份回滚，详见[主键补审报告](2026-09-07-reliability-identity-review.md)。账号 MR #44 合并后的 Cookie 会话验收另行记录，不能倒写为本文原有 Bearer 验收。

## 已修复的实际问题

| 问题与红灯 | 最小修复 | 绿灯回路 |
| --- | --- | --- |
| 单行输入会改变 TEXT/JSON 中的 LF；真实 ADD 已返回 201，数据库保存了被改写的文本。读取已有多行内容到编辑框同样丢失 LF。多行 exact 查询也发送错误的值。 | `string`/`json` 类型的记录编辑与查询输入使用 textarea；数字与时间仍按原字符串处理。 | 完整值逐项比较：浏览器输入、Change Set、HTTP content、SQL 读取、exact-id 回读结果和重开的编辑框。多行 exact 条件返回唯一匹配行。 |
| 自增 `BIGINT UNSIGNED` 为 `9223372036854775808` 或 `18446744073709551614` 时，新增返回 503 并回滚；`9007199254740993` 原先正常。 | 在插入所在的事务连接上执行 `SELECT CAST(LAST_INSERT_ID() AS CHAR)`，保持 HTTP string 契约。 | 三档 ID 均真实 ADD 201，后续 query、PATCH 路径、SQL 主键和重开编辑均准确。 |
| 普通表达式默认值被 `DEFAULT_GENERATED` 误认成计算列；显式写默认列为 400，作为 Auto Fill 的审计列令 query 为 422。 | 只把非空 generation expression 或 `STORED GENERATED` / `VIRTUAL GENERATED` 标记判为计算列。 | 普通默认列可显式 ADD/PATCH；省略 ADD 使用数据库默认、PATCH 省略保持默认后的值；Auto Fill 正常；真实计算列 ADD/PATCH 仍拒绝。 |
| 显式自增 `id="0"` 返回 0，SQL 却生成 1；非自增 ID 使用常量/表达式默认时返回上次会话残留 1，SQL 实际是 42/43。 | 非自增 ID 即使有默认值也必须显式提交，写前返回 400 `missing_required_field`；ADD 显示默认不包含的 id，MODIFY 仍隐藏。自增零值根据本次 INSERT 结果决定读取生成 ID 或返回实际保存的 0。 | 浏览器先生成 1，再提交 0：默认 SQL 模式返回/保存 2，`NO_AUTO_VALUE_ON_ZERO` 返回/保存 0；两者 exact-id 回查与 PATCH 均正确。非自增默认两表先拒绝且 SQL 零行，填写 42/43 后成功。 |
| 已有 CRLF / 独立 CR 在 textarea DOM 中显示为 LF；若直接允许编辑就可能无提示改写换行。 | 共享 `ManagedTextInput` 保留原始 state，含 CR 时只读并说明；用户点击“转换为 LF 再编辑”才转换。粘贴先从 ClipboardEvent 取得原文，含 CR 时按选择区插入原始字符串并进入保护。 | 改其他字段、包含原字段提交，HTTP/Change Set/SQL HEX 都保留 CR；显式转换后编辑才保存 LF。新增记录与 exact 查询均验证 CR 粘贴、原值提交和转换行为。 |
| VARCHAR 过长产生 MySQL 1406、TINYINT 越界产生 1264，却向用户返回 503，阻止直接修改草稿重试。 | 只将已复现的 1406/1264 加入既有输入错误分类。 | 400 `invalid_mutation_content`、安全消息和 request ID；SQL 行数不变，草稿值及包含勾选保留；纠正后 201 且仅新增一行。 |

生成列识别复用了根代理先行核查的 [MySQL 8.4 COLUMNS 官方定义](https://dev.mysql.com/doc/refman/8.4/en/information-schema-columns-table.html)：`DEFAULT_GENERATED` 表示表达式默认值。大 ID 红灯与锁定的 go-sql-driver/mysql v1.10.0 将 uint64 insert ID 存入 int64 的实现一致。

自增零值先检查本次 INSERT 的 `LastInsertId`：结果为 0 就返回实际零值，不读取可能残留的会话 `LAST_INSERT_ID()`；非零（包括 driver int64 溢出后的负数）才按字符串读取本次生成的完整 uint64。新查询不会离开插入的事务连接去连接池另取会话。没有把合法大整数转换成 JavaScript Number，也没有增加 HTTP 列属性或按列名猜类型。未知数据库错误仍为不可用；回归明确保留缺表 1146、死锁 1213、SIGNAL 1644 的分类。

CR 保护依据 [WHATWG textarea 规范](https://html.spec.whatwg.org/multipage/form-elements.html#the-textarea-element)：DOM API value 会将 CRLF / CR 规范化为 LF。页面显示可能使用 LF，但受保护草稿保留原文，不能据此宣称任意 CR 文本均可无损编辑。当前操作边界是保留原文提交，或明确转换为 LF 后编辑。粘贴验收使用 Chromium `DataTransfer` + `ClipboardEvent` 原文夹具，没有声称覆盖操作系统剪贴板的换行处理。

## 数据语义矩阵

| 数据 | 真实浏览器操作与数据库断言 |
| --- | --- |
| TEXT 与 JSON | 中文、emoji、LF、TAB、首尾空白；修改后的文本 12,668 UTF-8 字节。TEXT 比较完整字符串；JSON 保留请求原文与内部大整数，经 MySQL 保存后按数据库规范 JSON 文本比较回读和重开值。不是只比长度或做 JS JSON stringify 重写。 |
| NULL/空/省略 | 分别创建 SQL NULL、JSON 文本 `null`、JSON 字符串 `"null"`、JSON 字符串 `""`，并与 TEXT SQL NULL、真实空串、遗漏字段默认值区分。每组再 PATCH 一个字段，确认未提交的字段完全保持。 |
| 整数与小数 | `BIGINT` 的 `-9223372036854775808` / `9223372036854775807`，`BIGINT UNSIGNED` 的 `18446744073709551615` / `0`；65 位有效数字的正负 DECIMAL(65,20)，含末尾 0。完整 HTTP/SQL/编辑器值始终为 string。 |
| 日期时间 | 闰日 `2024-02-29`，TIME `23:59:59.123456` / `00:00:00.000001`，DATETIME 微秒，TIMESTAMP `2026-09-07T12:34:56.654321Z`。浏览器时区设为 America/Los_Angeles，仍严格遵守 ADR-0007 的 UTC 格式；未扩展为 MySQL duration。 |
| 默认/Auto Fill/计算 | DATETIME DEFAULT CURRENT_TIMESTAMP(6)、VARCHAR 表达式默认、真实 STORED/VIRTUAL 计算列并列对照。Auto Fill 创建时间与更新时间相同，处于数据库前后时刻内；修改保持创建时间并推进更新时间。 |
| 存储拒绝 | 1406 与 1264 由独占 MySQL 实际拒绝并保留编号，再由真实浏览器触发 Admin 拒绝、返回修改、纠正重试。 |

## 可重复的命令与工件

```bash
RCC_E2E_SUITE=complex-fields \
RCC_E2E_ARTIFACTS=/absolute/new-empty-directory \
bash scripts/browser-acceptance.sh
```

零主键的另一种 SQL 模式使用同一个 runner 的独占容器启动参数，不执行 `SET GLOBAL`，也不修改用户数据库配置：

```bash
RCC_E2E_SUITE=complex-fields \
RCC_E2E_CASE='explicit auto ID zero' \
RCC_E2E_MYSQL_SQL_MODE=ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION,NO_AUTO_VALUE_ON_ZERO \
RCC_E2E_ARTIFACTS=/absolute/new-empty-directory \
bash scripts/browser-acceptance.sh
```

默认 `all` 包含 `complex-fields`。调试可用 `RCC_E2E_CASE` 按用例标题子串缩小回路；空选择失败，不把零用例当通过。每次运行由 runner 创建并清理唯一命名的 MySQL 容器、volume、Admin 与 Web 监听。夹具仅使用明确列出的 `stage4_*` 表与两条 Mutation Policy；清理按这些确切名字进行。

修复前工件位于 worktree 同级的 `.records-reliability-20260907/`：

- `stage4-red-3/complex-fields/`：真实写入失败、request ID、SQL 行数、文本 SQL HEX 与 Change Set 差分、截图。文本错误写入的 request ID 为 `rcc-bbe220ce26ef0f2672d21d6ae20447f9`，SQL 实际有一行且其文本 HEX 不含 LF 字节。
- `stage4-query-red-2/complex-fields/`：多行查询红灯，HTTP 内容把 LF 换为空格；数据库有原始值。
- `stage4-editor-unit-red.log`：再次打开多行原值的确定性组件红灯。
- `stage4-errors-unit-red.log`：实际分类函数对 1406/1264 的确定性红灯。
- `stage4-green-1/`：首次完整 13 项通过，401/200 鉴权边界、fixture 前后相同、清理通过。
- `stage4-all-final/`：首轮 81 项组合运行，仅代表 ID/CR 补审前的检查点。
- `stage4-identity-cr-red/complex-fields/`：真实自增零值错 ID、常量/表达式默认 ID 返回会话残留值，以及 CR DOM 归一化的四项产品红灯；该次另一项随机时间小数末尾 0 差异是验收预期错误。HTTP 已按既有契约去尾 0，现只修正 HTTP 预期，SQL 仍比较完整六位微秒。
- `stage4-supplement-green-2/`：ID/CR 补审后的默认 SQL 模式 18 项全绿。
- `stage4-no-auto-zero/`：独占 MySQL 启动 `NO_AUTO_VALUE_ON_ZERO`，真实浏览器零 ID 新增、回查、修改通过。
- `stage4-supplement-all-final/`：补审后的最终组合运行。

早期 `stage4-red-1` 除再次确认 Auto Fill query 422 外主要是验收选择器错误；`stage4-red-2` 因新增测试漏传路由参数未完成构建；`stage4-query-red` 因根代理当时的测试探针类型错误未完成构建。`stage4-red-3` 的 JSON 标量 SQL 读取曾受 mysql CLI batch escaping 影响，后通过 `--raw` 修正验收读取；不把这些工具错误算成产品缺陷。

## 最终验证

首轮 `stage4-all-final` 在 2026-09-07T07:01:33Z–07:03:35Z 的 81 项是历史检查点，补审新增修改的最终验收另见下列记录。

- 最终默认模式组合运行 `stage4-supplement-all-final` 于 07:27:00Z–07:28:50Z 完成，共 86 项（14 + 6 + 28 + 20 + 18）。阶段 4 的 18 项全部通过，`pageErrors` 为空；临时表、Table Policy、规则均为 0，种子内容前后逐字节相同，进程、容器与 volume 清理通过。
- 独立 `stage4-no-auto-zero` 于 07:26:19Z–07:26:42Z 完成，真实 MySQL 启用 `NO_AUTO_VALUE_ON_ZERO`；先生成 ID 1，再显式写入 ID 0，响应、SQL、exact-id 查询和 PATCH 均为正确的 0。种子内容相同，清理通过。
- 两次运行均直接 Admin 无 Token 为 401、Web 同源代理为 200。完整时间、资源名称、请求编号、SQL 值和种子 SHA256 保存在同目录 JSON；没有用初轮 81 项代替补审后的最终结果。

- `pnpm --dir web test:run`：22 文件、167 项通过（当前共享树，包含根代理路由归属回归）。第一次在沙箱内因真实 TCP 流中断测试监听被拒绝，授权环境重跑通过；不把权限错误算作产品缺陷。
- `go -C admin test ./...`：全部非 integration 测试通过。日志 `stage4-supplement-go.log`。
- `pnpm --dir web build`、`node --check web/e2e/complex-fields.cjs`、`bash -n scripts/browser-acceptance.sh`、`git diff --check` 通过。
- 补审初次 `stage4-supplement-green` 在 macOS Bash 3 空数组严格展开时中止，没有执行浏览器用例。已将容器参数数组保持非空，并加 `run_completed` 保护：清理成功不能使未完成套件/数据库后检查的运行报告通过。该次历史 `exit status: 0` 不能作为验收成功证据。

仓库内保存了[完整阶段 4 红绿 HTTP、SQL 与最终结果](2026-09-07-reliability-stage4-browser.json)。原始持久目录还包含完整运行日志和失败截图，不含 Bearer Token 或数据库密码。

浏览器为 Chromium `151.0.7922.34`；Firefox/WebKit、键盘与窄屏长字段布局由阶段 5 继续。已有全类型只读 Go integration 并未在本阶段重复计作新增覆盖。

本阶段没有实现 #33 并发版本、#34 账号与审计身份或 #21 Agent；没有修改已合并工作包的历史证据。
