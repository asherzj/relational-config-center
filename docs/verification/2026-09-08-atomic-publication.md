# T5 / #52 原子发布验收

固定基点 `faaf0b1243116aa9ae60fa74f3b25b3163a38ce5`，分支 `codex/issue-52-atomic-publication`。范围 AC-026～036；单明细限制继续归 T6。管理台成功状态只表示数据库已提交，分发状态明确为 NOT_CONNECTED。

| AC | 实际验收入口与观察 |
| --- | --- |
| AC-026 | `TestReleasePublicationAddsFinalRow`、`TestPublicationIdentityCanonicalAndDelete`、`TestPublicationPublisherHistoryAndApprovalSurvivesRevocation`、账号权限矩阵：三种操作、合法状态、当前 PUBLISHER、审批人兼发布、申请人发布、审批撤权后合法审批保留 |
| AC-027 | `TestPublicationFloatValuesAndIdentityRoundTrip`、 `TestPublicationStoredRowsAreVerifiedBeforeReadOrReplay`、`TestReleasePublicationAddsFinalRow`、`TestPublicationSupportsTargetRowTrigger`、`TestCanonicalRowFixedLosslessValues`：真实默认/生成/纯目标行触发器、SQL NULL/JSON null/空串、固定字节与独立写定 checksum |
| AC-028 | `TestPublicationFloatIdentityMaintenanceKeepsOldVersions`、 `TestPublicationIdentityCanonicalAndDelete`：真实删除 tombstone、before、版本墓碑与重建；既有 RecordVersion 删除/重建回归已迁移完整发布 |
| AC-029 | `TestPublicationFrozenChangesAndDescriptions`、`TestApprovedPublicationRechecksPolicyAndNextDraftUsesReplacement`：DDL/数据版本/停用与替换语义拒绝、显示元数据变化允许、原已批准状态保留 |
| AC-030 | `TestPublicationAtomicPersistenceFailures`：真实 MySQL 七处持久失败（记录版本、Command、表版本、通知、目标释放、单据历史状态、请求结果）全部回滚；既有业务CHECK/唯一/FK约束失败回归经发布执行 |
| AC-031 | `TestCommittedWritesRemainSingleWhenHTTPResponsesAreLost`、`TestPublicationCommitUnknownSurvivesExecutableRestart`：HTTP成功后丢包、MySQL真正COMMIT OK被丢弃、真正Admin可执行文件重启、同actor/action/key/body恢复及唯一持久计数；browser真实execute后abort/reload恢复 |
| AC-032 | `TestPublicationActionCompetitionAndTableOrder`：执行/取消、双执行至多一个动作；独立单的同表游标/版本连续且随事务提交 |
| AC-033 | `TestOldRecordWriteRoutesAreRemoved`：三旧路由404；Web ADD/MODIFY/DELETE确认均保存草稿；browser主路径已迁移发布，静态扫描仅剩旧路由负向/日志脱敏断言 |
| AC-034 | `TestPublicationPublisherHistoryAndApprovalSurvivesRevocation`、`TestConcurrentAccountsOwnTheirBusinessChanges`、`TestRevocationRejectsNewRequestsButAllowsAuthenticatedWriteToFinish`：A/B/C历史身份、实际发布人自动字段与数据库时间、真实锁障碍后撤销不更改在途身份 |
| AC-035 | `TestPublicationRejectsUnknownNonAutoIncrementIdentity`、`TestPublicationRejectsUntrackedCascade`、`TestPublicationRejectsImplicitWritesAndAuditSpoofing`、`TestPublicationSessionLocksHiddenForeignKeyDDL`：隐藏跨schema级联、跨行触发器/多句/主键/审计伪造拒绝；真实PublicationSession阻止添加FK，checks=1/0均实测1205，提交后同DDL成功 |
| AC-036 | `TestPublicationCommitUnknownSurvivesExecutableRestart`：真实元数据断连503/期限504/真实COMMIT确认丢失503待确认；恢复原键不重复；通知NOT_CONNECTED且没有远端投递 |

迁移旧正向验收时，`publicationFixtureRequest` 接受显式 operation/table/id，并逐步发出真实建单、提交、独立审批与执行。它返回实际失败或成功HTTP响应，不解析旧URL、不伪造旧响应；旧 ordinary Policy 测试显式读当前版本，CAS 测试仍传固定基线。为使独立失败断言可复用同目标，失败夹具先断言 APPROVED/原版本后明确取消；原子失败专用测试直接执行且断言占用仍保留。旧错误400中仅动态JSON类型仍400，合法请求但业务内容无效按照发布契约变为422。

