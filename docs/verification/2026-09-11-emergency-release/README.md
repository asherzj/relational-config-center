# #104：应急草稿切换与整单发布

最新状态：用户明确授权后，真实浏览器 `03` 已通过，七张最终截图已逐张核对；后续冲突恢复窄修复、最终相交回归、类型检查与构建均已通过，独立 Standards 与 Spec 最终增量复核均为 0 项未解决问题。当前有效覆盖为 38 个唯一顶层 HTTP/MySQL 用例、433 个唯一 Web 用例及本票真实浏览器路径。源码和证据待 root 核验后暂存、提交与推送；工单关闭及项目记录由 root 负责。下方保留之前检查点的实际失败与审批中断记录。

固定基点 `e2564842e72e96b6ffb1e86708978b11dd8a651b`，独立分支 `codex/issue-104-emergency-release`，工作树 `/private/tmp/rcc-issue-104-emergency-release`。本票负责父 #100 的 AC-012、015、016、017、018、023；原单回滚模板和通知接入仍归 #105，功能级全量集成归 #106。`spec/` 保留实际读取的完整父规格和本票 ready 快照；#104 正文读取时评论为空。

## 行为及验收接缝

正向草稿整单选择常规或应急，明确切换时全部参与表按当前关联重新实例化；普通编辑仍保留已保存实例。应急提交要求原因，进入真实 `PENDING_PUBLICATION`，没有审批责任、批准事件或业务写入。当前 PUBLISHER 可手动执行，包括具备权限的原申请人；撤权也阻止已成功原请求的重放。成功待人工完结并保留目标，完结释放占用，不再允许回滚。

HTTP → Admin → 真实 MySQL 是授权、持久化、原请求、事务和竞争的主接缝。数据库故障注入使用真实表不可用、触发器和锁，不 Mock 自有服务或仓库；断言通过公开查询和发布单接口观察。React 组件测试负责交互及浏览器请求日志，真实 Chrome → Vite 同源代理 → Admin 进程 → MySQL 补系统路径与视觉验证。

## 验收外循环

原始日志保持实际退出结果，不把红灯日志改写成绿灯。`backend/initial-exit-codes.json` 记录前十一轮工具会话的最终退出码；后续每个检查的 `.json` 记录精确命令、环境、退出码和输入文件 SHA256。

| 验收 | 可观察事实 | 初次原始证据（backend/） |
| --- | --- | --- |
| AC-012 | 两表切换、回到常规、整单版本、原实例重推、过时拒绝 | `01-switch-red.log` → `02-switch-green.log` |
| AC-012 | 真实数据库交叠下切换先提交和提交先提交，另一个窗口拒绝且无混合类型 | `09-switch-submit-race.log` |
| AC-015 | 空白或超长原因拒绝、有效原因原文保存、无假审批、提交不写业务、真实待发布列表 | `03-submit-red.log` → `04-submit-green.log` |
| AC-016 | 仅编辑者拒绝、实时授予/撤销 PUBLISHER、具备权限的申请人手动发布、原请求重新鉴权 | `05-permission-red.log` → `06-permission-green.log` |
| AC-017 | 外键约束要求的跨表全局顺序；请求结果保存故障回滚全部业务与版本；原包恢复后一次成功 | `07-transaction-lifecycle.log` |
| AC-018 | 成功保留保护及回滚预览、人工完结不写值、完结释放目标且不再提供恢复、历史原结果与当前详情分开 | `07-transaction-lifecycle.log` |
| AC-023 | 模板读取、实例保存、切换/提交结果保存真实故障；原草稿不变；恢复后原键原正文返回同一实例 | `08-storage-recovery.log` |
| 受影响消费者 | 待发布列表允许动作、取消、复制与重新准备，派生新实例且新单原因为空 | `10-pending-consumers-red.log` → `11-pending-consumers-green.log` |

AC-017/018 的整单执行及完结能力原已存在，接通新的应急准入后首次路径直接通过，未人为制造红灯。源头失败边界覆盖本票实例路径；迁移及无影响执行器深层能力采用有效依赖证据，不声称重复验证。

## 受影响范围与依赖证据

公共状态、草稿输入、提交输入和审批读取分支发生变化，因此补跑本票全部 HTTP 验收，以及常规实例隔离、提交冻结、真实表审批、当前资格、发布/完结、失败历史、复制/重新准备、多表原单执行。全仓完整测试留给 #106；不是将已知失败延期。

