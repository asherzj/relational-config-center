# T7 / #54 审批回滚验收

固定基点 `f6984f04ca1e049db64d18f4315b72461cffeca8`。主入口是已认证 Admin HTTP；真实 MySQL 独立证明身份、竞争与事务，浏览器验证需要可见接通的路径。独立工作树 `.worktrees/release-order-t7`，分支 `codex/issue-54-approved-rollback`。恢复阶段完整 Go/Web/真实 MySQL/浏览器检查均已实际退出 0，没有依赖缺失 skip 或内部协作者 mock。下方保留首轮完整回归失败及中断丢失结果的真实历史。

| AC | 用例与观察 |
| --- | --- |
| AC-041 | `TestReleaseRollbackMixedPublication`：ADD/MODIFY/DELETE 反向完整正式流程、未审批执行拒绝、恢复业务值/实际 id、记录版本 2、原单 ROLLED_BACK、2 通知/6 Command/零目标，原正向键仍返回旧成功快照；`TestReleaseThousandItemsThroughExecutable` 在同一正式进程/default 4s 中追加全部 1,000 项 inverse、恢复 666 原行并删除 334 原 ADD |
| AC-041/逆序 | `TestReleaseRollbackUnwindsUniqueValueDependencies`：原 DELETE/MODIFY 先释放唯一值、后项 ADD 复用，回滚按实际执行逆序撤销，恢复原唯一值所有者；两个用例最初均 409 `duplicate_key`，修复后真实 MySQL 17.267s 通过（命令 19.066s） |
| AC-041/042 | `TestReleaseRollbackRestoresDeletedEnumIdentity`：两项已删 ENUM 逆序恢复各自的墓碑身份、版本 2且只有两个版本位置；`TestReleaseRollbackAssociationFailuresAreAtomic`：反向单 INSERT 故障不留下原关联，反向 DML/Command/版本/通知后更新原单 ROLLED_BACK 故障全部回退，去除故障后同键成功 |
| AC-042 | `TestReleaseRollbackRejectsLaterChanges`：申请前、提交前、批准后发生后项陈旧、真实删除重建和维护版本推进，整单拒绝；preview 新版本不能经 edit/submit 洗掉原基线；`TestReleaseRollbackRejectsNewUniqueConstraintAtomically`：新唯一约束在第二项失败，第一项/版本/Command/通知/请求全回退，原单不标记，批准与全部占用保留 |
| AC-043 | `TestReleaseRollbackRestoresBusinessFieldsWithNewAudit`：小动作请求恢复两项各 200,000 字节历史业务字段、SQL NULL/JSON 与 TIMESTAMP 微秒，原来/当前自动字段不作为用户输入，当前生成列重算、本次执行人/时间明确；`TestReleaseRollbackRestoresStoredTimeDuration`：真实 `-120:30:40.123456` 恢复；`TestReleaseRollbackRejectsChangedRestoreValue`：合法 BEFORE 触发器令 10 变 11 时拒绝且整笔回退 |
| AC-044 | `TestReleaseRollbackConcurrencyAndReapplication`：同键/不同键并发申请和执行收敛、取消/拒绝后重新申请、双向/历史关联、禁止反向编辑复制、成功后重复申请拒绝、旧申请键保留当时结果；`TestReleaseRollbackUsesCurrentIndependentRoles`：当前 EDITOR、非申请人 EDITOR、他人审批、自批拒绝、PUBLISHER 与实时撤权，永久身份保留 |
| AC-044/容量 | `TestReleaseRollbackPendingHistoryCanTerminate`：在旧 8MiB−64KiB 门禁内初始化有序且版本一致的提交前 EDIT 历史，经 8 次真实申请/取消后第 9 次接受 pending 原单接近新门禁（只余约 36 字节），最长 2,000 个 `<` 理由取消/拒绝均成功，关联/目标释放 |

开发期 red→green：混合请求最初 404 `route_not_found`；允许的触发器曾返回 200 但 quantity 没恢复，现 422 `rollback_restore_mismatch`；缺行 ENUM 曾 422 `release_snapshot_unsupported`，现恢复同一实际身份；纯 EDITOR 曾被 HTTP 默认管理员门禁拒绝，现正常申请；历史带符号 TIME 曾 422 `invalid_mutation_content`，现恢复完整值。容量问题最初 cancel/reject 都 422 `release_result_limit`，已修复 pending 与终止余量；最初两版注入 padding 仅为开发诊断，最终采用上述可达形状和真实增长循环。

