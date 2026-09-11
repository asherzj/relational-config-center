# #96：个人待办、审批进度与未读验证

本票固定基点 `836c95da4100d06a102dc6cd0266035cafe18ceb`，工单与父规格完整存档在 `issue-96.json` / `issue-92.json`。仅交付 AC-013～018；迁移和共享事务调用方的必要回归也在本票验证。#97 接后续发布结果事件，#98 完成全功能综合收口。

## 验收矩阵

| 用例 | 当前实现与有效自动化证据 |
| --- | --- |
| AC-013 | `TestApprovalNotificationsQualificationChangesAcrossEveryEntry` 覆盖成员、角色启停、账号启停、Web ADMIN 及离线授予入口；`OwnActionsPreserveEarlierResultsAndTruthfulPending` 保留旧结果未读、本人操作不新增自身提醒；`ApprovalProgressAndTerminalResults` 验证仍有可审表不清整单待办。见 `core-schema-reset.txt` 九项核心 PASS 段。 |
| AC-014 | `SubmitAggregatesCurrentRecipients` 公开提交、角色/表去重、资格收件人、原键重推、无关查看者。见同一核心日志；Web 个人计数与正式浏览器补充接线。 |
| AC-015 | `ApprovalProgressAndTerminalResults` 部分/全部批准、拒绝、取消与真实历史；`OwnActionsPreserveEarlierResultsAndTruthfulPending` 自己动作保留过去未读；见核心日志。 |
| AC-016 | `ObservedDetailBoundsLateAcknowledgement` 固定实际详情读序号；`ConcurrentLateReadPreservesNewEvents` 同一真实 MySQL fixture 并发旧 ack 与新资格/新批准（评审后 `concurrent-receipt-refactor.txt` 复验）；`FailuresRollbackBusinessAndReadProgress` 已读故障不改变未读。Web/浏览器验证详情成功展示、失败可重试、后台不提前确认。 |
| AC-017 | Web 可控时间及真实浏览器覆盖进入/返回、前台 30 秒、手动刷新、错误保留、401 停止隐藏工作区读取、恢复及换账号迟到响应隔离。见 `web/rcc96-web-final-notifications-3.log`（23 项）、`browser/rcc96-browser-notifications-final.txt`（7 路径）；全局保护区与 API 调用方纳入下述全 Web 覆盖。 |
| AC-018 | `FailuresRollbackBusinessAndReadProgress` 四动作写故障；`QualificationFailureRollsBackEveryEntry` 资格故障；`LostHTTPResponsesRecoverOriginalFacts` 真实 TCP HTTP 响应丢失、人工原请求重推；核对业务、事实、原结果及个人计数，不以 GET 补写。九项核心均见 `core-schema-reset.txt`。 |

## 结构、工具和回归

`TestApprovalNotificationsSchemaUpgradeReadinessAndProtection` 在版本 7 的真实库，通过正式命令验证 8 的 DDL 权限失败记录、显式恢复、重复升级、保留数据、新装物理结构一致、Admin 启动/就绪拒绝缺失或异常结构、通用控制表保护。实际 MySQL 8.4 SHOW CREATE 在 `schema-final-reset-preserved.txt`。已发布迁移 1～7 及 manifest 原样保留，unmanaged baseline 仍固定 5 后正式 up。

新增个人通知直接成为显式离线 `release-reset` 的受影响调用方：成功事务清理九张发布相关表，保留业务记录、账号、授权历史、角色、成员及永久引用；中断回滚保持九表原样。精确 manifest 校验后只允许已知个人通知到账号的单个非级联外键；未知入/出站、级联、触发器继续拒绝。成功及保留断言见 `schema-final-reset-preserved.txt`；拒绝矩阵见 `schema-reset-faults-verified.txt` 对应 PASS 段；中断回滚见 `core-schema-reset.txt` 对应 PASS 段。未执行用户环境 reset。

共享 GET 一致快照、授权事务、原业务写 DTO 的真实受影响回归选择见 `regression-selection.txt`。`affected-regression.txt` 原轮为 17 PASS / 1 FAIL；失败的重新准备竞争夹具改为通过 `data_lock_waits` 与实际连接核对既有授权锁阻塞，保持生产锁顺序和原归属/占用/重推/独立审批断言，`reprepare-lock-regression.txt` 完整 PASS。18 项据此逐项归并，不把原轮改记为全绿。代码移除现在无调用者的独立审批环境读取包装，使详情仅通过同一个只读事务获取资格和通知进度。

## 原始失败的归并规则