#103 的既有证据位于 `../2026-09-11-release-instances/README.md`。本票没有配置 Schema 或迁移修改，仍保留两张配置表和模板的完整多节点 `node_list`；两类模板可有多份、每表每类型唯一关联不变。Goose 与候选累计 manifest 复用 `../2026-09-11-template-approval-integration/README.md`。`unchanged-boundaries.json` 核对历史迁移和历史证据逐字节不变。

当前仍保留 #103 登记的 `ROLLED_BACK` 旧总览分支，正向常规与应急使用真实逐表实例；#105 接入原单回滚模板，#106 移除旧总览。缺配置或存储故障不得使用此分支兜底。本票没有新增配置表、兼容双写、自动重试、独立确认接口、独立回滚单或通知实现。

## 交付门禁

待受影响检查、真实浏览器和独立 Standards/Spec 双轴评审收口后，补本文件最终结果、源文件清单与证据散列，由 root 核对后精确暂存并按授权提交/推送。当前不把尚未完成的 #100 整体功能标记完成。Notion、GitHub 完成评论和关闭由 root 负责。

## 增量回归记录

`backend/12-http-regression.log` 实际 37 个顶层用例中 35 PASS、2 FAIL，exit=1。失败不是应急业务结果：`TestReleaseFailureAuditPreservesConcurrentCancellation` 等待旧订单锁，未覆盖已有授权锁；`TestOriginalOrderRollbackPreservesApplicationAndBothExecutions` 错将当前资格修订值作为固定历史字段。`14-isolated-red.log` 单独复现这两项及新提交专用输入负例，exit=1，原字节保留。

两处旧夹具修正参考已独立验证的 #97 提交 `7334e1103f612b4735992ef63bf1a0b7643b40ae`，仅采用认证后 HTTP body gate、当前授权锁的精确 blocker 等待，以及实时资格来自当前详情的比较边界。本票没有移植通知代码或通知断言；仍逐一比较业务结果、版本、状态、目标和历史。

Standards 初审的 P3 主观建议为提交输入不应扩展其他动作。本票增加 `SubmitReleaseOrderInput` 将 `emergency_reason` 限定在提交；execute、complete 和回滚预览仍使用原 version-only 输入。`14` 公开 HTTP 原因字段负例先失败；`15-isolated-fixes.log` 六项全部 PASS，包含这项契约、两处旧回归及受影响应急提交、发布和完结。`13-contract-compile.log` 中 Application、Domain 与 HTTP 测试通过；`16-all-admin-compile.log` 编译全部 Admin 包及 integration/browser 测试（`-run '^$'`，不声称全套测试执行）。

浏览器脚本初次文件写入被自动审批拒绝，理由为仅凭目标 URL 无法证明环境隔离。更安全实现要求显式 `RCC_E2E_ISOLATED=1` 并只接受 loopback；Go harness 仅在自身创建临时 MySQL 和本地进程时传入该标识。没有改用外部写入或规避审批。

该更安全版本第二次仍被拒绝：自动审批认为 loopback 与环境标识尚不足以证明准确目标可销毁，要求先获得知情的明确授权。当时脚本没有创建或执行；`web/helper/approval-rejection.txt` 保留拒绝原文，`browser-approval-plan.md` 列明一次性容器、本地进程来源及拟写入范围。该检查点的真实浏览器、视觉验收和最终 Web 增量双轴评审尚未完成，因此停止提交、推送和关闭 #104。

## 已完成的非浏览器验证

`backend/17-submit-original-final.log` 对最终提交专用输入补验应急原请求恢复和常规提交冻结，两项 PASS。`backend/effective-results-final.json` 逐一映射最终有效日志，共 38 个唯一顶层 HTTP + 真实 MySQL 用例有效 PASS；保留 `12` 和 `14` 的原始失败，不把子测试或重复补验累加。

Web 原批 `web/helper/19-web-full-suite.log` 实际 426 项中 414 PASS、12 FAIL，另有 1 个未处理的 EPERM 监听错误。11 项 ManagedDataPage 失败源于共享表规则夹具遗漏公开 DTO 必需的 `version`；原批和单独复现 `20` 保留，仅补 `version:"1"` 后 `23` 全部 11 PASS。另一项是未修改的真实 TCP 截断传输测试受沙箱监听限制，独立获准监听 loopback 后 `web/24-client-stream.log` 为 1 PASS；此测试不启动 Admin、数据库或被拒浏览器脚本。

