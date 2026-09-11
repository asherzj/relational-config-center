# #103：常规多表发布流程实例

本票只承担父 #100 的 AC-009、010、011、013、014、022。固定基点 `f7b3f7bbdb42665b67a590b1b182a86f5ecf3d32`，分支 `codex/issue-103-release-instances`，工作树 `/private/tmp/rcc-issue-103-release-instances`。完整工单正文、空评论及标签保存在 `spec/issue-100.json` 和 `spec/issue-103.json`。本目录与早期 Web 子代理说明分别记录自己的时点，早期失败、源码快照和评审原文均不回写。

实现使常规草稿在首次响应前保存各参与表的独立流程来源和节点。普通编辑保留已有实例；配置缺失可以保存，但不能提交；配置修复后显式保存仅补缺，GET 和提交都不实例化。提交时沿用已保存节点，另按 #94 冻结表角色身份，成员资格实时计算。发布与人工完结推进实际节点。快速回滚仅在原事务停止尚未完成的正向节点，保留真实审批和发布事实，不生成回滚模板实例。

## 验收外循环

公共鉴权 HTTP → Admin → 真实 MySQL 是持久化、角色和竞争事实的主验收接缝。React/API 接缝负责页面行为及请求日志，真实 Chrome → Vite 同源代理 → Admin 进程 → MySQL 负责跨进程和交互证明。故障与交叠只在真实数据库锁/触发器/表可用性边界安排，不 Mock 自有内部协作者。

| 责任 | 可观察事实 | 原始红 / 绿证据（backend/） |
| --- | --- | --- |
| AC-009 | 两表使用不同已保存实例；模板在首次 GET 前修改，详情仍返回原实例 | `01-ac009-red.log` → `02-ac009-green.log`；最终 `15-final-http.jsonl` |
| AC-010 | 缺常规配置显式列出且提交 409；修复后 GET 不写；原内容保存仅补缺；模板停用/换关联不替换旧实例 | `03-ac010-red.log` → `04-ac010-green.log`；最终 15 |
| AC-011 | 同表编辑/新增其他表保留旧实例；移除最后明细丢弃该表实例与占用；重新加入取得当前模板；竞争失败无部分保存 | `10-ac011-shared-save-probe.log` 首次已绿，调查确认 AC-009/010 的共享保存实现已满足，未制造假红；最终 15 |
| AC-013 | 复制/重新准备从当前关联新建实例，不继承原单节点进度/批准事实；占用转移、源单关联和原结果保持原子性 | `05-ac013-red.log` → `07-ac013-green.log`；最终 15；目标竞争补验 18 |
| AC-014 | 草稿节点与提交时审批分配分离；旧角色/申请人拒绝；逐表批准只推进本表，全部通过后整单发布；人员/时间来自真实决定与事件 | `08-ac014-red.log` → `09-ac014-green.log`；最终 15 |
| AC-022 | 真实管理 PUT 与草稿保存交叠，仅见完整旧/新模板与关联快照；已成功原请求仍返回原实例，不重新实例化 | `11-ac022-snapshot-probe.log` 两个交叠首次已绿；同事务单 JOIN + RR 已在前序切片实现，未制造假红；最终 15 |
| 当前授权 | create/copy/replay 等待账号维护锁后再读取当前权限；撤权提交后返回 403，无新单/新实例；旧会话随后 401 | `12-current-authorization-red.log` 三子例原先错误 201 → `13-current-authorization-green.log`；最终 15 |
| 失败原子性 | 配置读取、主单保存、原结果保存真实失败均不改变内容/版本/实例；恢复后相同正文和键可成功；GET/结果重放不写主单 | `14-save-failures-probe.log`；最终 15 |
| 回滚终止事实 | 既有原单快速回滚保留已完成的审批/发布，未完结正向节点 STOPPED，无虚构人员时间；GET/原键重放一致 | `19-rollback-flow-red.log` 原先 ACTIVE → `20-rollback-flow-green.jsonl` |

`06-ac013-green.log` 的实际 exit=1 保留：测试曾错误要求派生草稿连当前“提交前审批安排”也为空；更正为不继承已完成决定且节点 PENDING 后，07 真正通过。文件名不是结果，实际退出码和断言日志为准。

## 有效回归与命令

全部 MySQL/HTTP/browser 依次独占运行，使用本机一次性隔离容器，没有修改共享数据库或部署：

```sh
TESTCONTAINERS_RYUK_DISABLED=true
DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock
GOCACHE=/private/tmp/rcc-go-cache
```