Web 单测涵盖回滚理由/原键、双向关联、丢响应恢复、当前 ROLLED_BACK 刷新、反向提交恢复不 preview、数据页不接受反向 DRAFT 作为追加目标。新 `release-rollbacks.cjs` 已纳入正式 `make test-browser`，冻结后的真实 Chrome/MySQL 证据见下文。

初次正式 1,000 项逆转代表样本：申请 67B→1,725,911B / 322.77ms，提交 938.37ms，审批 223.51ms，反向执行 24B→3,314,336B / 1.41353s，原键重放 241.74ms。实际 666 行全部恢复 old、1,000 Record Version=2、2,000 Command、2通知、零目标；仅为该代表样本，不承诺任意字段体积或负载。

架构检查沿用 HTTP→Application、Go internal/服务身份隔离与 ReleaseOrderSession 不具备业务写权限，公共路由检查新增 `/rollback`。反向读取只增加受信历史基线能力；PublicationSession 仍是唯一完整业务发布事务。没有旧直写、免审批回滚、临时结构或真实通知投递。

## 评审与冻结

Standards（sol/high）和 Spec（astra/high）分别只读检查固定基点以来完整工作区差异。Spec 初审发现唯一值依赖的 P1，真实 red→green 后关闭，最终未关闭规格问题 0。Standards 初审的冻结列位置格式重复已由 Domain `TableExecutionSchema.Columns()` 收敛，硬性违规 0；复审保留一项主观建议：反向来源的 `len-1-index` 在意图、最终核验和 MySQL 墓碑读取中分别计算。本单保留这三处短映射，各自对应不同职责，并通过双 ENUM、混合和 1,000 项真实对应检查；暂不为一行索引扩大来源接口。

逆序后的旧错误索引夹具曾导致集中运行失败，已把正向顺序调整为 id/other，让反向第二项仍为陈旧项，并同步受控维护索引；五个真实场景补跑命令 12.642s、exit 0。失败日志不列为完整通过证据，最终完整入口仍覆盖全部场景。

2026-09-08 06:59:37+08 曾冻结 274 个代码/配置文件，聚合 SHA256 `3fcfa2422390bd753bbc2bbacef3923020604c78b82615a64a31c91743ae0dcc`。该轮曾观察到 Go/Web 检查与正式浏览器通过，但会话中断后 `/private/tmp` 下逐文件清单、原始日志和两份评审报告均已消失；保留上述已观察历史，不把缺失的原始文件声称为当前可读取证据，也不能证明旧清单与新清单的逐项等价。

恢复阶段没有重置或重写现有实现。2026-09-08 10:13:04+08 按当前明确规则（全部候选代码、配置与 fixture，排除 Markdown 和图片）重新冻结 278 文件，聚合 SHA256 `13e8f767d108f31de8bb310a6a62597bdd1a91e3133e7cfc3576c946d805e734`。完整清单与逐文件校验脚本保存在项目内持久目录 `.worktrees/release-delivery-records/recovery-t7/`；与旧 274 文件清单的定义不同，不直接比较两个聚合值。

恢复后的独立只读两轴复核仍为 Spec 实现阻塞 0、Standards 硬性违规 0及上述已接受主观建议 1。新报告为持久目录的 `spec-review.md`、`standards-review.md`。以下全部结果来自同一新冻结代码的实际执行，恢复阶段只更新文档和截图。

| 最终检查 | 已收取的实际结果 |
| --- | --- |
| `make test` | exit 0，四个 Go 模块，含依赖方向、公共路由、服务身份和草稿会话不能写业务数据检查；命令 17.466s |
| `make build` | exit 0，全部 Go 模块及 account-maintain；命令 0.524s |
| Web `pnpm test:run` | exit 0，23 文件 231 项；命令 15.594s |
| Web `pnpm typecheck` / `pnpm build` | 各 exit 0；命令 0.851s / 1.303s |
| `make test-browser` | exit 0，Admin 130.264s、命令 131.669s；全部 6 个正式管理脚本及账号系统路径通过，新回滚场景 11.95s |
| `make test-integration` | exit 0，Admin 1646.407s、HTTP 89.942s、命令 1648.073s；466 条命名测试及子测试 PASS，无 SKIP/FAIL；含全部回滚、1,000 项与既有升级用例 |

