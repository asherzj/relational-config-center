# #82：三表存储与原单回滚验收

范围为父规格 #81 的 T1 单表完整路径，主要验收 AC-010。多表、草稿目标占用、最终错误交互及原因补填由后续工单交付；本记录不表示 #81 整体完成。

固定基点：`0e1f9f572c10100ac4c4f1c42b8f430a09b32bab`。
工作分支：`codex/issue-82-original-order-executions`。
用户最新交付政策：验证及双轴评审后只做本地 commit，由主协调者接入并管理 Issue 状态；全部七张完成后统一 push 并向 main 提一个 MR。

## 验收接缝与保留的安全证据

下表中 Go 文件均位于 `admin/cmd/admin/`，除非另有说明。每个场景使用真实 MySQL；原有独立回滚申请/审批/第二张单关联的场景已移除，其仍有效的业务保护转入原单路径。

| 要求或风险 | 当前测试位置与观察 |
| --- | --- |
| AC-010 混合变更、无原因、原单倒序恢复 | `release_original_execution_integration_test.go::TestOriginalOrderRollbackPreservesApplicationAndBothExecutions`：HTTP 发布 ADD/MODIFY/DELETE，另一发布人员无原因回滚；ID 不变，申请、审批和原实际发布结果保持，原单 ROLLED_BACK，目标释放 |
| 三类业务表、不复制整单实际结果、两次成功身份 | 上述测试检查 1 主单/3 明细/2 成功执行/6 Command/2 通知；主单和执行摘要无实际明细数组，请求结果不再复制实际 Commands；回滚逐项结果在原明细行保存，通知 ID 不同 |
| 原操作重推、原发布事实保持 | 上述测试检查回滚响应重放及回滚后原发布键返回原 SUCCEEDED 业务结果；HTTP `allowed_actions` 继续按当前主单状态计算 |
| 唯一键依赖倒序解除 | `release_rollback_integration_test.go::TestReleaseRollbackUnwindsUniqueValueDependencies` |
| ENUM 身份恢复 | 同文件 `TestReleaseRollbackRestoresDeletedEnumIdentity` |
| 真实业务值、生成值、新审计字段、长历史 | 同文件 `TestReleaseRollbackRestoresBusinessFieldsWithNewAudit`；`release_quick_rollback_integration_test.go::TestQuickRollbackRejectsDatabaseEffectsThatCannotRestoreOriginalValues` 拒绝数据库效果导致无法恢复的值 |
| TIME 时长 | `release_rollback_integration_test.go::TestReleaseRollbackRestoresStoredTimeDuration` |
| 外部 SQL / 未推进版本的篡改 | `release_quick_rollback_integration_test.go::TestQuickRollbackRejectsUnversionedExternalChangesBeforePreviewAndExecution`，预览后再次改变也拒绝 |
| Schema、规则、记录版本变化 | 同文件 `TestQuickRollbackRejectsChangedSchemaRulesAndRecordVersions` |
| 必须持有原目标 | 同文件 `TestQuickRollbackRequiresItsOriginalRetainedTargets` |
| 当前权限、版本、预览摘要、原键异内容 | 同文件 `TestQuickRollbackUsesCurrentRolesAndRequiresReviewedVersionDigestAndCurrentRole` 与 `TestQuickRollbackRestoresMixedPublicationAndReplaysActualResult` |
| 完结/已回滚禁止、竞争唯一终态 | `release_completion_integration_test.go::TestReleaseCompletionPermanentlyClosesRollbackWindow`；quick 文件 `TestQuickRollbackCompetesWithCompletionAndOtherRollbacks`；原单测试拒绝再次回滚 |
| 回滚后项约束失败不留前项写入 | quick 文件 `TestQuickRollbackConstraintFailureRollsBackEarlierItems` |
| 主单/明细/成功执行/命令/通知/目标/请求失败全事务回滚 | quick 文件 `TestQuickRollbackPersistenceFailuresPreserveValuesVersionsHistoryAndTargets`；正向 `release_publication_integration_test.go::TestPublicationAtomicPersistenceFailures` |
| 持久实际结果损坏拒绝详情与重推 | publication 文件 `TestPublicationStoredRowsAreVerifiedBeforeReadOrReplay`；completion 文件 `TestReleaseCompletionRequiresTrustedPublicationOnReadAndReplay`；有界列表仅读主单摘要，不加载每单实际结果 |
| 现有正向权限、审批撤权后事实、跨 Schema 外键及触发器保护 | publication 文件 `TestPublicationPublisherHistoryAndApprovalSurvivesRevocation`、`TestPublicationRejectsUntrackedCascade`、`TestPublicationRejectsImplicitWritesAndAuditSpoofing`、`TestPublicationSessionLocksHiddenForeignKeyDDL` |
| 并发独立发布 | `business_auth_integration_test.go::TestConcurrentAccountsOwnTheirBusinessChanges`；首次全套揭示执行表空范围死锁，改为完整主键读取后重复通过 |
| 原记录并发基线及独立资源、唯一约束失败 | `record_version_integration_test.go::TestRecordVersionRealConcurrentWriters` / `TestRecordVersionSnapshotAndIndependentResources`、`release_batch_edges_integration_test.go::TestReleaseBatchEdgeIndependentUniqueConflict`；保留败方原基线、冻结/审批历史、零部分写入与409唯一冲突断言；申请与结果通过公开详情读取，冻结内部元数据仍核对主单 |
| 并发明细读取/增长，无空范围间隙锁 | `admin/internal/infrastructure/mysql/release_details_integration_test.go::TestIndependentReleaseDetailGrowthDoesNotLockIndexGaps`：在 Adapter 边界同步两原单事务，覆盖空存储及已有明细；这不提前开放 T2 的空草稿 HTTP 能力 |
| 1000 条、正式进程默认期限、原键恢复 | `release_batch_integration_test.go::TestReleaseThousandItemsThroughExecutable`，原单发布及倒序回滚；保留原有大结果/终止余量回归，最终容量政策由 #84 收敛 |
| 进程重启、身份资料与外部结构变化 | `release_history_delivery_integration_test.go::TestReleaseHistorySurvivesExecutableRestartAndExternalChanges` |
| 新建与迁移结构一致、迁移可重跑、启动门禁 | `account_delivery_integration_test.go::TestAccountUpgradeFromLegacyMatchesFreshSchema`，同表两条旧通知/Command、最长32字节旧ID；014 重跑两次，保留技术内容及状态、补39字节不同身份；比较三类业务表和技术表，进程就绪；启动错误含 014 |
| 浏览器单次确认、留在原单、三种内容、390px | `web/e2e/release-rollbacks.cjs`：真实 Chromium/Firefox/WebKit，空原因一次确认，原单 ID 保持，两次执行，申请/原发布/恢复结果切换，竞争与丢响应恢复 |

