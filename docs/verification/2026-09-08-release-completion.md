# 发布待完结与普通回滚收尾（#60）

规格：[T2 #60](https://github.com/asherzj/relational-config-center/issues/60)，父规格 [#59](https://github.com/asherzj/relational-config-center/issues/59) 的 AC-001、002、007、008，以及本功能涉及的权限、原请求恢复与确认要求。固定基点 `f513859c41e643e2b4d6ee37684dae67ad920e53`；分支 `codex/issue-60-release-completion`。

本工单的必要检查和 Standards/Spec 两轴评审均已通过，剩余发现 0。最终完整 MySQL 为 504 PASS/0 FAIL/0 SKIP；本地 macOS 正式三引擎全部 17 套、224 个具名检查通过，fixture 前后一致且资源清理已验证。Linux CI 本轮未运行。下文保留曾失败的运行、诊断和修复，不将历史失败计为通过。

## 行为证据

验收接缝沿用已确认的已认证 Admin 公开 HTTP API 和真实 MySQL 8.4。仅用 SQL 布置业务数据、注入真实持久化故障或损坏存储边界，不模拟内部协作者。最终业务值、状态、目标冲突、角色和原键结果通过公开 API 观察。

| 需求 | 验收与观察 |
| --- | --- |
| AC-001 | `TestReleaseCompletionRetainsKnownTargets`：普通成功保留 SUCCEEDED，重叠 submit 返回 `409 release_target_conflict`，同表不重叠记录完整审批并发布成功。 |
| AC-002 | `TestReleaseCompletionProtectsActualAndDeletedIDs`：自增新增取得实际 ID，删除保留原身份；再次提交两类身份都被拒绝，人工完结后相同 submit 请求均可成功。混合和千项普通审批回滚成功后断言全部占用释放。 |
| AC-007 | `TestReleaseCompletionReleasesWithoutRepublishing`：另一位仅 PUBLISHER 账号完结成功，配置值与 Record Version 不变、完整 Publication 逐字段相等，COMPLETE 历史归实际操作者且无意见；之后同记录可重新发布，COMPLETED 可筛选。 |
| AC-008 | `TestReleaseCompletionOrdinaryRollbackClosesBothOrders`：待完结时普通 rollback 拒绝；完结后新反向草稿未批准不能执行；独立审批后原单 ROLLED_BACK、反向 COMPLETED，反向结果的 complete/rollback 均拒绝且目标可再次提交。 |
| 当前权限、CAS、幂等 | `TestReleaseCompletionRolesConcurrencyAndReplay`：VIEWER/EDITOR/APPROVER 拒绝，当前 PUBLISHER 可操作；未发布、旧版本、重复终态拒绝；不同键竞争仅一个成功，原键重放原结果，改内容冲突，撤权后旧成功键也拒绝。ADMIN 完结由其他生命周期用例覆盖。 |
| 故障原子性 | `TestReleaseCompletionPersistenceFailureIsAtomic`：真实 MySQL trigger 分别拒绝目标 DELETE、COMPLETED 状态保存、成功请求结果保存；每次 503 后原单逐字段不变，原配置和 Record Version 不变，公开重叠 submit 仍冲突；移除故障后同键完结并释放。`TestPublicationAtomicPersistenceFailures` 改为实际自增目标 INSERT 故障，证明发布不能写业务值却遗漏占用。 |
| 结果完整性 | `TestReleaseCompletionRequiresTrustedPublicationOnReadAndReplay`：缺失 Publication 的 COMPLETED 文档不能从详情、列表或 complete 原键重放返回成功；完整性检查沿用可信实际发布结果规则。 |
| 浏览器确认与恢复 | `ReleaseOrdersPage.test.tsx` 覆盖取消无写入、无必填意见、确认文案、重复点击、丢响应后保留原键，并在恢复前禁用普通 rollback；`release-rollbacks.cjs` 使用真实 EDITOR/APPROVER/PUBLISHER 账号走发布→完结→新独立审批→普通回滚全路径，在真实成功响应被丢弃后跨刷新恢复两类原请求。 |

首次红灯分别观察到：已知和自增重叠 submit 错误返回 200；PUBLISHER complete 缺少路由/角色入口而返回 403；未完结 rollback 错误返回 201；COMPLETED 缺少 Publication 仍返回 200；完结未知结果恢复前普通 rollback 按钮仍可点击。对应实现后各定向验收转绿，未通过制造无意义失败满足红绿循环。

## 回归与命令

本机真实 MySQL 命令均使用以下进程环境，不修改任何 `.env.local`：

```sh
export DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
export RCC_ADMIN_TOKEN=''
export NO_PROXY=127.0.0.1,localhost
```

| 命令 | 结果 |
| --- | --- |
| `pnpm --dir web install --frozen-lockfile` | 独立工作树安装完成，无共享可写 node_modules。 |
| `go -C admin test -count=1 -timeout=8m -tags=integration ./cmd/admin -run '^TestReleaseCompletion'` | 最初四个完整生命周期用例通过。 |
| `go -C admin test -count=1 -timeout=10m -tags=integration ./cmd/admin -run '^TestReleaseCompletion\|^TestReleaseRollback\|^TestPublicationAtomicPersistenceFailures$'` | 权限、并发、完结故障和普通回滚回归；初次仅两个旧用例缺显式 complete，已修复并在后续定向通过。 |
| `go -C admin test -count=1 -timeout=10m -tags=integration ./cmd/admin -run '^TestReleaseCompletion\|^TestReleaseRollbackRestoresDeletedEnumIdentity$\|^TestReleaseRollbackRestoresBusinessFieldsWithNewAudit$\|^TestReleaseBatch\|^TestReleaseHistory'` | 完结全部用例、批量/千项和两条修复的回滚通过；历史 helper 初次多传 reason，按 complete 契约修复后单独通过。 |
| `go -C admin test -count=1 -timeout=5m -tags=integration ./cmd/admin -run '^TestReleaseHistorySurvivesExecutableRestartAndExternalChanges$'` | 通过，19.876 秒。 |
| `make test` | 全 Go 模块通过，包括层依赖、身份隔离及公开动作路由检查。 |
| `make build` | 全 Go 模块与可执行文件构建通过。 |
| `pnpm --dir web typecheck` / `pnpm --dir web build` | 通过。 |
| `pnpm --dir web test:run` | 28 个文件、298 个测试通过。 |
| `make test-browser` | 通过，119.94 秒；含账号恢复、未保存保护、规则、发布草稿、审批、批量、完结及普通回滚。 |
| `RCC_E2E_OUTPUT=/private/tmp/rcc-issue-60-browser-evidence go -C admin test -v -count=1 -timeout=5m -tags=integration,browser ./cmd/admin -run '^TestAccountBrowserSystemPath$/^release-rollbacks.cjs$'` | 通过，45.85 秒；输出目录预先创建，使用上面的真实 MySQL 环境。修正截图等待条件后重新运行并检查桌面和 390 px 确认面板。 |
| `make test-integration`（仅追加 verbose 输出和本地 NO_PROXY） | 最终完整回归退出 0，2,060.440 秒，504 PASS/0 FAIL/0 SKIP。保持仓库原 40 分钟套件超时及正式 4 秒业务事务期限；之前的失败及其原因保留在下文。 |
| `RCC_E2E_SUITE=all RCC_E2E_ENGINES=chromium,firefox,webkit make test-browser-acceptance` | 本地 macOS 通过，退出 0，460 秒，17 套/224 个具名检查；fixture 前后一致，cleanup verified 为 true。Linux CI 未运行。 |
| `pnpm --dir web test:dev` | 2 项通过；日志 `/private/tmp/rcc-issue-60-final-web-dev.log`。 |
| `git diff --check` | 通过。 |

测试 runner 对编译缓存、Docker socket、真实 TCP/HTTP 监听需要 sandbox escalation；首次受限环境失败已通过使用相同检查的正常本机权限恢复。没有放宽测试断言、跳过测试或扩大业务超时。

普通数据策略测试中的 `publicationFixtureRequest` 现显式执行完整发布并完结，向调用者保留实际 execute 的历史响应，便于下一条独立数据用例使用同一记录。专门测试待完结和目标占用的用例直接调用公开动作，不经过该收尾 fixture。原回滚、大历史容量、混合千项与重启测试均迁移到新的完结前提。

## 双轴评审与修复

| 评审发现 | 处理与证据 |
| --- | --- |
| Spec P2：SUCCEEDED 与无关联 COMPLETED 共用 `8 MiB − 64 KiB` 上限，合法极限发布单追加 COMPLETE 历史时可能永远无法完结。 | 新增 `TestReleaseCompletionAtPublicationCapacityPreservesRollback`：真实发布后布置顺序合法的长 EDIT 历史和不超过 2,000 字节的审批意见，使持久化结果距允许发布上限仅 16 字节；公开 complete 首次返回 `422 release_result_limit`，复现评审问题。修复允许无关联 COMPLETED 消耗原 64 KiB 余量中的 4 KiB，仍为普通回滚保留 60 KiB，总 8 MiB 和 HTTP 元数据 1 KiB 预留不变。转绿后检查 complete 原键重放、原 Publication 不变，再通过多次公开普通回滚申请/取消/重建消耗余量，最终独立审批并实际执行反向发布，原单 ROLLED_BACK、反向 COMPLETED、业务值恢复且目标可再次提交。 |
| Standards P2：审批文档仍说反向执行成功为 SUCCEEDED、所有执行成功都释放目标。 | 修正 `docs/admin-release-approvals.md` 的详情状态、公开动作恢复和事务说明：普通 SUCCEEDED 保留目标，人工 COMPLETED 释放；反向直接 COMPLETED 并释放。容量文档同步完结与普通回滚的预留分配。 |
| P3 建议：API 与确认框重复判断意见要求。 | 采纳；`releaseActionRequiresReason(action)` 由请求序列化和确认按钮共同使用，保持原请求正文及交互行为。现有全部 298 个 Web 测试、TypeScript 与生产构建通过。 |

修复阶段仍沿用上面的真实 MySQL 进程环境：

- 红灯：`go -C admin test -v -count=1 -timeout=5m -tags=integration ./cmd/admin -run '^TestReleaseCompletionAtPublicationCapacityPreservesRollback$'`，退出 1，公开 complete 返回 422；日志 `/private/tmp/rcc-issue-60-review-capacity-red.log`。
- 绿灯及相邻终止容量回归：`go -C admin test -v -count=1 -timeout=5m -tags=integration ./cmd/admin -run '^TestReleaseCompletionAtPublicationCapacityPreservesRollback$|^TestReleaseRollbackPendingHistoryCanTerminate$|^TestReleaseBatchBudgetRetainsCancellationHeadroom$'`，退出 0，68.637 秒；日志 `/private/tmp/rcc-issue-60-review-capacity-green.log`。
- `make test` 与 `make build` 重新通过，日志 `/private/tmp/rcc-issue-60-review-go-tests.log`、`/private/tmp/rcc-issue-60-review-build.log`。
- `pnpm --dir web typecheck`、`pnpm --dir web test:run`、`pnpm --dir web build` 重新通过，Web 28 个文件、298 个测试，16.54 秒；测试日志 `/private/tmp/rcc-issue-60-review-web.log`。

初次完整 MySQL 进程编译于本次后端修复前；进程已结束但没有完整输出或退出码，不能计通过。第二次完整回归由主代理启动并持有进程（会话 1156），使用冻结 v2 产品代码；该次完整结果为 make 退出 2，下面保留具体失败及修复。

增量复审结果：Spec 原 P2 关闭，剩余 0；Standards 确认原 P2/P3 已解决，并发现回滚文档仍把无关联 COMPLETED 的余量写为 64 KiB，已同步为 SUCCEEDED 保留 64 KiB、完结可消耗其中 4 KiB、无关联 COMPLETED 保留 60 KiB。该次增量只涉及回滚说明和本验收记录，`git diff --check` 通过；未改产品代码或测试，也未追加全量或浏览器验证。

## 完整回归发现的测试前提遗漏

主代理取得完整输出 `/private/tmp/rcc-issue-60-final-integration.log`：`make test-integration` 退出 2；Admin 包用时 1,802.858 秒并失败，其他包通过。仅两个顶层用例失败：

- `TestConcurrentAccountsOwnTheirBusinessChanges`：`actor_alpha`、`actor_beta` 各自成功发布新增记录后，在同一记录上直接提交 MODIFY，公开 submit 返回 `409 release_target_conflict`。本用例的本地 publish 闭包现使用原账号调用公开 complete 再开始独立变更，仍向调用者返回原 execute 响应，原发布人和业务 creator/modifier 归属断言全部保留。
- `TestPublicationFrozenChangesAndDescriptions`：恢复表结构并确认描述变化不阻止原发布成功后，直接复用目标准备 stale 测试，公开 submit 返回相同 409。现只在该成功 execute 后增加公开 complete，再继续原有结构冻结、记录版本冲突、规则禁用和配置不变断言。

本次只增加两处测试前提调用及注释，共 3 行；没有改产品占用规则或共享 fixture，没有删除、跳过、放宽断言。精确测试差异为 `/private/tmp/rcc-issue-60-full-fixture-delta.patch`。

真实 MySQL 定向命令使用本页环境：`go -C admin test -v -count=1 -timeout=5m -tags=integration ./cmd/admin -run '^TestConcurrentAccountsOwnTheirBusinessChanges$|^TestPublicationFrozenChangesAndDescriptions$|^TestRecordVersionAddDeleteRecreateAndRollback$|^TestReleaseCompletionRetainsKnownTargets$|^TestPublicationPublisherHistoryAndApprovalSurvivesRevocation$'`。该命令同时检查原失败、共享 fixture 的增删重建、待完结占用保留、发布人历史和撤权；五项全部通过，退出 0，45.703 秒，日志 `/private/tmp/rcc-issue-60-full-fixture-green.log`。该 fixture 增量的 Standards/Spec 两轴均通过，剩余 0；主代理随后取得最终完整回归通过结果，见下节。

## 本地代理隔离与最终完整回归

v3 首次完整回归（`/private/tmp/rcc-issue-60-v3-full-integration.summary.json`）用时 1,954.243 秒，503 PASS/1 FAIL/0 SKIP。唯一失败是账号验收 Python HTTP 客户端 `RemoteDisconnected`；全部本单生命周期和修复的 fixture 均已通过。主代理用最小 loopback HTTP stub 诊断：原客户端 3/3 失败且本地服务未收到请求，只设置 `NO_PROXY=127.0.0.1,localhost` 后 3/3 成功并收到预期 5 次请求，没有修改生产代码或测试断言。

相同正式 `make test-integration` 范围保留 40 分钟超时、原测试和原 GOFLAGS，并追加 verbose 输出以及仅本地地址的 NO_PROXY；完整执行退出 0，2,060.440 秒，504 PASS/0 FAIL/0 SKIP。完整日志 `/private/tmp/rcc-issue-60-v3-bypass-full-integration.log`，退出码同前缀 `.exit`，机器汇总同前缀 `.summary.json`。本页已核对该汇总。开发服务器检查另有 2 项通过，日志 `/private/tmp/rcc-issue-60-final-web-dev.log`。

正式 `make test-browser-acceptance` 的三引擎首轮前五套和 Chromium 无障碍通过；Firefox 首次本地导航 `NS_ERROR_NET_RESET`，该轮清理已验证。主代理进一步确认 Firefox 默认 launch 对本地最小页面 3/3 reset 且服务未收到请求；仅加入 `firefoxUserPrefs: { 'network.proxy.type': 0 }` 即 3/3 成功。NO_PROXY 对此浏览器系统代理继承不生效；没有改用户系统代理设置。

本次只调整验收启动配置：`local-account.cjs` 的共享 Firefox 选项覆盖普通脚本和 `release-rollbacks.cjs`；`browser-accessibility.cjs` 的独立启动点，以及 `accounts.mjs` 共用的持久上下文/独立审批浏览器选项也使用该直连偏好。其他引擎、headless、channel、executablePath、测试断言和生产代码保持原样。精确配置差异 `/private/tmp/rcc-issue-60-firefox-loopback-delta.patch`。

原真实 Firefox 无障碍专项命令：在本页进程环境下运行 `RCC_E2E_SUITE=browser-accessibility RCC_E2E_ENGINES=firefox RCC_E2E_ARTIFACTS=/private/tmp/rcc-issue-60-firefox-loopback-acceptance make test-browser-acceptance`。退出 0；Firefox 153.0 的 7 项检查通过，数据库后置状态符合预期，`run.txt` 记录 `cleanup verified: true`。日志为 artifact 路径加 `.log`；结果 `/private/tmp/rcc-issue-60-firefox-loopback-acceptance/browser-accessibility/firefox/result.json`。3 个改动脚本的 `node --check` 和 `git diff --check` 也通过。该配置增量双轴复审通过，剩余 0；最终正式完整三引擎结果见下文。

## 草稿列表返回的测试假设诊断

三引擎 v4 完整运行在前五套及 Chromium/Firefox/WebKit 无障碍全部通过后，`release-drafts.cjs` 的前四项也通过；随后在截图分支第二次返回列表时等待目标链接超时（原第 63 行），该轮退出 2 且清理成功。完整日志 `/private/tmp/rcc-issue-60-three-engine-v4-20260909.log`，原失败 `/private/tmp/rcc-issue-60-three-engine-v4-20260909/release-workflow/drafts/runner.log`。第 62 行的申请人/表/取消状态查询已经找到目标，失败出现在进入详情截图后再次返回列表的步骤。

按 diagnosing-bugs 构建了独立最小回路：真实 React 路由和列表组件，HTTP 边界使用默认第一页 20 条其他单据、显式筛选返回目标的固定数据。原流程运行 3.24 秒变红，重复带探针运行 3.13 秒同样红。探针仅记录公开列表路径和筛选值：默认列表 → 带表名/申请人/CANCELLED 筛选 → 目标详情 → 无参数默认列表；返回后的申请人及状态输入为空。由此排除“同一查询缓存陈旧”和“取消后目标不可读取”的假设。固定基点 `f513859` 的列表也在详情返回时重新挂载空筛选，本单没有修改此产品行为。

只改变一个输入的正对照（目标出现在默认第一页）2.36 秒通过；保持目标不在第一页，改为返回后再次显式填写同一筛选并查询，2.05 秒通过。诊断源码已移出工作树，保存在 `/private/tmp/rcc-issue-60-draft-list-diagnostic-{red,green}.test.tsx`；日志为 `/private/tmp/rcc-issue-60-draft-list-{minimal-red,probe,control,minimal-green}.log`。最小回路用于隔离前端流程原因，不作为真实 MySQL 通过证据。

修复仅在 `web/e2e/release-drafts.cjs` 提取本地 `findCancelledDraft`，首次返回与截图后返回都填写相同表名、永久申请人和 CANCELLED 状态并查询，再等待原目标链接。原取消意见、详情状态、链接、窄屏以及 VIEWER 断言全部保留；没有扩大等待时间，没有修改产品或后端。精确测试差异 `/private/tmp/rcc-issue-60-draft-list-delta.patch`。

真实浏览器专项采用本页 MySQL/NO_PROXY 环境和预先创建的 `RCC_E2E_OUTPUT=/private/tmp/rcc-issue-60-draft-list-browser-green`，命令 `go -C admin test -v -count=1 -timeout=5m -tags=integration,browser ./cmd/admin -run '^TestAccountBrowserSystemPath$/^release-drafts.cjs$'`，退出 0，44.916 秒；草稿专项 6 项检查通过（10.28 秒），日志 `/private/tmp/rcc-issue-60-draft-list-browser-green.log`。`node --check web/e2e/release-drafts.cjs`、`git diff --check` 通过；工作树无诊断文件和 `[DEBUG-T2-list]` 插桩。

## 最终验收与提交前核对

最终正式浏览器命令使用上面的本机进程环境：`RCC_E2E_SUITE=all RCC_E2E_ENGINES=chromium,firefox,webkit RCC_E2E_ARTIFACTS=/private/tmp/rcc-issue-60-three-engine-v5-20260909 make test-browser-acceptance`。退出 0，UTC `2026-09-08T20:38:03Z` 至 `2026-09-08T20:45:43Z`，共 460 秒；这是本地 macOS arm64 的真实 Admin/Web/MySQL 验收，没有运行 Linux CI。

17 套、224 个具名检查全部通过：前五套 110 项，三引擎无障碍各 7 项，草稿 6 项、审批 4 项、批量 5 项，三引擎发布/完结/普通回滚各 5 项，三引擎账号与请求恢复各 21 项。实际浏览器版本为 Chromium `151.0.7922.34`、Firefox `153.0`、WebKit `26.5`。原 fixture 行逐字节一致，数据库后置检查通过，`cleanup verified: true`。

- [可随仓库保存的最终结果汇总](2026-09-09-release-completion-results.json)：含完整 MySQL、三引擎各套计数、浏览器版本、其他必要检查和双轴结果。
- [正式浏览器运行及清理记录](2026-09-09-release-completion-browser-run.txt)。
- 本机原始浏览器目录 `/private/tmp/rcc-issue-60-three-engine-v5-20260909`，完整日志为该路径加 `.log`；各套原始结果和截图保留在该目录。

草稿查询测试修复的最后增量经 Standards/Spec 两轴复审，剩余 0。提交前重建当前完整差异并核对与冻结 v5 的 SHA256 `7002fb2f9c8c7e7d8012b919a454ec4424850e69424298db4e94fa5475a2165d` 完全相同；随后仅更新本页最终记录并加入两份可移植证据，33 个改动的代码/测试文件逐字节未变。工作树没有临时诊断测试或 DEBUG 探针，`git diff --check` 通过；本轮没有无新增理由重跑稳定测试。

## 浏览器截图与日志

真实浏览器使用现有灰色主题，确认抽屉完整显示释放目标占用、关闭快速回滚和重新独立审批的后果，无意见输入框。桌面和 390 px 截图在动画结束后采集；移动端文档宽度等于视口，确认按钮位于视口内。反向发布结果显示已完结和原单关联，没有再次回滚入口。

- [桌面完结确认](2026-09-08-release-completion-desktop.png)
- [390 px 完结确认](2026-09-08-release-completion-mobile.png)
- [390 px 普通反向结果](2026-09-08-release-completion-reverse-mobile.png)
- [状态、原键恢复与浏览器错误记录](2026-09-08-release-completion-browser.json)
- [390 px 布局检查](2026-09-08-release-completion-layout.json)

完整本机日志位于 `/private/tmp/rcc-issue-60-*.log`：`directed`、`directed-followup`、`history` 记录后端定向验收；`go-tests`、`build`、`web-tests`、`browser`、`browser-evidence` 记录回归。最终完整 MySQL 输出与退出码使用本页 `v3-bypass-full-integration` 前缀；早期 `full-integration` 无结果记录不计通过。最新评审快照为 `/private/tmp/rcc-issue-60-review-v5.patch`，全部文件清单为 `/private/tmp/rcc-issue-60-review-v5-files.txt`。

## 文档及后续

沿用 `web/DESIGN.md` 的灰色主题和已有抽屉、焦点及认证恢复规则，未引入新的共享视觉规则。Admin 词汇表、ADR-0020/0022 的覆盖说明、已确认 ADR-0023、角色与发布/回滚契约已同步。没有控制表迁移、旧发布单兼容、双写或 Feature Flag。

快速回滚接口属于 #63，本单只实现完结关闭该恢复阶段的领域规则及确认说明。标题、重新准备、人员解析和详情整体重排属于其他工单。技术验收和双轴评审已完成；提交、远端分支、GitHub 工单及 Notion 项目记录的收尾以最终工单证据为准。