本次原始输出与命令、时间、退出码分别保存到持久目录的 `go-test`、`go-build`、`web-test`、`web-typecheck`、`web-build`、`browser`、`integration-serial` 对应 `.log` / `-result.json`。Colima 原实例恢复后配置仍为 2 CPU、约 4 GB，Docker 29.5.2、MySQL 8.4；Go 1.27.0、Node 24.19.0、pnpm 10.28.2。完整 MySQL 于 10:17:02+08 在浏览器结束后单独开始，没有另行并行构建或测试；`GOFLAGS=-v` 只增加输出，多包执行仍可能包级缓冲。正式完整 MySQL 超时仍为 40 分钟，没有放宽业务默认 4 秒期限。

完整入口于 10:44:30+08 实际退出 0。本轮 `TestAccountUpgradeFromLegacyMatchesFreshSchema` 11.97s、`TestReleaseThousandItemsThroughExecutable` 16.60s、全部 12 个回滚顶层测试均 PASS；完整 MySQL 包结果与原始输出可重新读取。`collect_checks.py` 核实七项检查均已完成且 exit 0、没有跳过或失败、上述 14 个关键顶层用例实际 PASS，并再次逐文件验证冻结未变；汇总保存为 `checks-summary.json`。本轮成功不改变首轮失败的事实，也不证明之前启动延迟的根因。

本轮可复查的 1,000 项逆转样本：申请 67B→1,725,911B / 332.291ms，提交 894.702ms，审批 222.937ms，反向执行 24B→3,314,337B / 1.395614s，原键重放 240.082ms。完整恢复和记录计数断言均通过；该代表样本仍不构成任意体积或负载的性能承诺。

## 正式浏览器证据

冻结后正式入口使用独立 MySQL 8.4、Admin、Vite 和 Chrome。不同 EDITOR、APPROVER、PUBLISHER 先完成正向发布；真实服务器接收回滚申请后故意丢响应，刷新仍使用原键、原理由恢复同一张只读反向草稿；再次独立审批发布后，原业务值恢复，版本为 2，原单 ROLLED_BACK、反向单 SUCCEEDED、双向关联与永久历史完整。

持久目录 `browser-evidence/release-rollbacks.cjs/rollback-evidence.json` 记录原单 `89131506c8d7fc735dad19425dc737ad`、反向单 `ec229bca4957aa9fdfa7488b24968247`，`rollback_request_replayed_with_same_key=true`、`browser_errors=[]`、恢复记录版本 `2`。两个布局快照均为 viewport=document=390、overflowing=[]。恢复前的两个旧截图另存于持久目录，下面链接更新为本次正式入口截图并再次目视检查。

已复制并目视检查最终截图：

- [原单已回滚、最新反向单及原实际发布历史](2026-09-08-rollback-original-mobile.png)
- [反向发布结果、原单链接及新的独立审批历史](2026-09-08-rollback-reverse-mobile.png)

## 首轮完整回归与启动超时复验

首轮 `make test-integration` 命令实际 1698.135s、exit 2，Admin 包1696.569s；唯一失败为既有 `TestAccountUpgradeFromLegacyMatchesFreshSchema` 在固定10秒窗口内未就绪，失败时子进程尚未退出且输出为空。HTTP包96.817s及其余包通过，但这一轮整体明确记为失败。该失败及下一段单项复验是中断前已观察记录；其原始 `/private/tmp` 输出已消失，恢复后无法重新读取。

该时间窗与独立browser及若干构建/测试重叠，是资源延迟的可能解释，尚未证明根因。保持冻结代码、启动夹具10秒及正式业务4秒全部不变，单独 `go test -v -count=3 -tags=integration ./cmd/admin -run '^TestAccountUpgradeFromLegacyMatchesFreshSchema$'` 三次均通过，Admin34.663s、命令36.084s、实际exit0。随后中断前的完整串行重跑没有可恢复的最终退出结果，不能记为通过；本次恢复重新执行完整正式入口，结果以最终表格和持久真实退出记录为准。没有通过放宽期限、跳过旧升级或仅拼接单项通过来宣称全套成功。
