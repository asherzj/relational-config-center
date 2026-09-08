# T8 / #55 发布单升级与整体交付验收

固定功能基点 `ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087`，在独立 `codex/issue-55-upgrade-delivery` 分支集成当时最新主分支 `5df095c57769aed828cba9f617adcbfad48d7861`。18 处真实冲突逐处保留正式发布语义和主分支的输入、真实 ID、恢复及浏览器改进；没有恢复旧记录直写实现。父规格 #48 保持开放，分支交付不代表合并或部署。

本候选记录按持久原始输出填写；按用户要求先提交推送并创建 MR；本地检查现已完成，最终签核及远端 CI 状态继续据实补充；远端最终提交 CI 的实际结论固定记录在 [#55 完成评论](https://github.com/asherzj/relational-config-center/issues/55)，不通过 CI 后再追加文档提交改变已验证候选。全部 48 项的主要交付归属及前七单固定提交证据见[验收索引](2026-09-08-release-acceptance-index.md)。

## AC-045：存量升级

`TestAccountUpgradeFromLegacyMatchesFreshSchema` 在真实 MySQL 先完成原有 001–007 结构，写入符合 007 布局的旧账号、Argon2 密码、有效原 Cookie/CSRF 会话及已有 Policy/业务配置，再执行 008–012。该夹具模拟迁移前的持久状态，没有声称启动过历史版本二进制。最终实际 Admin 进程复用旧会话和旧密码，永久账号 ID、登录版本及 Policy/包含 CRLF、制表符和尾空格的业务值保持；存量未跟踪记录基线为 `0`，没有伪造 Record Version、发布单、Command、通知或角色授予。

升级后的 13 张控制表与最终新建 Schema 比较列、索引及外键；旧账号首次引入角色为 VIEWER，写入 403，首位 ADMIN 仅由实际维护命令明确授予。缺少 009、缺少 010–012 组成部分时实际启动退出 1，并提示对应迁移。旧记录写路径真实拒绝。

`TestAccountRoleUpgradePreservesAccountsAndGrants` 从 008 只完成 roles 列的中断点继续，结构未完成时拒绝 Ready；恢复、再次运行迁移和重复授予后，保留当前 ADMIN、角色版本 2 与唯一维护事件。已有账号/角色/记录版本、发布历史和幂等结果不会因重跑而重置。007 仍是一次性迁移，其 DDL 不被本期重新设计为可任意重放。

## AC-046：正式浏览器流程与恢复

`make test-browser-acceptance` 在最终相同字节的候选上以 `RCC_E2E_SUITE=all`、`RCC_E2E_ENGINES=chromium,firefox,webkit` 实际执行，2026-09-08 04:55:05～05:02:07 UTC 完成，命令耗时 422.655 秒、退出 0。数据库完整夹具逐字节一致，所有临时数据、监听器、容器和数据卷完成清理，生成凭据检查通过。

| 正式入口中的场景 | 实际通过证据 |
|---|---|
| 未保存导航、规则说明 | 14 项、6 项 |
| 写入恢复、目录与数据操作、复杂字段 | 28 项、20 项、42 项；复杂字段失败 0、页面异常 0 |
| 320px 可访问性和键盘操作 | Chromium / Firefox / WebKit 各 7 项；四类表面无横向溢出，底部动作可达 |
| 草稿、审批、混合与 1,000 项批量 | 6 项、4 项、5 项；编辑保存等待实际 PUT，两项内容及版本一致 |
| 正向与重新审批的回滚 | 三引擎各 4 项；独立 EDITOR / APPROVER / PUBLISHER，原单和反向单双向关联 |
| 账号、登录/刷新、冲突和执行恢复 | 三引擎各 21 项；390px 未知执行后刷新，以同一正文和原键恢复，只有一个 ADD Command |

可访问性中的未知 201 场景证明**草稿创建**恢复；账号和批量场景另行证明真正已提交的**执行**响应丢失后恢复，不混用两类证据。三引擎使用独立回滚表和随机账号/模板身份，保留真实记录版本和审计字段。

[WebKit 390px 未知执行画面](2026-09-08-release-unknown-webkit-390.png) · [WebKit 390px 反向发布与历史](2026-09-08-release-rollback-webkit-390.png)。

主分支更严格的未知结果分类原先只认识目录拒绝码，真实浏览器因此发现首次明确 `publication_unsupported` / `release_auto_id_ambiguous` 422 也锁定原草稿。当前识别正式发布的确定拒绝，保留输入并允许修改；先前已发生网络/响应未知后，同一 422 仍保留原正文和请求键。状态、记录版本等明确冲突继续要求查看最新并显式重建；授权或读取失败不会被当作原执行成功或丢弃原请求。

发布草稿编辑复用 `ManagedTextInput`，原 CRLF/CR 内容先只读保留，只有用户明确转换为 LF 后才允许修改。非法 ENUM 值可保存为待审批申请，因此浏览器沿实际独立审批、执行拒绝、取消/复制、明确修改并重新审批验证，不能以旧直写阶段的拒绝位置代替正式流程。

## AC-047：完整检查与临时结构退出

| 检查 | 实际结果 |
|---|---|
| Web 全量 `pnpm --dir web test:run` | 28 个文件、296 项通过；命令 22.753s |
| Web `typecheck` / `build` / `test:dev` | 全部退出 0；开发地址检查 2 项通过 |
| 正式完整浏览器入口 | 退出 0，422.655s，三引擎全套且清理通过 |
| 保留的 `make test-browser` 系统入口 | 退出 0；命令 106.863s / Admin 104.317s，真实账号日志及共享回滚夹具通过 |
| 最终 Go 全模块测试 / 构建 | 全部退出 0 |
| 完整真实 MySQL `make test-integration` | 退出 0，1774.439s；493 项测试及子测试通过，FAIL 0 / SKIP 0；Admin 1772.679s，HTTP 113.024s |
| 缺少必需依赖的负向检查 | Docker 无效地址实际退出 1、无跳过；必需浏览器不存在实际退出 1；两者均为预期失败检查 |

完整 MySQL 于 2026-09-08 09:38:47～10:08:22 UTC 实际运行成功，`GOFLAGS=-v` 保留全部测试名称。启动、历史、身份及批量回归均包含在该完整结果中；早期文件名含 green 但退出 1 的运行没有计为通过。

主分支新增的实际 ID 验收已迁到 `Create → Submit → 独立 Approve → Execute`。真实红例发现 `TINYINT(1)` 自增返回 2 时原实现可以提交一个公开 boolean 主键解析器无法寻址的记录；修复后在同一事务提交前校验最终 ID。每项 INSERT 从协议结果取得实际自增 ID，并以 uint64 无损表达；保留无符号大 ID、非标准自增步长、元数据锁、实际 SELECT 撤权、触发器安全拒绝及整笔回滚。不是通过重新引入旧 facade 修复。

`ManagedTableMutation`、旧 MutationExecutor/MutationSnapshot 协作者和旧 `/rows` 正向调用不存在；旧三种写路由由真实 HTTP 及编码路径负向检查保护。1～1,000 项正式混合发布和反向流程保留，默认业务期限仍为 4 秒。通知状态仍为 `NOT_CONNECTED`，没有实际投递或消费者成功假象。

独立 Spec 评审未发现具体问题；Standards 未发现硬性违规，有一项低优先主观建议：共享 WriteRecovery 组件集中目录字段展示。当前展示分支不恢复旧写入口或隐式重试，既有组件和真实恢复验收已通过，本期接受该非阻断建议并保持执行代码不变。两轴报告形成时完整 MySQL 尚在运行；最终证据签核及远端 CI 结论以 #55 后续记录为准。浏览器执行前后 320 个执行/测试/配置文件的 SHA-256 清单完全一致；记录在持久验收目录。

## AC-048：永久历史

`TestReleaseHistorySurvivesExecutableRestartAndExternalChanges` 使用真实 Admin 可执行进程和五个独立账号，通过公开 HTTP 创建 DRAFT、PENDING_APPROVAL、APPROVED、REJECTED、CANCELLED、ROLLED_BACK、SUCCEEDED 全七种状态；所有申请、意见和正反向结果来自正式动作。修改申请人、审批人、发布人的显示名，使用实际维护工具修正邮箱并停用账号；随后弃用规则、停用分配、改变规则名称并删除业务表，在已有历史上重跑 008–012，停止旧进程后真正启动新进程。

存活 VIEWER 读回全部单据与完整差异、意见、永久身份、Command 及正反向关联，与变更前持久投影逐项完全相同。已有详情路径的 ADMIN DELETE 返回 400 `method_not_allowed`；伪造 history/applicant_id 的 PUT 返回 400 `invalid_request`，再次完整读回不变。VIEWER 不能假借停用发布者的原成功请求执行。历史无需当前业务表、账号显示资料或现行规则即可读取，没有物理删除或自动清理入口。

## 范围与运维文档

当前操作、角色初始化、升级维护窗口、未知结果原键恢复和受保护回滚已同步至[升级指南](../admin-release-upgrade.md)、[角色指南](../admin-account-roles.md)、[审批指南](../admin-release-approvals.md)及[发布协议](../design-notes/publication-contract.md)。ADR-0019/0020 的既定决策保持，增加当前实现指针；领域词汇不写入任务进度。旧 FLOAT 身份在途单先取消，再停止旧写入者、升级并提高维护基线；首次初始化与已有角色重跑保留明确区分。

Environment、跨表发布、规则目录审批、业务 Schema 编辑、灰度、Worker、Server/Client 实际分发、版本大盘和历史自动清理仍不在本期。Notion 项目总表将在最终 CI 通过后按实际 Admin 已交付范围更新；当前尚未写入本功能完成状态，Agent 审计与 Runtime 工作保持未来范围。