原 ManagedTableMutation、MutationSnapshotSession/Executor、直接行写适配器与其无入口DTO全部删除。原5个内部mock快照用例的自动字段、默认/非法内容、权限与策略状态保证由同名业务政策真实MySQL发布用例及A/B/C测试接替，不保留伪造旧成功调用。原内部mutationSnapshotBarrier已移除；认证撤销用实际InnoDB锁等待证明在途请求已认证。原HTTP安全测试的panic改为对生产恢复middleware的真实路由边界测试；不可用/超时/未知的保证由真实MySQL系统验收接替。记录主键等价、最大版本、删除重建、维护代际及不同记录查询快照保证继续执行原用例名；实际发布的同表提交串行、不同表独立由真实HTTP双请求和MySQL外部进度锁验证。

真实故障切片首先发现 metadata socket timeout 会被误归503。修复为发布请求期限短于socket；随后测试自己的stall代理需传播client真实FIN，否则上个未清理事务持有原请求锁，已修正为观察FIN而非固定睡眠。最终 wire 切片实际 exit0（12.111s）。其他定向：空ID/uint64/能力/旧错误迁移；A/B/C/冻结/PublicationSession MDL 27.311s；账号迁移7项61.038s及新增控制表9.380s；Web发布结果/恢复27例通过。最终增加持久文档损坏 checksum、Schema digest、Command ID 的拒绝验证，以及 NO_AUTO_VALUE_ON_ZERO 实际发布；主键等价/损坏恢复/真实重启四项定向实际 exit0（39.938s）。浏览器旧 ENUM 写失败场景在新草稿阶段不会执行数据库约束：该场景改用真实缺必填字段 422 验证输入与错误保留；ENUM/CHECK/FK/唯一约束失败仍由真实 MySQL 发布执行回归覆盖。

## Standards

独立只读 agent `review_standards`，gpt-5.6-sol / high，固定基点与完整 diff 如上。初审的持久行未校验（硬性）、重复发布语义比较（主观）及旧 DTO（主观）全部接受：所有详情、列表和原键重放统一验证冻结 Schema 与 canonical before/final；提交/执行共用语义构造；旧内部直写路径及 DTO 删除。复审补充 Command ID 未绑定实际行与 README 测试预算过时，两项也已修复：发布写入与持久校验共用 `RecordID()`，真实改坏 ID 的 red/green 验证通过；预算统一 40/45 分钟。最终复审 0 遗留。

## Spec

独立只读 agent `review_spec`，gpt-6-astra / high，固定基点与完整原始 #48/#52、规格/计划/上下文。初审三项全部接受：非自增 DEFAULT 身份无法确定须在草稿准备前拒绝（显式提供 ID 可发布）；旧内部直写和正向测试删除并迁移真实发布；停用策略验收改为独立无数据冲突的新批准单以确实命中停用拒绝。`TestPublicationRejectsUnknownNonAutoIncrementIdentity` 真实 red/green、旧路径静态扫描与完整迁移、`TestPublicationFrozenChangesAndDescriptions` 的独立停用场景验证闭环。最终复审 0 遗留。

Standards 初审与首次复审共 5 项均已修复，完整回归后补充的并发失败阶段断言也已修复，累计 6 项、最终 0；Spec 初审 3 项与后续 404 必须绑定 DELETE 赢家的断言共 4 项均已修复，最终 0。浮点生产代码、真实验收与维护文档再经两轴独立复审，仍各 0。最后的 integration 夹具字段清理、Web 禁止自动保存断言、两个浏览器按钮/准备失败迁移及截图收尾，也经同一两轴独立复审，仍各 0 遗留。两条轴都未将运行中的完整套件算为通过。

## 最终验证与交付

首轮完整运行后端 SHA-256（本轮失败，不作为交付绿灯）：`177be53fbb910dd978969739a0b3722eb94e01555d477d2a764ee1745d1b6fa9`（123 个 Admin、迁移及正式构建/CI 入口文件；完整 integration 入口列出 204 项测试，其中非 browser 集成文件有 159 个顶层用例，2026-09-08 03:11:44 Asia/Shanghai）。更早的一次启动在 integration 专用 HTTP 夹具发现已删旧字段的残留引用，立即中断并修正；全部 integration 包编译通过后才进行了上述首次完整运行。没有把首轮失败或编译专用 `-run '^$'` 当作运行验收。