日志保持原始状态，含失败的整轮不称为全绿。原始 `.txt` / `.log` 中工具输出的行尾空格和末尾空行也原样保留；Git 空白检查对源码和人工编写的 Markdown/JSON 执行，排除这些原始输出。`core-schema-reset.txt` 的九项通知核心、reset 成功和中断项通过；同轮 schema 的 REVOKE 1141、reset 的重复 FK 名 1826 是夹具失败。修复夹具后 `schema-reset-faults-verified.txt` 的 reset 拒绝全部通过，但 schema 因故障早于 migration attempt 创建而仍处于 pending；随后给仅迁移 attempt 表 CREATE 权限，正式通知 DDL 失败被确实注入，最终 `schema-final-reset-preserved.txt` 两项通过。上述失败没有用作证明故障边界的通过证据。

`ac014-red.txt` 为缺 Docker host 的环境错误；`ac014-red-http.txt` 才是真实缺接口红灯。`ac018-business-failure.txt` 首轮把未知字段 400 写成 422，改为原键有效异正文 409 后 `ac018-failure-verified.txt` 通过。`go-targeted.txt` 的本机子进程绑定/缓存限制不充当产品失败，通过证据为授权后重跑日志。Web 原始红绿与浏览器真实写 DTO 回归失败均按原状归档，再单列最终有效证据。

## #97 正式接口

`ReleaseOrderSession.RecordApprovalNotifications(ctx, order, actor, resultRecipients)` 由原业务事务在状态保存后、原请求结果保存前调用一次。它沿唯一审批资格规则同步待办，结果收件人去重推进进度并排除本人，保存失败整笔回滚。#97 提供申请人及历史审批参与者并接入发布、完结、回滚、重新准备取消。写响应仍是原请求结果；只有后续成功展示的 GET header 里的独立 notification.sequence 可作已读依据。不存在后台补通知、GET 补写、业务自动重放、旧单快照兼容或新的初始化路径。

## 资源与复核

测试显式使用 Colima socket 和 `TESTCONTAINERS_RYUK_DISABLED=true`；每次一台本票 MySQL，真实业务竞争保留在单 fixture 内。迁移、MySQL 与正式浏览器串行。用户两个运行中的常驻容器未修改。最终资源清单见 `resource-inventory.txt` / `resource-verification.json`；没有本票存活容器或服务进程，已有其他停止/Created 容器保持原状。两轴审查见各自报告；源码与证据哈希见 `source-manifest.json` / `evidence-manifest.json`。提交与远端 SHA 由提交后交接核对，不以自引用写回本次证据。

## 有效验证汇总

- **真实 MySQL：31 个唯一顶层用例**，包括通知核心 9、新迁移 1、直接受影响 reset 3、共享调用方 18。`mysql-results.json` 对每项记录有效日志、行号和原始 PASS 行，原失败轮仍完整保留。
- **Go：5 个有测试的影响包通过**（domain/application/mysql/http/cmd-admin），account-maintain 编译通过但没有测试文件；见 `go-final.txt`。公开 route/写与读 DTO、依赖方向、只读通知 reader 不获写能力的机器检查包含在内。
- **Web：421 项当前有效覆盖**。`web/rcc96-web-full-suite.log` 是 39 文件 420 项中的 419 PASS＋1 本机 TCP bind EPERM；相同代码的 `client-stream-escalated.log` 对该项 PASS。后来只改已读组件在卸载后的同账号共享缓存刷新，并加 1 个公开 UI 用例；23 项受影响通知/契约测试在 `final-notifications-3.log` 复验通过，其他未受影响全量结果仍有效。`final-build-4.log` 包含 typecheck 与生产 build 通过。所有简称文件在 `web/` 下均带 `rcc96-web-` 前缀。没有将原 420 项整轮声称为无失败。
- **正式浏览器：2 个 runner，12 个关键路径**。原中心 `browser/rcc96-browser-center-final-3.txt` / 同名目录 result.json 为 5 PASS；本票 `browser/rcc96-browser-notifications-final.txt` / 同名目录 result.json 为 7 PASS，两者 errors=[]。正式 UI 覆盖桌面、390px、键盘、未读/待审批区别、详情 ack 失败和恢复、401 暂停、换账号迟到响应。最终截图由 Web 实现者及 root 实际查看，无新增视觉问题。

Web 共享 fixture 审计：通用 account-session helper 仅为无关页面提供明确零计数；个人通知用例自己控制 counts 成功/失败。缺失 notification 的 GET 契约测试绕开通用 release fixture，确保默认值不掩盖生产解析失败。

全部已发布 1～7 SQL/manifest 共 14 文件与基点逐字节相等，见 `immutable-migrations.json`；依赖锁文件未变化，node_modules/dist 未纳入交付。

Standards 和 Spec 从完整未提交固定基点差异分别独立审查。Spec 初始唯一 P2 为重新准备回归缺口，已保留发现并追加真实锁等待复验关闭；当前剩余 0。Standards 初始非阻断 P3 为并发夹具的平行字段数组，改为局部具名 concurrentRequest 后仅该完整顶层用例在 `concurrent-receipt-refactor.txt` 复验 PASS，评审追加关闭，剩余硬性 0／主观 0。此次另有 import 分组格式整理；均无生产行为变化，不使其他已有证据失效。
