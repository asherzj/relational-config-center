# #84 多表有序明细与整单执行验收

固定实现基点：`192861fe845929721a7bfe5fc89114b8c7e791ba`。本票对应父规格 #81 的 AC-001、006、007、009、014；只在隔离工作树实现。#85～#88 的复制/重备最终交互、失败重推、原因编辑与过渡清理仍按 [T3 交接](../design-notes/multitable-release-tickets/t3-multitable.md) 分工。

## 业务证据

| 验收 | 实际证据 |
| --- | --- |
| AC-001 | 三表发布与原单恢复：同一主单、两条成功执行、申请及原发布事实不变，逐项明细保存恢复事实；列表按任一实际表筛选且只返回父单一次。`TestMultitablePublicationPreservesGlobalOrderAndOriginalResults` |
| AC-006 | 两表交替的 1,000 项按十页各 100 项增量保存，1,001 项原子拒绝；整单发布 1.276 秒、恢复 2.162 秒，沿用默认 4 秒业务期限。大值路径以 130 条、每条 90,000 UTF-8 字节，超过旧字段和整单预算，真实 HTTP 保存→冻结→独立审批→发布→详情→恢复，并逐值核对。计时仅为本机 fixture 测量，不是额外 SLO。`TestMultitableThousandPagedDetailsExecuteAsOneOrder` / `TestMultitableLargeValuesThroughHTTPLifecycle` |
| AC-007 | 既有完整权限/生命周期集成保留；跨表全部目标与引用随草稿取消、拒绝、批准后管理员取消、人工完结、恢复统一释放。审批/发布不释放，人工完结不改业务值且关闭回滚。两个真实窗口继续按整单 CAS 保存，冲突重建只应用本窗口改动。 |
| AC-009 | 已知 ID 外键交错：新增 parent2→修改 child1 引用2→删除 parent1→另一表自增新增，按任一表完整 regroup 都不能满足依赖；发布按冻结正序，恢复严格逆序。每表版本、游标、通知归属核对，自增真实 ID 继续占用。两表控制存储、晚表版本和通知失败均回滚前面的业务值及全部事实。`TestPublicationAtomicPersistenceFailures` |
| AC-014 | 多表明细显式表名、稳定 detail_id、分页及键盘排序；发布错误使用正向全局位置，恢复错误使用倒序位置。保留 main 字段编辑/NULL/生成字段和 T2 管控键/负责人/双窗口路径。桌面与 390px 测实际抽屉和内部控件边界。 |

`TestMultitableDraftWaitsForAllGuardsBeforeAnySnapshot` 将连接池限制为 1，使用 `performance_schema.data_lock_waits` 确认草稿已等待晚表 B 的规则锁，再提交 B 的外部变更；草稿读取提交后的 B。名称解析仅用当前连接的标量，不借第二连接、不在所有保护锁之前读取字典建立 RR 快照。`TestReleaseFreezeMetadataCaseInsensitiveNames` 真实 MySQL 8.4 mode1 验证大写规则创建、同物理表别名重复与跨单补充键保护；mode0 缺失精确规则时安全拒绝，不能借另一大小写表的规则。

发布链路的明细路由解除旧 HTTP envelope/字段/整单字节预算，其他 API 和无明细动作保持原限制。`TestReleaseDetailCapacityRouteContract` 使用实际路由注册，分别测试声明长度和流式正文的允许/拒绝集合。数据库语句分批组装始终属于同一整单事务，单个大明细可独立写入，不是新的容量上限。

## 检查与原始日志

日志根目录为 `/private/tmp/rcc-84-evidence`。本地完整 Go 集成清单为 **284 项，284 个最终真实 PASS，0 缺失、0 测试 skip**。逐项 `(package,test) → PASS 来源`、三个无测试包和四份原始日志 SHA-256 见 [精确清单](2026-09-10-multitable-integration-manifest.json)。没有将包级 `[no test files]` 冒充测试。