| 正式命令 | 实际结果 |
| --- | --- |
| `make test` | exit 0；全部 Go 模块测试，含公共路由、HTTP 依赖方向、共享服务身份隔离与草稿不可写业务行的 AST 检查 |
| `make build` | exit 0；Admin、Server、Client、Shared 及 account-maintain |
| `pnpm --dir web test:run` | exit 0；23 文件、217 项，13.76s |
| `pnpm --dir web typecheck` | exit 0 |
| `pnpm --dir web build` | exit 0 |
| `make test-integration` | exit 0；最终冻结完整真实 MySQL：Admin 1416.765s、HTTP 82.081s，全部包通过；首轮失败不计通过 |
| `make test-browser` | exit 0；浮点修复后最终 72.453s，主路径及四个历史验收脚本全部通过，含 390px 与未知状态截图 |
| `git diff --check` | exit 0 |

数据库正式命令统一设置 `DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock` 和 `TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock`。MySQL 8.4、真实 Admin 可执行文件、Chrome/Vite，均使用隔离夹具；未跳过缺失依赖，没有投递 worker。Go 完整套件预算 40 分钟、CI 45 分钟：T4 已需 1340.976s，本单新增真实容器/故障/并发切片并完整迁移旧保障，增加运行余量，不删减验收。

旧入口搜索覆盖 Admin、Web、E2E、scripts 中 Go/TS/TSX/JS/CJS/MJS/Python/Shell；`/rows` 只剩负向拒绝、日志脱敏或“禁止调用”的测试，旧构造器/业务写适配器均无正向引用。完整临时结构退出：T6 / #53 移除单明细限制；T7 / #54 从真实 canonical before、实际 ID 与 post-record-version 生成受保护反向单；T8 / #55 汇总跨工单验收。

浏览器截图：[发布结果桌面](2026-09-08-publication-result-desktop.png)、[发布结果 390px](2026-09-08-publication-result-mobile.png)、[真实执行成功后响应丢失的待确认状态](2026-09-08-publication-unknown.png)。窄屏主导航已隐藏，无水平溢出；实际 ID、记录/表版本、Schema/checksum、永久执行人、数据库时间与 NOT_CONNECTED 文案可见。补拍中一次在注册后的导航等待超时；添加真实注册 201 断言及失败截图后，最终正式入口全部通过。

首轮完整回归唯一失败为 `TestRecordVersionRealConcurrentWriters`：两个固定基线请求竞争时，败者可能在提交阶段先遇目标占用冲突，而原断言仅接纳记录版本冲突。修复保持两个原始基线不刷新；明确区分 DRAFT v1 的提交拒绝与真实夹具核实 APPROVED v3 后显式取消为 CANCELLED v4 的执行拒绝，只在赢家 DELETE 时允许记录不存在；严格验证唯一成功单/Command/通知、表版本/游标、记录版本、零残留占用及败者历史。初始加强版真实重复 3 次 25.593s，收紧版重复 3 次 25.778s；最终阶段断言与浮点切片合跑 27.240s 实际通过。

独立真实 MySQL 发现 `CAST(FLOAT AS CHAR)` 只输出六位有效数：普通字段丢位，返回 ID 不能往返，两个真实主键会得到同一旧权重。新增切片先实际 red（18.656s），再以真实 32 位解析、提升 DOUBLE 后读取、正负零身份归一修复。查询、草稿 before、canonical final 与缺行/存在主键身份共同使用这一存储语义；普通值和相邻主键 ADD→返回 ID 查询→MODIFY、0.1 输入，以及 FLOAT/DOUBLE -0 删除后 0 重建均验收。旧 FLOAT 短 key 及 DOUBLE -0 key 的版本 5→维护基线 6→新身份连续推进，旧 key 保留 5。升级必须取消旧在途单、停写并按 ADR-0021 推进维护基线，详见记录版本维护门禁，不能静默重置版本。

另 `account_delivery_integration_test.go` 的三行 012 迁移仅做 gofmt；去位置 AST（保留 build tag、注释、导入、字面量与操作）一致，`/private/tmp/t5-format-equivalence.log` exit 0。这项排版调整已纳入最终新冻，不与失败首轮字节混淆。

最终新冻结：`b263584b3ef30ed53887cbfb61bfddc3625069655e807eb8d8563a1d10ef7de0`，2026-09-08 03:57:46 Asia/Shanghai，124 个 Admin/迁移/正式入口文件，161 个非 browser 集成文件顶层用例；完整 integration 入口共 206 项。此冻结包含上述全部语义与格式修复。浮点修复后的 `make test`、`make build` 与所有 integration 包编译均实际 exit 0；正式完整套件开始后没有再修改这些冻结文件。完整 `make test-integration` 已实际 exit 0，Admin 1416.765s、HTTP 82.081s；收取退出状态后重新核对 124 个文件的逐文件 SHA-256 全部一致。
