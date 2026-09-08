# 发布单验收用例索引（AC-001～AC-048）

范围为[父规格 #48](https://github.com/asherzj/relational-config-center/issues/48)确认的管理台正式发布、同表批量、人工审批及受保护回滚。父规格保持开放；工单交付不等于合并或部署。

下表按用例的主要交付工单索引证据。T1～T7 已分别完成验证、双轴评审、推送和关闭；链接固定到当时已验收的提交及对应文档行。早期证据记录当时的阶段行为，旧直写入口及单条容量限制已分别在 T5、T6 删除，相关记录并发回归迁入正式发布入口。最终候选的回归及远端 CI 由 T8 证据补充。

T8 的本地完整检查已通过，最终提交的远端 CI 及功能完成记录仍待确认；以下分别标明已有实际证据与剩余门禁。

## T1：全局账号角色

[工单 #49](https://github.com/asherzj/relational-config-center/issues/49) · 已关闭 · 固定提交 `1ea7f4441605141b59b30b38b6f92751ab4221c4`。

| 用例 | 验证的行为与入口 | 固定证据 |
|---|---|---|
| AC-001 | `TestRegisteredAccountIsViewer` 和 `TestAccountRoleUpgradePreservesAccountsAndGrants`：新注册及迁移存量账号均为 VIEWER；旧会话可查询，无相应角色的写操作返回 403；迁移重跑不覆盖已授予角色。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/1ea7f4441605141b59b30b38b6f92751ab4221c4/docs/verification/2026-09-07-account-roles.md#L13) |
| AC-002 | `TestMaintainerBootstrapsAdministratorAndAssignsRoles`：真实 `grant-admin` 只授予指定账号，重复授予不重复变更。`make test-browser` 从默认 VIEWER 注册开始，经维护命令初始化 ADMIN，再在角色界面为独立账号组合分配 EDITOR、APPROVER。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/1ea7f4441605141b59b30b38b6f92751ab4221c4/docs/verification/2026-09-07-account-roles.md#L14) |
| AC-003 | `TestCurrentSessionUsesRolePermissionMatrix`：五角色逐项验证现有目录和记录入口，角色变化后复用原 Cookie/CSRF；浏览器独立账号也用原会话读到新角色。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/1ea7f4441605141b59b30b38b6f92751ab4221c4/docs/verification/2026-09-07-account-roles.md#L15) |
| AC-004 | 同一权限矩阵验证仅 ADMIN 可管理规则和角色；伪造角色头无效；通用发现、查询和写入拒绝 `rcc_` 控制表。共享服务的请求身份和 HTTP 依赖方向由机器检查保护。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/1ea7f4441605141b59b30b38b6f92751ab4221c4/docs/verification/2026-09-07-account-roles.md#L16) |
| AC-005 | `TestRoleChangeIsAuditedAndIdempotent` 验证永久操作者/目标、前后角色、时间、原请求重放及同键不同内容冲突；`TestRoleChangeRaceAndAuditFailureDoNotLeavePartialGrants` 用真实 MySQL 触发器拒绝审计，证明 503 后角色、版本、历史无部分提交，并验证两个同版本请求仅一成功。浏览器验证历史中的永久操作者，并沿用进程日志与浏览器存储保密检查。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/1ea7f4441605141b59b30b38b6f92751ab4221c4/docs/verification/2026-09-07-account-roles.md#L17) |
| AC-006 | `TestLastEnabledAdministratorCannotBeRemovedOrDisabled`、`TestRoleChangesAndAccountMaintenanceAreConcurrentSafe`：普通撤权和维护停用不能移除最后一个启用 ADMIN；并发仅一个操作成功；显式 enable/grant-admin 可按文档恢复。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/1ea7f4441605141b59b30b38b6f92751ab4221c4/docs/verification/2026-09-07-account-roles.md#L18) |

## T2：统一记录版本

[工单 #33](https://github.com/asherzj/relational-config-center/issues/33) · 已关闭 · 固定提交 `18262bcf8585e3f7221a249a64266a7ae8190b75`。

| 用例 | 验证的行为与入口 | 固定证据 |
|---|---|---|
| AC-007 | `TestRecordVersionLegacyBaseline` 返回独立 `record_versions:["0"]`；`TestRecordVersionSnapshotAndIndependentResources` 在真实 RR 会话中与另一 HTTP 写入交错，旧数据/旧版本一起保留，新查询取得新数据/新版本。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/18262bcf8585e3f7221a249a64266a7ae8190b75/docs/verification/2026-09-07-record-versions.md#L9) |
| AC-008 | `TestRecordVersionRealConcurrentWriters` 真实并发初始化/修改只有一个 200，另一个 409；修改/删除最多一个生效。`TestRecordVersionCompareAndSwap` 旧版本修改和删除均拒绝。不同记录不等待另一记录的持有事务。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/18262bcf8585e3f7221a249a64266a7ae8190b75/docs/verification/2026-09-07-record-versions.md#L10) |
| AC-009 | `TestRecordVersionAddDeleteRecreateAndRollback` 新增 1、删除 2、重建 3；旧版本拒绝，CHECK 失败不改变行与版本；已删除单独返回 404。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/18262bcf8585e3f7221a249a64266a7ae8190b75/docs/verification/2026-09-07-record-versions.md#L11) |
| AC-010 | 真实 `utf8mb4_0900_ai_ci`、`utf8mb4_unicode_ci`、`utf8mb4_0900_bin`、DECIMAL、TIMESTAMP fixture 验证等价主键共享版本与不同主键独立；首次缺失版本竞争、控制存储失败、维护基线超过 2^53、非事务业务表拒绝、迁移重跑及缺表/非事务控制表就绪失败。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/18262bcf8585e3f7221a249a64266a7ae8190b75/docs/verification/2026-09-07-record-versions.md#L12) |
| AC-011 | 版本必填 422，旧版本 409；API 契约拒绝缺失、无效、长度不匹配的版本元数据并保持无损字符串。Web 测试验证保留输入和旧差异、只读查看最新值、显式重建后另一次确认。真实浏览器在另一 HTTP 写入后取得 409，按同一路径重新确认，MySQL 最终内容和永久操作人均核实。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/18262bcf8585e3f7221a249a64266a7ae8190b75/docs/verification/2026-09-07-record-versions.md#L13) |

## T3：持久发布草稿

[工单 #50](https://github.com/asherzj/relational-config-center/issues/50) · 已关闭 · 固定提交 `b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c`。

| 用例 | 验证的行为与入口 | 固定证据 |
|---|---|---|
| AC-012 | `TestReleaseDraftSaveAndReload`、`TestReleaseDraftDiffAndServerBaseline`、`TestReleaseDraftMissingIdentityAndTombstone` 通过认证 HTTP 保存 ADD/MODIFY/DELETE，校验永久申请人、服务器 before、独立版本及重读；草稿保存不改变业务行，不创建记录版本、不占用目标。真实浏览器从数据页保存后直接查询 MySQL 对应业务行仍为 Alpha、版本仍为 0。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c/docs/verification/2026-09-07-release-drafts.md#L9) |
| AC-013 | `TestReleaseDraftCurrentAuthorizationAndListing` 证明 EDITOR/ADMIN、VIEWER、仅 PUBLISHER、他人及撤权后的授权；`TestReleaseDraftCASCancelAndIdempotency` 和真实并发测试证明旧版本不能覆盖。Web 与真实两窗口验证冲突保留输入、先查看最新单据、明确重建后另发请求。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c/docs/verification/2026-09-07-release-drafts.md#L10) |
| AC-014 | `TestReleaseDraftDiffAndServerBaseline` 证明 value、SQL NULL、空字符串、未提交、不存在、生成列、自动字段待执行的不同语义；before 和自动字段伪造被拒绝。`TestReleaseDraftRejectsLossySnapshotAndRetainsSavedSchema` 验证无法无损快照的 BLOB 表前置拒绝、操作人字段容量不足拒绝，以及保存后 Schema 改名仍可读完整旧差异。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c/docs/verification/2026-09-07-release-drafts.md#L11) |
| AC-015 | 授权/列表测试覆盖表名、申请人、状态、单号、游标及 404。`TestReleaseDraftReplayUsesCurrentActionsAndRejectsChangedDigest` 证明原结果重放保留原业务版本，但动作按当前单据状态重新求值。页面提供正式列表/详情、筛选、历史和状态动作。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c/docs/verification/2026-09-07-release-drafts.md#L12) |
| AC-016 | CAS/取消测试验证原因、版本推进、原键重放、再次取消新键拒绝、无物理 DELETE 接口；真实浏览器取消后刷新，仍能筛选和查看历史，VIEWER 无编辑/取消动作。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c/docs/verification/2026-09-07-release-drafts.md#L13) |
| AC-017 | 创建/编辑/取消都按永久账号、操作与请求键去重，内容摘要冲突明确拒绝；`TestReleaseDraftConcurrentAndAtomicStorage` 验证同键并发只有一单、不同键 CAS、数据库故障整笔回滚及 Admin 新实例恢复。真实浏览器让服务器实际成功后中断响应，刷新使用原键原内容找回同一单。Web 验证账号隔离、后续 403 仍保留原请求、明确版本/状态拒绝后才解除未知状态并要求核对重建。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/b4f5adc1ed4fd4bf35fd9429a10ee5c9b030a25c/docs/verification/2026-09-07-release-drafts.md#L14) |

## T4：审批和在途目标

[工单 #51](https://github.com/asherzj/relational-config-center/issues/51) · 已关闭 · 固定提交 `faaf0b1243116aa9ae60fa74f3b25b3163a38ce5`。

| 用例 | 验证的行为与入口 | 固定证据 |
|---|---|---|
| AC-018 | `TestReleaseSubmitFreezesIntent`、`TestReleaseSubmitRevalidatesBaselineAndRules`、`TestReleaseFreezeTracksExecutionSemantics`、`TestReleaseFreezeMetadataVisibility`、`TestReleaseFreezeMetadataGrantNameIdentity`、`TestReleaseFreezeMetadataCaseInsensitiveNames`、`TestReleaseExecutionSchemaHoldsMetadataLock` | [原始证据](https://github.com/asherzj/relational-config-center/blob/faaf0b1243116aa9ae60fa74f3b25b3163a38ce5/docs/verification/2026-09-08-release-approvals.md#L7) |
| AC-019 | `TestReleaseTargetsCompeteAndCancelReleases`（Résumé/RESUME 真主键等价）；`TestReleaseWorkflowAtomicityAndCompetition`（结果存储失败回滚）；`TestReleaseAutoIncrementZeroIdentity`（生成型 0 拒绝，NO_AUTO_VALUE_ON_ZERO 下真实 0 占用） | [原始证据](https://github.com/asherzj/relational-config-center/blob/faaf0b1243116aa9ae60fa74f3b25b3163a38ce5/docs/verification/2026-09-08-release-approvals.md#L8) |
| AC-020 | `TestReleaseApprovalCurrentRolesAndHistory`；Web 仅 APPROVER 流程 | [原始证据](https://github.com/asherzj/relational-config-center/blob/faaf0b1243116aa9ae60fa74f3b25b3163a38ce5/docs/verification/2026-09-08-release-approvals.md#L9) |
| AC-021 | `TestReleaseRejectedCopyRechecksBaseline`；浏览器拒绝→读取新基线→复制 | [原始证据](https://github.com/asherzj/relational-config-center/blob/faaf0b1243116aa9ae60fa74f3b25b3163a38ce5/docs/verification/2026-09-08-release-approvals.md#L10) |
| AC-022 | 已批准取消/在途取消的上述 HTTP 用例；浏览器申请人取消已批准单 | [原始证据](https://github.com/asherzj/relational-config-center/blob/faaf0b1243116aa9ae60fa74f3b25b3163a38ce5/docs/verification/2026-09-08-release-approvals.md#L11) |
| AC-023 | `TestReleaseWorkflowAtomicityAndCompetition` 同版本批准/拒绝/取消三方竞争 | [原始证据](https://github.com/asherzj/relational-config-center/blob/faaf0b1243116aa9ae60fa74f3b25b3163a38ce5/docs/verification/2026-09-08-release-approvals.md#L12) |
| AC-024 | `TestReleaseApprovalCurrentRolesAndHistory`；浏览器真实提交后丢审批响应，刷新同键找回 | [原始证据](https://github.com/asherzj/relational-config-center/blob/faaf0b1243116aa9ae60fa74f3b25b3163a38ce5/docs/verification/2026-09-08-release-approvals.md#L13) |
| AC-025 | `TestReleaseApprovalCurrentRolesAndHistory` 当前角色撤权后拒绝新请求，旧审批保持 | [原始证据](https://github.com/asherzj/relational-config-center/blob/faaf0b1243116aa9ae60fa74f3b25b3163a38ce5/docs/verification/2026-09-08-release-approvals.md#L14) |

## T5：原子正式发布

[工单 #52](https://github.com/asherzj/relational-config-center/issues/52) · 已关闭 · 固定提交 `39c632aa7560f900d2a76724898efe4f117d4145`。

| 用例 | 验证的行为与入口 | 固定证据 |
|---|---|---|
| AC-026 | `TestReleasePublicationAddsFinalRow`、`TestPublicationIdentityCanonicalAndDelete`、`TestPublicationPublisherHistoryAndApprovalSurvivesRevocation`、账号权限矩阵：三种操作、合法状态、当前 PUBLISHER、审批人兼发布、申请人发布、审批撤权后合法审批保留 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L7) |
| AC-027 | `TestPublicationFloatValuesAndIdentityRoundTrip`、 `TestPublicationStoredRowsAreVerifiedBeforeReadOrReplay`、`TestReleasePublicationAddsFinalRow`、`TestPublicationSupportsTargetRowTrigger`、`TestCanonicalRowFixedLosslessValues`：真实默认/生成/纯目标行触发器、SQL NULL/JSON null/空串、固定字节与独立写定 checksum | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L8) |
| AC-028 | `TestPublicationFloatIdentityMaintenanceKeepsOldVersions`、 `TestPublicationIdentityCanonicalAndDelete`：真实删除 tombstone、before、版本墓碑与重建；既有 RecordVersion 删除/重建回归已迁移完整发布 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L9) |
| AC-029 | `TestPublicationFrozenChangesAndDescriptions`、`TestApprovedPublicationRechecksPolicyAndNextDraftUsesReplacement`：DDL/数据版本/停用与替换语义拒绝、显示元数据变化允许、原已批准状态保留 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L10) |
| AC-030 | `TestPublicationAtomicPersistenceFailures`：真实 MySQL 七处持久失败（记录版本、Command、表版本、通知、目标释放、单据历史状态、请求结果）全部回滚；既有业务CHECK/唯一/FK约束失败回归经发布执行 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L11) |
| AC-031 | `TestCommittedWritesRemainSingleWhenHTTPResponsesAreLost`、`TestPublicationCommitUnknownSurvivesExecutableRestart`：HTTP成功后丢包、MySQL真正COMMIT OK被丢弃、真正Admin可执行文件重启、同actor/action/key/body恢复及唯一持久计数；browser真实execute后abort/reload恢复 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L12) |
| AC-032 | `TestPublicationActionCompetitionAndTableOrder`：执行/取消、双执行至多一个动作；独立单的同表游标/版本连续且随事务提交 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L13) |
| AC-033 | `TestOldRecordWriteRoutesAreRemoved`：三旧路由404；Web ADD/MODIFY/DELETE确认均保存草稿；browser主路径已迁移发布，静态扫描仅剩旧路由负向/日志脱敏断言 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L14) |
| AC-034 | `TestPublicationPublisherHistoryAndApprovalSurvivesRevocation`、`TestConcurrentAccountsOwnTheirBusinessChanges`、`TestRevocationRejectsNewRequestsButAllowsAuthenticatedWriteToFinish`：A/B/C历史身份、实际发布人自动字段与数据库时间、真实锁障碍后撤销不更改在途身份 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L15) |
| AC-035 | `TestPublicationRejectsUnknownNonAutoIncrementIdentity`、`TestPublicationRejectsUntrackedCascade`、`TestPublicationRejectsImplicitWritesAndAuditSpoofing`、`TestPublicationSessionLocksHiddenForeignKeyDDL`：隐藏跨schema级联、跨行触发器/多句/主键/审计伪造拒绝；真实PublicationSession阻止添加FK，checks=1/0均实测1205，提交后同DDL成功 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L16) |
| AC-036 | `TestPublicationCommitUnknownSurvivesExecutableRestart`：真实元数据断连503/期限504/真实COMMIT确认丢失503待确认；恢复原键不重复；通知NOT_CONNECTED且没有远端投递 | [原始证据](https://github.com/asherzj/relational-config-center/blob/39c632aa7560f900d2a76724898efe4f117d4145/docs/verification/2026-09-08-atomic-publication.md#L17) |

## T6：同表混合批量

[工单 #53](https://github.com/asherzj/relational-config-center/issues/53) · 已关闭 · 固定提交 `f6984f04ca1e049db64d18f4315b72461cffeca8`。

| 用例 | 验证的行为与入口 | 固定证据 |
|---|---|---|
| AC-037 | `TestReleaseMixedBatchPublication`、`TestReleaseBatchExistingEnumPrimaryKeys`、`TestReleaseThousandItemsThroughExecutable`：同表 MODIFY/DELETE/ADD 集合、正式进程默认 4s 下 1,000 项。`release-batches.cjs`：数据页加入本人同表草稿、明确多行删除、任意明细编辑/移除、完整大单分页与审批发布。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/f6984f04ca1e049db64d18f4315b72461cffeca8/docs/verification/2026-09-08-mixed-batch.md#L9) |
| AC-038 | `TestReleaseBatchDuplicateIdentityDoesNotReplaceDraft`；`TestReleaseBatchEdgeDraftValidation` / `RequestLimitsPreserveDraft`：非法后项、数值/字符/PAD SPACE 等价重复、跨表、0/1,001 项与正文限制，原草稿逐字不变；`CopyInvalidItemIsLocated` / `FieldBudget` / `ExpandedResultBudgetRollsBack` / `BudgetRetainsCancellationHeadroom`：64 KiB 字段、8 MiB 实际结果及终止余量。HTTP `error.item_index`，Web 错误定位与发送前浏览器容量拒绝。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/f6984f04ca1e049db64d18f4315b72461cffeca8/docs/verification/2026-09-08-mixed-batch.md#L10) |
| AC-039 | `TestReleaseBatchEdgeOverlappingSubmissions`：交叉目标竞争唯一赢家、失败零残留、取消释放及原失败键重试；`StaleMemberRollsBack` / `MidwayConstraintRollback`：后项陈旧、真实 CHECK/业务唯一性中途失败，无业务/记录版本/Command/表版本/通知部分生效，APPROVED 与占用保留。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/f6984f04ca1e049db64d18f4315b72461cffeca8/docs/verification/2026-09-08-mixed-batch.md#L11) |
| AC-040 | `TestReleaseBatchEdgeAutoIncrementAndIndependentRequests` / `IndependentUniqueConflict`：真实 increment=3/offset=2 的逐项 id，原键结果相同，独立相似请求不合并，业务唯一约束真实竞争全批回滚；正式 1,000 项和浏览器丢执行响应恢复不重复新增。 | [原始证据](https://github.com/asherzj/relational-config-center/blob/f6984f04ca1e049db64d18f4315b72461cffeca8/docs/verification/2026-09-08-mixed-batch.md#L12) |

## T7：重新审批的回滚

[工单 #54](https://github.com/asherzj/relational-config-center/issues/54) · 已关闭 · 固定提交 `ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087`。

| 用例 | 验证的行为与入口 | 固定证据 |
|---|---|---|
| AC-041 | `TestReleaseRollbackMixedPublication`：ADD/MODIFY/DELETE 反向完整正式流程、未审批执行拒绝、恢复业务值/实际 id、记录版本 2、原单 ROLLED_BACK、2 通知/6 Command/零目标，原正向键仍返回旧成功快照；`TestReleaseThousandItemsThroughExecutable` 在同一正式进程/default 4s 中追加全部 1,000 项 inverse、恢复 666 原行并删除 334 原 ADD `TestReleaseRollbackUnwindsUniqueValueDependencies`：原 DELETE/MODIFY 先释放唯一值、后项 ADD 复用，回滚按实际执行逆序撤销，恢复原唯一值所有者；两个用例最初均 409 `duplicate_key`，修复后真实 MySQL 17.267s 通过（命令 19.066s） `TestReleaseRollbackRestoresDeletedEnumIdentity`：两项已删 ENUM 逆序恢复各自的墓碑身份、版本 2且只有两个版本位置；`TestReleaseRollbackAssociationFailuresAreAtomic`：反向单 INSERT 故障不留下原关联，反向 DML/Command/版本/通知后更新原单 ROLLED_BACK 故障全部回退，去除故障后同键成功 | [原始证据1](https://github.com/asherzj/relational-config-center/blob/ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087/docs/verification/2026-09-08-approved-rollback.md#L7)、[原始证据2](https://github.com/asherzj/relational-config-center/blob/ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087/docs/verification/2026-09-08-approved-rollback.md#L8)、[原始证据3](https://github.com/asherzj/relational-config-center/blob/ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087/docs/verification/2026-09-08-approved-rollback.md#L9) |
| AC-042 | `TestReleaseRollbackRestoresDeletedEnumIdentity`：两项已删 ENUM 逆序恢复各自的墓碑身份、版本 2且只有两个版本位置；`TestReleaseRollbackAssociationFailuresAreAtomic`：反向单 INSERT 故障不留下原关联，反向 DML/Command/版本/通知后更新原单 ROLLED_BACK 故障全部回退，去除故障后同键成功 `TestReleaseRollbackRejectsLaterChanges`：申请前、提交前、批准后发生后项陈旧、真实删除重建和维护版本推进，整单拒绝；preview 新版本不能经 edit/submit 洗掉原基线；`TestReleaseRollbackRejectsNewUniqueConstraintAtomically`：新唯一约束在第二项失败，第一项/版本/Command/通知/请求全回退，原单不标记，批准与全部占用保留 | [原始证据1](https://github.com/asherzj/relational-config-center/blob/ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087/docs/verification/2026-09-08-approved-rollback.md#L9)、[原始证据2](https://github.com/asherzj/relational-config-center/blob/ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087/docs/verification/2026-09-08-approved-rollback.md#L10) |
| AC-043 | `TestReleaseRollbackRestoresBusinessFieldsWithNewAudit`：小动作请求恢复两项各 200,000 字节历史业务字段、SQL NULL/JSON 与 TIMESTAMP 微秒，原来/当前自动字段不作为用户输入，当前生成列重算、本次执行人/时间明确；`TestReleaseRollbackRestoresStoredTimeDuration`：真实 `-120:30:40.123456` 恢复；`TestReleaseRollbackRejectsChangedRestoreValue`：合法 BEFORE 触发器令 10 变 11 时拒绝且整笔回退 | [原始证据](https://github.com/asherzj/relational-config-center/blob/ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087/docs/verification/2026-09-08-approved-rollback.md#L11) |
| AC-044 | `TestReleaseRollbackConcurrencyAndReapplication`：同键/不同键并发申请和执行收敛、取消/拒绝后重新申请、双向/历史关联、禁止反向编辑复制、成功后重复申请拒绝、旧申请键保留当时结果；`TestReleaseRollbackUsesCurrentIndependentRoles`：当前 EDITOR、非申请人 EDITOR、他人审批、自批拒绝、PUBLISHER 与实时撤权，永久身份保留 `TestReleaseRollbackPendingHistoryCanTerminate`：在旧 8MiB−64KiB 门禁内初始化有序且版本一致的提交前 EDIT 历史，经 8 次真实申请/取消后第 9 次接受 pending 原单接近新门禁（只余约 36 字节），最长 2,000 个 `<` 理由取消/拒绝均成功，关联/目标释放 | [原始证据1](https://github.com/asherzj/relational-config-center/blob/ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087/docs/verification/2026-09-08-approved-rollback.md#L12)、[原始证据2](https://github.com/asherzj/relational-config-center/blob/ab7414ea96ffd89cb7ba1ac67e35a4297f1ba087/docs/verification/2026-09-08-approved-rollback.md#L13) |

## T8：存量升级与功能验收

[工单 #55](https://github.com/asherzj/relational-config-center/issues/55) · 进行中。下面列出该工单的责任和完成证据要求，当前不标记通过。

| 用例 | 必须验证的行为 | 最终证据 |
|---|---|---|
| AC-045 | 保留旧账号、会话规则、稳定身份、Policy 和业务数据；默认只读、显式首位 ADMIN；中断/重复迁移不破坏数据或重复授权，控制结构不完整时拒绝服务写入，旧调用方无直写能力 | `TestAccountUpgradeFromLegacyMatchesFreshSchema`、`TestAccountRoleUpgradePreservesAccountsAndGrants`；完整 MySQL 493 项测试及子测试通过，零失败/跳过 |
| AC-046 | 独立申请、审批、发布账号在真实浏览器完成正向与反向发布；刷新/登录恢复、冲突输入保留、当前角色限制 | 完整 `make test-browser-acceptance` 退出 0；Chromium/Firefox/WebKit 各实际回滚 4 项、账号/冲突/执行恢复 21 项；另保留 Go 浏览器入口通过 |
| AC-047 | 正式 Go/Web/真实 MySQL/浏览器目标、构建、架构和契约检查实际执行并通过；固定基点两轴评审；最终提交对应的远端 CI 通过 | Go 测试/构建、Web 296 项及类型/构建/开发配置、完整 MySQL 与两条浏览器入口通过；独立审查无硬性阻断；最终签核及远端 CI 待 #55 记录 |
| AC-048 | 全部状态的完整差异、意见、永久身份及关联在重启/账号资料变化后可追溯；无公开物理删除、改写或自动清理入口 | `TestReleaseHistorySurvivesExecutableRestartAndExternalChanges` 在完整 MySQL 中通过，覆盖七种状态、实际重启、资料变更、拒绝删除及伪造改写后的逐项一致读取 |

以上四项统一由 T8 的 `docs/verification/2026-09-08-release-delivery.md` 记录最终候选、本地检查和实际浏览器证据。最终远端 CI 链接同时记入 #55 完成评论，避免为补充 CI 链接另建未经 CI 验证的提交。

## 本期边界

通知只持久化为 NOT_CONNECTED。实际通知 worker、Server/Client 分发、灰度、Environment、跨表发布、版本大盘和历史自动清理均不在本期；项目总表按实际 Admin 范围记录完成状态。