- 普通四模块 `make test`、`make build` 通过：`root-go-test.log`、`root-go-build.log`；评审修订后受影响 HTTP/application/domain/MySQL 普通包通过：`review-go-unit.log`；最新四模块构建通过：`root-go-build-reviewed.log`（成功静默，退出 0 由工具确认）。
- Web 全套 **333/333、29 文件**通过：`web-full-completed.log`，包含真实 TCP 中断；类型检查及生产构建通过：`web-build-final.log`；移除重复事件订阅后，受影响发布/未保存保护 **78/78** 再次通过（`web-journal-listener-reviewed.log`），最新类型及构建通过（`web-build-reviewed.log`）。
- 首轮 `go test -json -p 1 -tags=integration -count=1 -timeout=70m ./...` 保留在 `integration-full.jsonl`，**该次失败**：266 顶层 PASS、两个 cmd/admin 失败，15 个 Adapter 用例因混合时序编译失败未运行。目录不可用的早期规则锁错误曾错误映射为 500，已修正为既有 503 契约；另一个查询 case 在注册前 CSRF 过期，墙钟 GET 05:33:05→POST 05:49:11 超过 10 分钟，而 case 单调用时为 10.59 秒，符合宿主暂停影响，不修改时限。Adapter 在长运行期间读到新 helper 引用、依赖却已从旧源码编译，保留原 build failed。
- 最新完整 Adapter **15/15**：`integration-adapter-final.jsonl`；最新 cmd/admin 目标 **8/8**：`integration-cmd-reviewed.jsonl`，含上述两个原失败、大小写规则和全部新多表/大值/原子边界；新增 HTTP 契约 **1/1**：`integration-http-capacity.jsonl`。四份来源合并覆盖最终 284 项，不重复执行已经有效的其他案例。

## 浏览器

执行 `TestAccountBrowserSystemPath`，真实 Chrome→Vite 同源代理→Admin→单个隔离 MySQL；已有账号、未保存保护、规则说明、草稿、审批、千项批量、回滚、T2 占用和本票多表脚本全部列入范围。最初输出目录未建而 ENOENT 的 setup 失败保留在 `browser-output-setup-failed.jsonl`。

首次完整应用日志 `browser-full.jsonl` 保留真实失败，账号及未保存保护、规则说明、T2 占用脚本通过；`browser-reviewed.jsonl` 补齐草稿、审批与回滚脚本；`browser-final.jsonl` 中剩余批量和多表脚本及父测试全部 PASS（83.817 秒）。因此完整账号路径与八个脚本都有真实 PASS，逐项来源和 SHA-256 见 [浏览器清单](2026-09-10-multitable-browser.json)，没有将前两份失败的完整运行标绿。修订使用真实 POST/PUT 响应等待原键重放完成，不以刷新后已存在的终态替代发请求证据。390px 首次测量为抽屉进入动画帧，诊断坐标 left=357.67/right=707.67；保留截图后改为等待真实抽屉边界稳定，再检查输入及含表名选择框，未修改产品 CSS。

本票多表脚本从正式 UI 创建无起始表空单，跨表加入同 ID 明细，增量扩展 25 项后跨页编辑并键盘排序，独立审批、正序发布和原单倒序恢复。真实 **9,437,321 UTF-8 字节** 请求经正式页面发送且数据库保存，拦截成功响应模拟丢失；刷新、撤权、切换账号后，以原账号原 key/正文恢复同一版本 1 草稿并核对完整大值。真实 IndexedDB put 配额错误时 0 PUT，保留本窗口输入；open 不可用时账号及无关查询仍可访问，发布区显示读取错误。移动恢复图等待导航收起，定位第一条恢复明细并断言真实原值在 390×844 可视区。

已人工查看：[390px 编辑](2026-09-10-multitable/multitable-editor-mobile.png)、[桌面发布结果](2026-09-10-multitable/multitable-publication-desktop.png)、[390px 恢复结果](2026-09-10-multitable/multitable-restoration-mobile-final.png)。旧抽屉进入帧和侧栏过渡图只保留在临时证据目录，未作为验收截图。

根协调器另有实际 journal 模块的 Chrome 存储接缝证据 `root-journal-browser.json`：9,437,299 字节正文刷新后原 key/SHA 一致，同 scope 两窗口只有一个写入成功且胜者正文保留，账号隔离、精确 key 删除及真实 IndexedDB put 配额错误保留旧记录。此检查没有业务 API，不能替代上述完整应用路径。

## 审查与资源

Standards 与 Spec 由两个独立只读子代理审查固定基点以来的真实 tracked/untracked diff。前轮发现已修正：共享错误位置映射、规则查询可索引等值与精确名称检查、大写规则创建、恢复倒序错误坐标、冷启动水合和跨窗口本地输入、journal 故障影响范围。Standards 最终通过：硬性违规 0、保留主观坏味道 0；Spec 独立核对最终源码、284 项映射、完整浏览器范围和移动恢复截图，未解决发现 0。两轴均不把 #85～#88 的既定后续范围计作本票缺陷。

数据库验收始终 `-p 1` 或串行目标集，任意时刻最多一个本任务 MySQL 容器，保留用例内部并发与真实屏障。每次运行由其 testcontainers 清理自己创建的容器；未操作他人容器、既有开发库或全局 Colima 配置。