## TDD 与问题定位

日志目录：`/private/tmp/rcc-82-logs`。

- `ac010-red.log`：原无原因 HTTP 回滚在旧实现返回 422 `release_invalid`；`ac010-green.log` 为首次纵向转绿。
- `ui-red.log` / `ui-green.log`：空原因确认的组件测试由红转绿。
- `concurrency-diagnosis.log`：InnoDB 显示两个首次发布持有 `rcc_release_executions` 的间隙锁并互相等待插入；`concurrency-green.log` 记录修复后并发场景三次通过。该日志中后续新增存储断言曾将 JSON null 的长度当作数组，已修正该断言，并在最终定向及完整套件验证。
- `details-lock-red.log` / `details-lock-green.log`：空明细同步增长复现同类锁问题，完整主键读取/删除后空和非空场景均三次通过。
- `migration-legacy-red.log`：同表两条旧通知在改主键时出现1062；`post-review-regressions.jsonl` 中同场景修复后通过，014仅补旧技术身份，不改旧业务单或技术内容。
- 首轮 `integration-full.jsonl` 是诊断记录，含旧启动文案、系统代理和执行表范围锁失败，不能作为最终通过证据。

## 最终验证索引

完整集成运行显式设置 `NO_PROXY=127.0.0.1,localhost` 与小写 `no_proxy`，避免 macOS 系统代理把 Python 会话脚本的回环请求送出；这只改变测试环境，没有修改业务脚本。Docker 使用 Colima socket，并由各测试/浏览器脚本创建及清理自己的随机容器、端口和数据。

