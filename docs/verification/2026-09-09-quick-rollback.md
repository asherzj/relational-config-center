# T4 / #63 免审批快速回滚验收

对应 [#63](https://github.com/asherzj/relational-config-center/issues/63)，父规格 [#59](https://github.com/asherzj/relational-config-center/issues/59) 的 AC-003/004/005/006/015。工作树 `/private/tmp/rcc-issue-63-quick-rollback`、分支 `codex/issue-63-quick-rollback`。真实 HTTP → Admin → 独立 MySQL 8.4 是后端行为入口，浏览器调用相同接口；没有 mock 自有内部协作者。

## 固定基点与依赖合并

功能固定基点为 `17cdaf015d000ac42d3c1f7404192690effe1dcd`。这是 T1 `f7a39bafe2e767b706a5f4f158e7972cdfe53af0` 与 T2 `911f23b3c1d2f79008f641b317a71b2ddb042a9e` 的真实合并，共同祖先 `f513859`。最终评审同时审查合并决议和基点之后的功能差异。

16 个冲突保留了 T1 外层标题、人员姓名及失败回退、精确标题/状态等待、账号搜索、跨引擎顶层模态 Tab 逻辑，以及 T2 成功待完结、真实目标保护、人工完结和普通反向结果直接结束。T2 独有 completion fixture 补齐 T1 要求的标题，没有生产兼容默认值。旧普通回滚浏览器 5 项全部保留。冲突路径、原始 combined diff 和 `git show --remerge-diff --binary` 存于 `/private/tmp/rcc-issue-63-evidence/merge-*`；没有把语义调整藏在 feature 基点里。

合并基点验证：`make test`、Web typecheck、28 文件 302 测试通过；真实 MySQL 依赖交叉检查 137.373 秒、exit 0。

## 后端行为与红绿

| 行为 | 真实证据 |
| --- | --- |
| 当前 PUBLISHER/ADMIN 可恢复普通 SUCCEEDED；不限定原发布人 | `TestQuickRollbackPreviewShowsWholeRestorationWithoutWriting` 和 `TestQuickRollbackUsesCurrentRolesAndRequiresReviewedVersionDigestAndReason`；EDITOR/APPROVER/VIEWER 预览和执行被拒，实时撤权后包括原键恢复也被拒，当前 ADMIN 成功 |
| 整单 ADD/MODIFY/DELETE 恢复、真实结果和历史 | `TestQuickRollbackRestoresMixedPublicationAndReplaysActualResult`；逆序恢复实际行/版本，原单 ROLLED_BACK、反向 COMPLETED、唯一 QUICK_ROLLBACK/实际执行人/原因、原发布结果保留、目标可重新提交、禁止再次反向 |
| 外部 SQL 未递增 RCC 版本仍不能覆盖 | `TestQuickRollbackRejectsUnversionedExternalChangesBeforePreviewAndExecution`；预览前和确认后直接改值均拒绝，整单及历史/版本/占用保持不变 |
| 原占用不允许丢失后自动抢占 | `TestQuickRollbackRequiresItsOriginalRetainedTargets`；删除一个真实目标 reservation 后整单拒绝，原业务值及其他目标不变 |
| 预览绑定当前 Schema、规则和版本 | `TestQuickRollbackRejectsChangedSchemaRulesAndRecordVersions`；规则、表结构、记录维护推进分别失败；修改预览摘要和原单版本同样拒绝 |
| 完结、多个快速回滚与重试竞争 | `TestQuickRollbackCompetesWithCompletionAndOtherRollbacks`；屏障同时释放三种并发组合，终止状态/历史/版本只有一个结果，同键返回同一结果 |
| 任一持久化失败都无部分成功 | `TestQuickRollbackPersistenceFailuresPreserveValuesVersionsHistoryAndTargets`；8 个真实 MySQL trigger 故障边界覆盖 Record Version、Table Version、Command、通知、反向 INSERT、原单 UPDATE、目标释放和请求结果；移除故障后原键成功 |
| 后项约束或实际触发器效果不符仍整单回退 | `TestQuickRollbackConstraintFailureRollsBackEarlierItems` 和 `TestQuickRollbackRejectsDatabaseEffectsThatCannotRestoreOriginalValues`；后项唯一值冲突或真实 BEFORE trigger 改变恢复值时，前项、版本和全部控制记录也回退 |
| 合法容量可终止 | `TestQuickRollbackCanTerminateAtAcceptedPublicationCapacity`；使用接近 8 MiB−64 KiB 门禁的有序历史及 1,998 字节原因，成功结束并按原键恢复；沿用 T2 的终止余量保护 |

逐步红绿的原始日志保存在 `/private/tmp/rcc-issue-63-evidence/`。预览和执行最初因路径未接通得到 403；外部 SQL 和缺失目标的测试最初错误接受 200，分别通过真实规范行锁/校验和及严格原占用检查转绿。Schema 测试曾因 MySQL 在 ALTER 后把生成表达式字符集改写而继续正确拒绝；诊断后使用没有生成表达式的专用业务表隔离规则、Schema 和版本三类行为，生产断言未放宽。组合定向后端运行 `quick-backend-final.log` 149.481 秒、exit 0；含全部 11 个 quick 顶层测试及普通回滚/完结兼容。`make test` 通过，公共路由机器检查包含两个新入口。

后端随后冻结；根任务持有并收取完整 `make test-integration` 的最终退出结果，Go、SQL、模块、Makefile 和 Python 输入按 `/private/tmp/rcc-release-detail-reference/issue-63-backend-freeze.json` 的 147 项逐文件 SHA256 核验，全部未变。前端和文档工作不更改这些输入。

## Web 与浏览器

Web 在预览读取中、读取失败、缺原因和超长原因时禁止执行；实际恢复差异使用“当前值/恢复值”表头。未知结果保留原预览摘要、原因和请求键，同单完结和新快速回滚暂停，刷新后恢复原键原正文。明确拒绝后不自动刷新预览；主动读取当前状态、重新审阅恢复差异后才允许新键，已结束单据不能重建快速回滚。

新增 7 条 Web 测试（含三个角色参数用例）覆盖这些行为。红绿测试曾发现请求结束时自动重新读取预览，修复为未知或拒绝期间禁用自动读取，保留“只读取一次”断言。预览失败时原因保留、2,000 UTF-8 字节限制和当前角色优先于旧详情 allowed_actions 均通过。

原 `web/e2e/release-rollbacks.cjs` 保留普通回滚 5 项，并增加 3 项真实 quick 行为：

1. 独立 EDITOR/APPROVER/PUBLISHER 完成正向发布；当前发布人查看真实恢复值、填写必填原因，桌面和 390px 取消均不写入，EDITOR 看不到快速回滚。
2. 使用 `route.fetch()` 让真实服务器成功提交后中断浏览器响应；双击只发一次，刷新恢复与首次 key/body 完全相同，预览读取次数不增加；记录真实执行人、原因和双向关联，实际业务版本只增加一次，两单直接结束。
3. 另一真实 PUBLISHER 在已读预览后先完结；浏览器提交旧预览收到 409，原因保留，主动读到 COMPLETED 后没有重建入口，业务值和版本保持竞争赢家的结果。

初次定向浏览器 v1 在构建阶段因测试使用了 Testing Library 不支持的 `exact` 属性失败，修正测试类型后继续。v2 的上述前两条成功，第三条发现通用恢复区域仍显示不可用确认按钮；保留“没有重建入口”断言，针对已结束的 quick 请求隐藏该按钮并说明原因保留。该轮不记为全套通过。

按现有 `web/DESIGN.md` 做有边界 impeccable 检查：一次 detector 返回 `[]`；一次桌面和 390px 成组目视检查发现长预览挤走操作按钮、窄屏值过度折行，局部修正为可滚动恢复区与固定底部操作、表格局部横滚。未更改共享视觉体系。

Web 最终 typecheck 通过，完整 28 文件 309 测试通过（`web-full-tests-green.log`）。首次全量因沙箱拒绝旧真实 TCP 用例监听 `127.0.0.1` 而失败；授予所需 loopback 权限后按原范围重跑通过，没有更改用例或超时。

最终局部修正后的定向浏览器 v3 在旧 `release-batches.cjs` 的 1,000 项 execute 收到真实 HTTP 504，Admin `duration_ms=4002`，触及原有 4 秒事务边界，未进入 quick 脚本。原样保留该失败，未把它归因为资源竞争，也未改后端、期限或等待断言；v2 的同一旧批量用例曾通过。`/private/tmp/rcc-issue-63-browser-focused-v3/run.txt` 记录 exit status 1、cleanup verified true，外层 make exit 2。v2 两条成功 quick 路径及其桌面/390px截图位于 `/private/tmp/rcc-issue-63-browser-focused-v2/release-workflow/rollback-chromium/`；该时点局部视觉修正仍待正式入口确认，不能用 v2 截图声称最终视觉已通过；随后最终确认见下文。

正式 `make test-browser` 首轮于 2026-09-09 07:32:34+08 实际退出 2，命令 61.636 秒。首个 `accounts.mjs` 在第 312 行等待旧式 heading `notification_templates · 已完结` 超时 30 秒，尚未进入 quick 脚本；该轮不是前述 1,000 项事务超时。来源追溯表明此行由 T2 `911f23b3` 新增，合并时未改为 T1 的独立标题/状态结构。当前详情的 heading 是发布单标题，表名/状态是相邻段落，即使完结成功旧定位器也不能匹配。修复只把这一行改为已有 `releaseState(page, '已完结')`，继续分别精确等待标题 `notification_templates 配置变更` 与状态 `notification_templates · 已完结`，不删除完结步骤、不放宽状态或超时。

同类核对覆盖 T2 在 accounts、batches、drafts、rollbacks 中新增或调整的等待：accounts 另外两处已发布待完结、batches 两处发布成功、rollbacks 的普通发布/完结/反向结果及 drafts 取消状态均已使用标题与精确状态等待；publishSingle 的真实 HTTP COMPLETED 断言继续保留。该定位器修复先通过 `node --check` 和差异检查；代理未重跑浏览器或数据库，随后根任务持有原正式入口的完整绿色复验，结果见下文。失败原日志 `/private/tmp/rcc-issue-63-final-go-browser-v1.log` 保留，日志确认测试容器已停止并删除。

## 最终出口

完整 MySQL 已实际退出 0：530 条命名测试及子测试 PASS、0 FAIL、0 SKIP，命令 2145.724 秒。原始根任务汇总已原样复制为[完整 MySQL 汇总](2026-09-09-quick-rollback-mysql-summary.json)，147 个后端输入再次核验未变；未拼接单项结果或跳过失败用例。

| 已完成检查 | 最终结果 |
| --- | --- |
| `make test-integration` | exit 0；530 PASS / 0 FAIL / 0 SKIP；2145.724 秒 |
| `make test` | exit 0；含分层和公开路由机器检查 |
| `make build` | exit 0；3.074 秒，根任务持有最终结果 |
| Web `typecheck` | exit 0 |
| Web `test:run` | exit 0；28 文件、309 项；29.52 秒 |
| Web `test:dev` | exit 0；1.017 秒，根任务持有最终结果 |
| `make test-browser` | exit 0；127.852 秒；包含原账号完整路径、普通回滚和新增 quick 3 项 |
| `test-browser-acceptance` all / 三引擎 | exit 0；473.417 秒；17 套件、234 检查，清理及数据库复核通过 |
| 独立 Spec 评审 | 0 问题；覆盖真实合并决议及功能差异，文档和 completion-wait 增量复审均为 0 |
| 独立 Standards 评审 | 最终 0 硬性问题；升级指南与 completion-wait 增量复审均为 0 |

Standards 的三项非阻断 P3 建议已接受记录：HTTP preview 清除私有字段逻辑重复、`quick-rollback` 动作字符串跨边界分散、`PublicationPlan.TargetOrderID` 命名可更具体。本范围不进行这些可选重构，不重启已冻结后端验证。评审事实及静态结果归档在[可提交概要](2026-09-09-quick-rollback-results.json)，其中 `ready_to_commit` 为 true；尚未执行提交或外部交付。

根任务在独占资源下依次收齐两条正式入口。`make test-browser` v2 实际退出 0，修正后的 accounts 完结等待、既有 1,000 项批量发布与 quick 全路径均通过；[命令汇总](2026-09-09-quick-rollback-browser/go-browser-summary.json) 记录 127.852 秒，测试容器已停止并删除。

随后完整 `RCC_E2E_SUITE=all RCC_E2E_ENGINES=chromium,firefox,webkit make test-browser-acceptance` 实际退出 0，[命令汇总](2026-09-09-quick-rollback-browser/three-engine-command-summary.json) 记录 473.417 秒。[完整浏览器汇总](2026-09-09-quick-rollback-browser/summary.json) 包含 17 个套件、234 条检查，没有缺失项；[运行记录](2026-09-09-quick-rollback-browser/run.txt) 确认 exit status 0、cleanup verified true。fixture 行未变，数据库收尾复核通过。此前 v2/v3 和正式 GoBrowser 首轮失败事实仍保留，没有调整原有超时或把失败结果替换成成功。

Chromium、Firefox、WebKit 的回滚脚本均通过旧普通 5 项和新增 quick 3 项。各引擎的[真实单号与原键恢复记录](2026-09-09-quick-rollback-results.json) 已归档，三个结果均为 `quick_request_replayed_with_same_key_body=true`、恢复 Record Version 4、`browser_errors=[]`。预览读取 2 次分别对应第一次取消和第二次主动打开；未知结果及刷新恢复不再读取预览。独立竞争完结单号也保留在每份 `rollback-<engine>.json` 中。

最终四张截图由根任务通过 `view_image` 成组确认，并按原文件逐字节复制入仓库：

- [Chromium 桌面整单恢复预览](2026-09-09-quick-rollback-browser/chromium-preview-desktop.png)
- [WebKit 390px 恢复预览](2026-09-09-quick-rollback-browser/webkit-preview-mobile.png)
- [Firefox 未知结果与原原因恢复](2026-09-09-quick-rollback-browser/firefox-unknown.png)
- [WebKit 390px 已完结反向结果](2026-09-09-quick-rollback-browser/webkit-result-mobile.png)

确认结果：浮层稳定不透明，恢复说明和危险操作 footer 清楚可达；390px 表格局部横滚，页面没有整体横向溢出；未知结果显示保留的原因和原请求重试；反向 COMPLETED 结果保留真实发布人与原单关联。这是局部修正后的正式 artifact，未使用 v2 截图替代最终确认。

产品与测试文件自评审快照 v4 后未变，147 个后端输入仍逐项一致。本轮仅补齐验收 Markdown、JSON、原始摘要/运行记录及四张截图。

后端预算和事务期限、既有普通审批回滚、自批限制以及 NOT_CONNECTED 分发边界均不变。当前没有暂存、提交、推送或关闭工单。
