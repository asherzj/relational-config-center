# 功能 #81 本地验收索引

2026-09-11（Asia/Shanghai）：七张切片已经本地实现与验证。下表保留每项主要工单，不将前序切片责任改记为 #88。最终真实用例来自逐项核对的 335 项清单，原有切片证据与最终集成证据共同支撑本地验收；远端合并、部署和 Notion 项目记录由协调者另行完成，本记录不代表已执行这些动作。

| 验收项 | 主要工单 | 切片证据 | 最终集成真实用例 |
| --- | --- | --- | --- |
| AC-001 | #84 | [2026-09-10-multitable-execution.md](2026-09-10-multitable-execution.md) | PASS：`TestMultitablePublicationPreservesGlobalOrderAndOriginalResults`<br>`TestOriginalOrderRollbackPreservesApplicationAndBothExecutions`<br>`TestPagedExecutionResultsStayOnOriginalDetailsAndCurrentOrder` |
| AC-002 | #83 | [2026-09-09-draft-target-reservations.md](2026-09-09-draft-target-reservations.md) | PASS：`TestDraftConcurrencyKeyProtectsValuesAndDefinition`<br>`TestDraftKeyEligibilityDefaultsAndAutoIDReferences`<br>`TestDraftSaveAndKeyDefinitionRaceCannotBypassReferences` |
| AC-003 | #83 | [2026-09-09-draft-target-reservations.md](2026-09-09-draft-target-reservations.md) | PASS：`TestDraftReservationsBeginOnSaveAndEndOnCancel`<br>`TestDraftSharedKeyReferencesReleaseOnlyAfterLastDetail` |
| AC-004 | #83 | [2026-09-09-draft-target-reservations.md](2026-09-09-draft-target-reservations.md) | PASS：`TestDraftConcurrencyKeyDatabaseEquality` |
| AC-005 | #83 | [2026-09-09-draft-target-reservations.md](2026-09-09-draft-target-reservations.md) | PASS：`TestDraftIncrementalEditsReplaceTargetsAtomically`<br>`TestDraftTargetReplacementRollsBackOnPersistenceFailure`<br>`TestReleaseHeaderAndDetailPagesRequireOneWholeOrderVersion` |
| AC-006 | #84 | [2026-09-10-multitable-execution.md](2026-09-10-multitable-execution.md) | PASS：`TestMultitableThousandPagedDetailsExecuteAsOneOrder`<br>`TestMultitableLargeValuesThroughHTTPLifecycle` |
| AC-007 | #84 | [2026-09-10-multitable-execution.md](2026-09-10-multitable-execution.md) | PASS：`TestDraftAllTargetTypesReleaseOnEveryTerminalAction`<br>`TestDraftSubmitRejectsChangedTargetIdentity`<br>`TestReleaseCompletionPermanentlyClosesRollbackWindow` |
| AC-008 | #85 | [2026-09-10-multitable-copy-reprepare.md](2026-09-10-multitable-copy-reprepare.md) | PASS：`TestMultitableCopyCompetesForEveryTargetAndLinksBothOrders`<br>`TestMultitableReprepareTransfersChangedTargetsAtomically`<br>`TestMultitableDerivedDraftRequiresExactDetailIdentity` |
| AC-009 | #84 | [2026-09-10-multitable-execution.md](2026-09-10-multitable-execution.md) | PASS：`TestMultitablePublicationPreservesGlobalOrderAndOriginalResults`<br>`TestMultitableDraftWaitsForAllGuardsBeforeAnySnapshot` |
| AC-010 | #82 | [2026-09-09-original-order-executions.md](2026-09-09-original-order-executions.md) | PASS：`TestQuickRollbackRestoresMixedPublicationAndReplaysActualResult`<br>`TestQuickRollbackRequiresItsOriginalRetainedTargets`<br>`TestQuickRollbackUsesCurrentRolesAndRequiresReviewedVersionDigestAndCurrentRole` |
| AC-011 | #86 | [2026-09-10-safe-manual-retry.md](2026-09-10-safe-manual-retry.md) | PASS：`TestConfirmedReleaseFailureHistoryPreservesOriginalRetry`<br>`TestReleaseFailureHistoryUnavailableDoesNotLeaveBusinessWrites`<br>`TestQuickRollbackPersistenceFailuresPreserveValuesVersionsHistoryAndTargets` |
| AC-012 | #86 | [2026-09-10-safe-manual-retry.md](2026-09-10-safe-manual-retry.md) | PASS：`TestQuickRollbackCompetesWithCompletionAndOtherRollbacks`<br>`TestPagedExecutionResultsStayOnOriginalDetailsAndCurrentOrder` |
| AC-013 | #87 | [2026-09-10-rollback-reason-history.md](2026-09-10-rollback-reason-history.md) | PASS：`TestRollbackReasonCanBeCorrectedByExecutorOrAdministrator` |
| AC-014 | #84 | [2026-09-10-multitable-execution.md](2026-09-10-multitable-execution.md) | PASS：`TestQuickRollbackUsesCurrentRolesAndRequiresReviewedVersionDigestAndCurrentRole`<br>`TestReleaseHeaderAndDetailPagesRequireOneWholeOrderVersion` |
| AC-015 | #88 | [最终清理与升级](2026-09-11-release-final-cleanup.md) | PASS：`TestReleaseSchemaStagesRecoverWithoutRewritingLegacyBusinessFacts`<br>`TestReleaseWritesRequireExplicitTableOnEveryDetail`<br>`TestPagedExecutionResultsStayOnOriginalDetailsAndCurrentOrder` |

最终普通详情和结果真正按服务端页读取，编辑/复制按明确用途收集同一版本完整申请。跨页版本变化拒绝混页，当前主单与权限独立于旧幂等确认；这些读路径与原有整单事务、千条容量一并验证。

[最终报告](2026-09-11-release-final-cleanup.md)记录全部 Go 模块测试和构建、Web 398/398 与开发服务器 2/2、335 项 integration 清单及实际复验来源、正式三引擎 34 范围和隔离 Compose 六场景。浏览器同一多表原单贯通管理员管控键设置、25 项分页准备、独立审批发布、原单倒序回滚与实际执行人原因补填；桌面和 390px 的操作/结果、错误保留及原键全文重推均有证据。

AC-015 另外核对所选 main 的 22 份已发布迁移/冻结输入哈希不变，新增分阶段可恢复 Goose 00004/00005；真实权限故障、显式 baseline/recover、全结构只读 Ready 和离线 release-reset 保护保留。不重写旧业务发布单 JSON，不清理现有环境数据。所有测试使用任务自有隔离库并串行；早期资源故障和每次真实测试失败在原始记录中保留。