| 检查 | 日志或产物 |
| --- | --- |
| `make test` / `make build`（全部四个 Go 模块） | `make-test-final.log` / `make-build-final.log` |
| `go -C admin test -json -count=1 -timeout=40m -tags=integration ./...` 完整入口 | `integration-final.jsonl`；固定剩余分片后主动中止，只采用已完成部分，不能称整次运行通过 |
| 最终剩余测试的两个互斥分片，避免单进程40m限制遗漏 | `final-shard-plan.json` 保存218个 cmd/admin 顶层测试完整清单及分片前77项通过、剩余71/70项；`final-shard-1.jsonl` / `final-shard-2.jsonl`，每片25m上限 |
| 评审修正及新增保护的最终回归 | `post-review-regressions.jsonl`：升级、记录并发、独立唯一冲突、容量历史、完结/回滚余量及1000条正式进程 |
| 最终存储定向：并发账号、草稿原子保存、原单双执行 | `final-storage-focused.log` |
| `pnpm --dir web typecheck` / `test:run` / `build` | `typecheck.log` / `web-tests-full.log` / `web-build.log`；28 文件、326 测试 |
| `pnpm --dir web test:dev` | `web-dev.log` |
| `make test-browser` | `account-browser.log` |
| 最终三引擎 `RCC_E2E_SUITE=release-workflow ./scripts/browser-acceptance.sh` | `browser-final.log`，产物 `/private/tmp/rcc-82-browser-final` |
| 最终源码指纹 | `final-source-sha256.json`，记录运行套件时源码逐文件 SHA-256；提交前再次核对 |

浏览器最终三引擎全流程均通过，数据库后置检查保持业务种子与规则状态。已人工检查最终桌面确认、390px确认和原单结果页截图：原因选填，整单确认可达，原审批仍在，申请/原发布/恢复结果可切换；窄屏对比表沿用带键盘焦点的横向滚动。

最终 `final-coverage.json` 核对通过：218/218 个 cmd/admin 顶层测试全部有适用的通过证据；固定基础77项、第一片71/71（649.2s）、第二片70/70（706.1s），两片零失败、零实际测试跳过。其他5个有测试的 Admin 包均通过，另3个包本来无测试文件。评审后9项定向回归全部通过（151.7s）。1000条正式进程实际发布2.61s、回滚2.13s，低于现有默认4s期限，两种原键重放均通过。

最终逐文件源码SHA-256核对没有变化。完整入口启动后仅修改迁移014及5个相关测试fixture/注释文件；运行时代码与Web保持一致，迁移和测试改动由评审后定向及最终分片验证，不将不同快照合称一次全绿运行。`full-suite-source-sha256.json` 保留完整入口启动快照，`final-source-sha256.json` 保留最终快照。

主协调者明确授权固定分片后停止重复主套件。`main-suite-cancellation.json` 记录已通过清单、停止的两进程及所属容器；停止前通过 `lsof` 核对工作目录和日志文件。两条 Go 命令均已退出，其四个容器经逐 ID 检查均已回收，没有操作其他任务资源。诊断首轮同样主动停止，不用于替代最终未覆盖范围。

## Standards

成文规范硬性违规0。初轮1项P3主观 Duplicated Code：容量测试复制主单编码形状。已提取 `seedReleaseWorkflowHistory`，只修改历史、版本和更新时间，保留真实业务存储；原 reviewer 已只读复核，未解决0。

## Spec

初轮1项P1：014为旧通知添加空执行身份后，改主键会使同表多条旧通知冲突。已仅为旧技术Command/通知补 `legacy:<order_id>`，保留原内容及状态，新增真实升级/重跑/最长旧ID验收；原 reviewer 已复核源码和通过日志，未解决0。

Standards 未解决0（历史最高P3已修复）；Spec 未解决0（历史最高P1已修复）。两轴独立并行审查了固定基点至实际工作树的完整差异，包含未跟踪新文件；工具线程槽位限制下复用已空闲事实代理进行 Spec 轴，要求重新完整读取最新规格。

## 后续接入

存储键、主单 `item_count` 驱动的完整主键锁定、逆向 Commands 与原项映射、成功执行身份及全部过渡结构见 [T1 过渡清单](../design-notes/multitable-release-tickets/t1-transitions.md)。旧 `/rollback` 只拒绝，无法创建第二张单；其剩余 DTO/内部关联分支及旧恢复界面外观明确归 #88 清理。