每个 `.exit` 记录对应进程的真实退出码。`backend/15-final-http-plan.json` 包含精确范围和正则；15 使用 `go test -json -tags=integration ./admin/cmd/admin -run <regex> -count=1 -timeout=15m`。`backend/effective-final-http-results.json` 逐用例记录原始批次与当前有效结果。

- 15 原始批次为 26 顶层：24 PASS、2 FAIL。18 精确补验两个失败，均 PASS。原批次从未改记为全绿。
- 两项窄夹具适配：在途撤权测试显式绑定常规模板；重新准备竞争测试等待 #94 已有授权控制锁，再核对完整目标转移和新占用者 409。未增加运行时默认、降低授权或删除原子性断言。
- 回滚节点修复的 20 批次包括真实混合恢复/原结果重放、与完结及其他回滚竞争、存储失败原子性，三项 PASS。与此变更相交的既有分页、混合发布与进程重启三个用例在 `backend/21-rollback-caller-regression.jsonl` 窄复验三项 PASS。最终 29 个唯一顶层 HTTP/MySQL 用例全部具有当前有效通过证据。
- `backend/16-units-and-contracts.log` 中 domain、application、HTTP 通过（含公开路由/容量、HTTP 依赖方向机器检查）；cmd/admin 仅因 sandbox 禁止本地监听失败。已授权监听环境中的 `backend/17-process-and-boundaries-recheck.log` 为 PASS 20.730s，非业务修复。

Web 有效 148 个测试，来自六个文件，按真实依赖复用：

| 范围 | 当前有效来源 |
| --- | --- |
| ReleaseOrdersPage 77 + UnsavedChanges 24 | Toast 生产改动后 `web/18-feedback-affected-regression.log`，101 PASS |
| release-orders API + release-journal | `web/10-affected-regression.log` 的 10 PASS；这两类源码及生产依赖未受 Toast/回滚后端变更影响 |
| ManagedDataMutationPage 30 | `web/13-consumer-regression.log` 的 29 PASS + `web/15-summary-consumer-green.log` 的 1 PASS；仅失败用例的响应夹具修正 |
| ReleaseDiff 7 | `web/13-consumer-regression.log`；没有生产依赖变更 |

Standards 发现的成功反馈缺失已使用 `web/16-save-feedback-red.log` → `17-save-feedback-green.log` 修复：已确认保存后 Sonner 提示“草稿已保存”；之后详情读取失败仍有独立错误/请求号。未知结果不显示成功。页面与关闭保护已完整复验，API/journal 不重复计数。`web/19-final-typecheck.log`、`20-final-build.log` 均 PASS。早期 Web 子代理的 147 快照保留于 `web/README.md` 和 `verified-source.json`，本节是增量后的有效范围；不能把历史总数直接相加。

## 真实浏览器

`browser/19-system-path.log` 首跑 PASS，`run-01` 保留其六张原图。`browser/20-final-system-path.log` PASS 31.52s；`run-02` 的七张图为最终主场景视觉证据，额外验证来源折叠区键盘打开/关闭，截图前返回页首避免 fixed 顶栏落在全页图中间。产品代码未为截图改变。

主系统命令：`go test -tags=integration,browser ./admin/cmd/admin -run '^TestReleaseFlowBrowserSystemPath$' -count=1 -v -timeout=6m`，每次显式传新绝对路径 `RCC_E2E_OUTPUT`。脚本拒绝覆盖已有截图/结果。`run-02/release-instances-browser-evidence.json` 记录真实主单身份、每阶段实例及原键/正文手动重推事实。

四个具名检查组覆盖：真实 Web 保存两表实例/刷新只读和来源键盘折叠；模板编辑停用/切换关联隔离；390px 缺流程阻止提交、配置补齐后 GET 不写、成功响应丢失后的刷新与原样手动重推；角色冻结、独立部分/全部审批、整单发布及人工完结的真实节点人员时间。AC-011/013/022 的主要数据库竞争事实由 HTTP/MySQL 验收负责，不声称浏览器重复覆盖。

人工逐张检查桌面与 390px：表名、不同长模板名与状态可辨；移动端节点纵向排列；缺配置只列缺失表且无伪造节点；来源 disclosure 有键盘焦点；部分批准时整单仍待审批；完整批准与完结的人员/时间清楚。移动页面无横向文档溢出，申请差异沿用局部横向滚动。

