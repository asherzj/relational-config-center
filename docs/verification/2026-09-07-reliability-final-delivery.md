# 管理台可靠性工作包交付记录

本记录汇总本轮六阶段工作，主要可靠性修复锚定合并提交 `24ee35239c5220f8072d21d0b096f4b60b019ec8`，随后同步 main 的 `1590d743f863c481da179f3c355cd55479bfaac3` 账号时间精度修复及 `cd5df5845ea9b978f07639d7ac5d88243e400517` 的 shadcn/ui 改造。分支为 `codex/management-reliability-20260907`，目标是 [MR #43 → main](https://github.com/asherzj/relational-config-center/pull/43)。运行状态以 [MR 检查](https://github.com/asherzj/relational-config-center/pull/43/checks)及 [Notion 项目交付记录](https://app.notion.com/p/3ca8544cc98980559f27e17f2c94cbaf)为准；本 MR 不自动合并。

## 交付内容

1. 新增可从干净 checkout 启动的真实 MySQL、Admin、Web production preview 浏览器验收，以及 Linux CI、超时、认证边界、完整 fixture 比对和资源清理。
2. 未知写入保留草稿、锁定再次提交、按实际目标只读核对并要求人工恢复；修复快速重复确认、两种浏览器后退窗口及结果导航误弹离开提醒。
3. 补齐规则草稿、元数据、生命周期、表分配、查询条件和操作权限的真实页面覆盖，修复 Auto Fill 大小写与冲突校验。
4. 保留 LF、CR/CRLF、NULL、空字符串与完整大整数，正确区分表达式默认值和计算列；在同一事务内确认真实可寻址主键，覆盖零、规范化、触发器和非事务表拒绝。
5. 修复嵌套弹层、原生 inert、键盘焦点、滚动锁和 320px 窄屏；完成 Chromium、Firefox、WebKit 的独立检查。
6. 更新运行与验收文档、纠正原工作包时间和已完成能力的旧描述，核对 Issue 状态并同步 Notion。过程中合入 main 已交付的本地账号能力，保留真实 Cookie/CSRF、同账号恢复和跨账号清草稿。

阶段模型按任务复杂度选用：阶段 1、3、5 为 GPT-5.6-Sol/high；阶段 2、4 为 GPT-6-Astra/high；阶段 6 文档为 GPT-5.6-Luna/medium。各阶段按顺序启动 subagent，父代理负责衔接、实际故障补审、提交、推送及交付核验。具体记录见[工作包](2026-09-07-reliability-work-package.md)。

## 账号合并时的本机检查点

| 验证 | 结果 |
| --- | --- |
| Web typecheck 与全量测试 | 25 文件 / 243 项通过 |
| 全部 Go 模块 | `make test`、`make build` 通过 |
| Admin race | 通过 |
| 完整 MySQL integration 构建 | 165 个顶层测试、含子测试共 372 项通过，0 失败、0 测试跳过；包含该构建中的普通单元测试 |
| 真实 Chrome 账号系统路径 | 41.752 秒通过，含注册、恢复、草稿隔离、实际 Account ID 写入和 Cookie/存储检查 |
| 生产构建浏览器组合 | 10:00:51Z–10:03:38Z，131 项通过 |
| 认证与清理 | Cookie/CSRF 边界通过，完整种子行相等，本轮进程、容器及数据卷清理通过 |

131 项由 14 项未保存保护、6 项规则说明、28 项写入恢复、20 项操作覆盖、42 项复杂字段以及三引擎各 7 项组成。独立认证检查和 Chrome 账号系统路径不重复加入该合计。

原始结果的 SHA256、各项用例、环境、源码树指纹和清理证据已写入[结构化报告](2026-09-07-reliability-final-delivery.json)。本机原始产物位于工作树相邻的 `.records-reliability-20260907/merged-cookie-verified/`。合并和故障修复细节见[账号合并验收](2026-09-07-reliability-account-merge.md)。

## shadcn/ui 合并后的验证

main 在收尾期间合入 [MR #46](https://github.com/asherzj/relational-config-center/pull/46)。本分支解决了 12 个冲突文件，采用新的中性主题、Radix 组件和 inline Sheet/Dialog，同时保留未知写结果锁、只读核对、原始 CR/CRLF、history entry 防护与原生 inert。抽屉仍保留 320px 下单列收缩及底部操作可达约束。Playwright 继续固定为本轮已验收的 1.62.1。

Web 25 文件 / 243 项、typecheck、production build 通过；真实 Chrome 账号路径 47.76 秒通过。合并时修复了新 jsdom `matchMedia` mock 对 Node TCP 测试环境的假设；操作验收脚本改为定位可见的 checkbox 角色，避免命中 Radix 隐藏表单输入。两处都未放宽产品或写次数断言。

浏览器组合的本次结果与源码检查点见结构化报告的 `shadcnMainFollowUp`；此前表格和 Linux 图片保留为其对应提交的历史证据。最终 HEAD 的 CI 以 MR/Notion 的明确结果为准，不把早期检查点写成新树验收。

## Linux 与事实边界

Linux 每次提交独立运行 Web、Go unit and build、MySQL 8.4 integration、Browser acceptance 四项。产品提交 `24ee352` 的 [run 34109234808](https://github.com/asherzj/relational-config-center/actions/runs/34109234808) 已于 10:06:56Z 通过 Browser acceptance：131 项、三引擎各 7 项、fixture 及清理均通过。artifact `10013816450` 下载后的 SHA256 与 GitHub 元数据一致，为 `21d4d10f7bdfbb96ee5147af8e6604834ecf48c26dfeb4479e04ec5191731e3d`，结构化报告保留各套件结果摘要。该 workflow 后因新推送取消，取消的 MySQL job 不计为通过。随后 `702f5e8` 的 [run 34111882098](https://github.com/asherzj/relational-config-center/actions/runs/34111882098) 四项全部通过，MySQL 耗时 20 分 16 秒；Browser 131 项的 artifact `10014847498` SHA256 为 `31b1926b9eed3417dcaad390e96a66783825234e22e6edb0bff0bb189d440ae8`，下载、数量、fixture 及清理均已核验。shadcn 合并后的最终 HEAD CI 另在 MR 正文及 Notion 确认。

阶段 5 初次 Linux WebKit 在 320px 出现裁切，红图、修复与本次真实 [Linux 绿图](2026-09-07-reliability-stage5-linux-webkit-drawer-320-green.png)已保留在[阶段 5 报告](2026-09-07-reliability-stage5-browser-accessibility.md)。本次 artifact 已单独核对布局、各套件通过数量及清理，不以本机结果替代 Linux 结论。

Playwright WebKit 代表该构建，不代表系统 Safari 全版本。CR/CRLF 剪贴板场景使用合成 DataTransfer/ClipboardEvent；Firefox/WebKit 的 beforeunload 验证为浏览器事件，不宣称覆盖原生提示。故障注入在真实数据库提交后破坏响应，并核对实际写次数；跨客户端并发仍使用现有 last-write-wins 语义。

## 时间、Issue 与资源

收尾时发现 main 已通过 MR #45 修复账号到期精度：应用时钟与 MySQL `DATETIME(6)` 统一到 UTC 微秒精度，避免存储取整延长会话寿命；完整 integration 的进程上限从 20 调整为 25 分钟，CI 总上限仍为 30 分钟。旧 main 的 20 分钟超时和两项到期边界失败有独立日志，不能解释为本轮浏览器回归失败。该修复已同步到工作树，6 个账号组与 2 个纳秒边界子例在真实 MySQL 上全部通过（52.382 秒、0 失败、0 跳过），6 个容器已清理；普通 application/mysql 检查也通过。最终 HEAD 完整 CI 另在 MR/Notion 确认；上表 372 项仍表示此前本机检查点，不篡改为新树全量计数。

本轮开始于 04:08:58Z，本机最终组合结束于 10:03:38Z，至此历时 5 小时 54 分 40 秒。该跨度包含验证运行、CI 等待和任务恢复，不能全部记为纯编码时间；最终交付时间与额度在 Notion 收尾记录中确认。原一轮约 1 小时 57 分不等于 8 小时，也不与并行 subagent 时间相加。本轮没有通过空等或无新风险的重复测试补足时长。

[#22](https://github.com/asherzj/relational-config-center/issues/22) 已随 MR #42 于 04:04:22Z 关闭；PM-006 与原 PM-053–057 的当前状态已在 Notion 更正。本轮使用新的 worktree 和分支，各阶段提交均已推送，原工作区未提交内容保持原状。额度充足，截至本记录未兑换 reset。