因此 Web 当前有效覆盖为原批 414 + 夹具补验 11 + 传输补验 1 = 426 个唯一用例 PASS，详见 `web/effective-results-final.json`；不是声称存在一次全绿的全量运行。发布单与 API 的 90 项（`16`）及修改页的 30 项（`18`）已包含在原批中，不重复计数。TypeScript `21`、生产构建 `22` 均通过。

后端初审 Standards 为 0 项成文规范违规，1 项 P3 输入职责建议已按公开 HTTP 红绿修复；独立 Spec 后端初审为 0 项问题。原报告及原时点散列保留在 `reviews/`，最终增量评审尚待完成。`non-browser-source-sha256.json` 固定当前 32 个源码/领域/接口文档文件；`non-browser-source.patch` 保存已跟踪文件差异，`non-browser-new-test.patch` 单独保存尚未暂存的新 HTTP 测试。此快照不宣称包含之后的浏览器或最终设计文档收口。

## 明确授权后的系统验收与评审修复

`browser-authorization.md` 记录用户在知悉测试目标和写入范围后明确同意继续。`browser/01-run.log` 为原始失败，原因是脚本未等待实际重推响应；`02`、`03` 均为实际 PASS，最后一轮只补稳定手机视口截图。每轮建立新的 MySQL、Admin、Vite 及浏览器上下文，未使用或清理既有预览容器。最终运行身份、源码输入与清理检查见 `browser/03/`；七图的逐项视觉核对与有效范围见 `browser/visual-verification.md`。`web/DESIGN.md` 已同步正式应急规则和恢复边界。

Web 首轮增量评审发现确定冲突时未按最新发布方式处理原因、应急复制/重新准备仍暗示审批、成功缺反馈，以及应急仍显示最近审批人。`web/review-fixes/25`→`26`、`27`→`28b`、`29`→`30b`、`31`→`32` 分别保留红绿证据；`28` 的选择器歧义失败也保留。完整发布页面/API `33` 为 93 PASS，`34` 类型检查与 `35` 构建通过。成功反馈采用中性“应急发布已提交”，历史原请求重推不宣称当前仍待发布。

两轴最终增量共同发现同一读取门禁问题：预览失败、再次 GET 失败或最新状态无提交资格时，原因编辑仍可重新生成提交包。`web/39`、`40` 保留该三项及成功反馈缺失的 4 个实际失败，未知结果保护原本通过。修复后 `41` 六项 PASS：每轮检查先撤销重建许可，只在本轮完整读取、预览成功且有当前提交资格、基线未变时启用原因编辑及重建；失败保留原因与原请求。确认重建成功显示中性提示，未知结果不显示成功且可从原操作恢复原包。

最终 `web/42-final-conflict-regression.log` 为 108 PASS，包含发布页 88、API 9、复用恢复组件的配置内容页 11；`43` 类型检查、`44` 构建及 diff-check 通过。唯一有效 Web 数为原 426 + 首轮新增 3 + 最后新增 4 = 433，不重复计算相交回归。`web/effective-results-delivery.json` 映射最终覆盖；`browser/03` 之后仅确定冲突恢复组件及测试变化，浏览器所走未知结果原包路径未变，该路径证据继续有效。没有为这项局部修复重跑全部后端或全仓套件。

`unchanged-boundaries-final.json` 再次逐 Git blob 比较固定基点的全部 1,109 项迁移和历史验收文件，并列出当前 SHA256，全部未变。最终源码及文档范围见 `delivery-source-input.json`，待交付文件和证据散列将由 root 在评审报告齐全后核验，之后才允许暂存、提交和推送。

最终独立报告及各自输入清单原字节归档在 `reviews/rcc-104-standards-final-round2.md`、`reviews/rcc-104-standards-final-round2-source.json`、`reviews/rcc-104-spec-final.md`、`reviews/rcc-104-spec-final.inputhash.json`；两轴均关闭全部发现，未读取对方报告，未将本票放行扩大为 #100 整体完成。`delivery-files.txt` 列出本票待提交的全部源码、文档和证据（含被忽略的原始日志）；`delivery-evidence-sha256.json` 对证据逐文件记录 SHA256，自身除外。root 在精确暂存前还需独立比对该清单和当前字节。