发布专用旧浏览器夹具新增表后也必须显式选 STANDARD。`browser/21-regression.log` 在业务断言前因未创建输出根目录而 ENOENT，保留原错误。后续 `22-regression.log` 使用新建独立目录，按子测试选择审批、批量、占用、多表、快速回滚和字段展示。22 实际五个子脚本 PASS、draft-targets 一项 FAIL，整体 exit=1；失败缘于空草稿不再提供 submit 按钮，测试仍等待禁用按钮。仅改为按钮不存在断言，`23-regression-recheck.log` 保留全部目标、冲突、两窗口检查并通过（子脚本16.86s，含 accounts 前置共53.20s）。其余无影响脚本不执行。回滚子场景检查持久节点为 COMPLETED/COMPLETED/STOPPED，页面没有进行中或 `aria-current=step`。22 的 release-approvals 检查文案仍写“four actual stages”，实际断言已是逐表实例与整单阶段；最终源码只更正该日志标签，未改变测试动作或断言。

`browser/24-table-approvals.log` 实际 PASS 37.38s，七个检查组继续证明无独立审批人拒绝、成员变化、按表授权、角色分配版本冲突、部分审批、全表批准和空快照独立 ADMIN 接手。`table-approvals/result.json` 保存具名检查，桌面/390px 图与范围几何数据一并保留。有效浏览器范围为本票主路径、六个既有发布子场景与正式按表审批路径，共 51 个具名检查组，另执行 accounts 共享真实登录/角色/发布前置。`browser/effective-browser-results.json` 逐场景指向实际通过运行。

## 结构、迁移与临时退出

新增实例字段由 Go 公共 HTTP 测试与 Web Zod 响应契约共同保护；接口和跨层机器检查沿用 HTTP 包中的契约、容量和依赖方向测试。不改变依赖方向，模板/关联一致读取位于 MySQL Adapter 的既有 RR 发布事务内，不把全局授权锁当成模板管理也被串行化的保证。

本票没有新 DDL，实例复用主单文档。基点已集成外部 6/7 与模板候选 8/9；SQL/累计 manifest 未改，历史接管 baseline 固定 5。新安装、只读 Ready、升级与真实 MySQL 清单生成证据复用 `../2026-09-11-template-approval-integration/README.md`；本票不重复无影响的 schema 套件，不恢复旧 SQL 或弱化就绪。

`admin/CONTEXT.md` 新增逐表流程/节点术语；ADR-0027 记录草稿实例与提交审批的不同固定时点、明确保存及事务一致读取；草稿、审批与表模板 API 文档更新；Web DESIGN 同步逐表节点与整单阶段的分工。

临时结构：`ReleaseProgress.tsx` 及 Page 的 `ROLLED_BACK` 旧总览分支仍用于既有原单回滚。正向常规路径已使用保存实例，缺配置绝不走兜底。现有回滚事务已正确终止未完成的正向节点；#105 负责新的原单回滚模板/实例与其节点推进，#106 负责移除旧总览并复验桌面、390px、键盘。保留原单身份、真实审批/执行/恢复历史，无独立回滚单或额外人工完结。root 在关闭 #103 时登记给 #106。#104 应急发布及 #105 通知未在本票宣称交付。

## 快照与评审

`review-source-sha256.json` 是首个最终评审输入时点，保留不改。`final-review-source-v3-sha256.json` 包含最终 43 个变更/新增源码文档；`final-source-sha256.json` 记录待交付 623 个源码及文档；`final-source-change.patch` 保留对固定基点的完整源码改动。`final-input-applicability.json` 列出各批次输入与最终版本差异，新增回滚分支及专属夹具仅使相交检查需要补验。`unchanged-boundaries.json` 核对原有历史证据和迁移字节未改。`reviews/` 保留独立两轴原文与其输入 hash；本 README 的复验说明不能取代评审。

独立两轴第三轮均为 0 当前发现：`reviews/rcc-103-standards-review-round3.md` 和 `reviews/rcc-103-spec-review-round3.md`。Standards 原成功反馈 P2、Spec 后续回滚节点 P2 均已以真实红绿及受影响回归关闭，初轮及中间报告原样保留。最终 43 变更输入与 623 全源码清单匹配；`source-sha256.json` 已同步最终值，旧时点另存 `source-sha256-before-final-fixture-recheck.json`。`evidence-sha256.txt` 覆盖本目录其他全部文件，不包含自身。

提交和推送须等待 root 核对验收、原始日志、当前有效覆盖及两轴最终复核。工单评论、关闭和项目记录由 root 执行；本文件不把尚未完成的 #100 整体功能标记完成。
