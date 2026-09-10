# #86 异常报错、安全重推与失败历史

固定基点：`e66d96e5aadd54f9ec0f6bf2a6138c2ff7ed0e9c`。本票承接 #81 的 AC-011/012；只在隔离工作树实现，最终完整功能集成由协调者负责。

## 业务结果与失败历史

| 边界 | 真实证据与结论 |
| --- | --- |
| 确认未提交、原请求可重推 | `TestConfirmedReleaseFailureHistoryPreservesOriginalRetry` 用真实晚项唯一键冲突证明前项业务行、记录版本、成功执行记录、Command 和通知全部回退；主单仍 APPROVED v3。只追加 EXECUTE_FAILED 历史，不推进 CAS。失败审计绑定原 key/digest，成功前同键异正文已返回 idempotency_conflict。外部冲突修复后原 key/expected_version 成功一次，同键重放返回相同结果。 |
| 失败历史也不可用 | `TestReleaseFailureHistoryUnavailableDoesNotLeaveBusinessWrites` 由真实 MySQL trigger 同时拒绝最终主单保存与独立审计；HTTP 原错误附带 `execution_outcome=not_committed`、`failure_history=unavailable`，主单、历史、业务值及成功结果均无伪造。解除故障后原键成功。 |
| MySQL COMMIT OK 丢失 | `TestPublicationCommitUnknownSurvivesExecutableRestart` 复用真实 wire proxy：前两模式在提交前失败，各追加一条失败历史；第三模式确实消费数据库的 COMMIT OK 并使驱动收不到 ACK（断言 ack=1），主单已 SUCCEEDED、成功执行仅1条、失败历史仍2条，HTTP 不带 not_committed。进程重启后原键返回原成功结果。HTTP 提交后响应丢失另由既有独立用例覆盖。 |
| 审计与取消竞争 | `TestReleaseFailureAuditPreservesConcurrentCancellation` 用外部未提交的唯一键 INSERT 阻塞发布，确认真实业务行锁等待，再确认取消等待主单锁。外部提交导致 duplicate_key 后，取消先提交 CANCELLED v4；审计在当前记录追加原 v3 的失败事件，完整比较取消结果和当前主单，业务事实/旧历史未被覆盖，成功结果、通知和目标为零。 |
| 回滚失败与并发 | 真实约束、外部漂移、规则/结构变化、九个持久化故障点、原目标保护、回滚/完结及双回滚竞争仍验证整单原子性。失败后的 `assertReleaseFailureOnly` 深比较所有业务事实及旧历史，只允许新增一个有操作人、时间与原版本的失败事件。 |

[机器清单](2026-09-10-safe-manual-retry.json) 列出 **32/32 受影响 MySQL 顶层用例**的实际 PASS 来源，缺失0。最终内部具名执行尝试重构后的 `/private/tmp/rcc-t5-evidence/backend-reviewed-final.log` 重验上述五个关键边界，全通过；其余不变路径复用本票已通过记录。未对每票重复全仓数据库套件；未修改的多表执行基础复用 [#84 的 284 项真实集成清单](2026-09-10-multitable-integration-manifest.json) 与主分支集成证据。

## 原动作与输入保护

未知响应只显示真实错误并刷新正常主单读取；原业务按钮复用完整原 key/body。只在实际发送期间互斥，不自动发送，不再提供独立未知结果确认入口。已知冲突仍可显式核对最新配置并确认新请求。旧发布重放的历史 SUCCEEDED 结果不会写回缓存覆盖当前 ROLLED_BACK；旧编辑重放也不会将 PENDING_APPROVAL 主单倒退到旧 DRAFT。

浏览器 IndexedDB 保持完整账户隔离日志、事务提交后才发送、按精确键清理、同范围不覆盖旧请求。已确认业务失败也保留原请求；HTTP 成功之后本地删除失败仍触发正常主单刷新，不冒充业务失败。冷账户读取日志完成前只等待新建入口；其他工作区查询不受门禁影响。

**13 个受影响 Web 文件，213/213 PASS，跳过0**，含请求解析、权限/会话、发布页、配置页、Change Set、未保存保护和真实 TCP 响应中断。机读用例与来源见同一清单及 `/private/tmp/rcc-t5-evidence/web-reviewed-final.json`。审查补充覆盖：

- 延迟冷 IndexedDB 读取时零发送，水合后用原键和完整两表申请保存；完整明细摘要标明全局位置、实际表、操作和记录身份。增量 upserts 缺少全局顺序时只显示稳定明细标识，不伪造全局位置。
- 当前主单已进入待审批时仍能以当前 EDITOR 权限重复旧编辑请求，正常读取保持真实待审批状态。
- 配置页单条/批量未知结果后 Escape、取消及返回修改可用，退出不清原日志、不发送额外写请求。
- 另一窗口留下的不同 create 请求被安全重推后，当前配置页单条输入与批量选择均保留；只在结果对应当前窗口的标题、去向和明细时清理输入与导航。

## 浏览器与可视检查

真实 Chrome → Web 同源代理 → Admin → 一个隔离 MySQL，受影响路径均有 PASS：草稿6、审批9、批量发布5、原单回滚7、多表8、字段恢复4，以及 accounts 父路径。最终 `/private/tmp/rcc-t5-evidence/browser-review-final.log` 复验 accounts 和后三个受审查修复影响的子路径。

多表路径完成 UI 晚表复制冲突 → 保留两表申请 → 用户在原复制动作再次确认 → 成功新草稿 → 新旧双向链接；重新准备也通过真实原操作跨提交前/后响应丢失，保持相同 key/body。真实应用 9,437,321 字节申请仍在刷新、撤权及账户切换后用原 key/全文恢复同一草稿，且存储写失败零发送；没有 sessionStorage 正文回退。

独立浏览器套件：`write-recovery-step2` **28** 检查、`browser-accessibility-final` **7** 检查、`complex-fields-final` **42** 行为证据，均 `ok=true`、页面错误0。前者含仅本次夹具数据库 stop/start 后原键恢复，保持原端点；后两者覆盖 320/390px 键盘/滚动和真实字段/身份矩阵。目录里的原始 JSON 保留预期数据库拒绝边界，未把预期 SQL 错误写成业务成功。

已实际查看 [快速回滚错误桌面](2026-09-10-safe-manual-retry/quick-error-desktop.png)、[快速回滚错误390px](2026-09-10-safe-manual-retry/quick-error-390.png)、[草稿错误390px](2026-09-10-safe-manual-retry/draft-error-390.png) 和[多表复制晚项冲突](2026-09-10-safe-manual-retry/copy-late-conflict.png)。前两张明确滚动到真实错误正文，原请求说明与正常确认按钮完整可读。字段恢复脚本在第三张状态实际 Escape → 确认离开 → 普通新建草稿入口再次保存，精确比较原 key/body 和真实 v1 草稿。

## 验证过程与评审

普通 Go `go test ./...`（含架构规则）、`go build ./...`、Web 类型检查和生产构建、11个受影响浏览器脚本语法、`git diff --check` 均通过。完整日志及 SHA-256 记录在机器清单。

TDD 原始 RED 保留：缺失败回执、已失败未成功前同键异正文未绑定、旧独立重试交互、冷水合入口、原请求多表身份、未知退出门禁和配置页跨窗口输入。`backend-affected-step1` 的旧整单不变断言在允许唯一失败事件且深比较剩余事实后复验通过。初次审计竞争 trigger 夹具被既有业务 trigger 合约提前拒绝；改用真实外部唯一键事务/锁等待，不放宽生产合约。`web-affected-final` 的真实 TCP 测试受 sandbox EPERM，授权监听后通过，最终全选定范围也通过。浏览器初次旧 locator/异步主单读取导致断言早于原请求完成，以及一次缺输出父目录的 setup 失败均保留；最终等待真实重推响应并复验，没有把失败运行标绿。旧 quick-unknown 截图的错误正文被裁掉，最终两种视口重新滚动并实际检查。

Standards / Spec 两个独立只读审查以固定基点的实际 tracked/untracked diff 为准；首轮分别审查，修复后各自增量复核，未重跑数据库或修改文件。

- Standards：冷水合入口、原请求多表身份、终态前进后的原编辑重推和陈旧文档均修复；主观 Data Clumps 建议通过具名 `releaseExecutionAttempt` 收束。最终硬性违规 **0**、主观坏味道 **0**。审查者核对 32 条 MySQL PASS 来源、213 条 Web 机读用例、日志哈希及四张可读截图。
- Spec：首轮配置页未知退出门禁及跨窗口输入丢失两项 P2 均修复，并补单条/批量正式页面用例和真实浏览器退出后原操作重推。最终残余发现 **0**，未发现新规格缺口或范围蔓延。

因旧审查槽仍占用并发额度，协调者明确复用 `/root/issue_84` 和其 `standards_review` 两个独立只读角色完成本票审查；最终结论已同时回传实现者和协调者。

所有数据库验收串行，任意时刻最多一个本任务 MySQL，保留用例内部真实并发/锁屏障。只清理本次夹具资源，未操作用户开发数据库或全局 Colima；没有新增临时兼容、自动重试、旧发布数据迁移或独立回滚单。本票退出旧未知结果确认入口，后续 #87/#88 仍按父规格负责原因事后修改及其他原有残余结构清理。
