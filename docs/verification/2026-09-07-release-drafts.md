# #50 / 发布单 T3：持久草稿验收

固定基点：`18262bcf8585e3f7221a249a64266a7ae8190b75`（已交付 T2）。分支 `codex/issue-50-release-drafts`，独占工作树 `.worktrees/release-order-t3`。来源为 GitHub #50、父 #48 的 AC-012～017 及已批准的 `release-order-tickets/03-release-drafts.md`。本单不修改主 checkout、T1/T2 工作树或父规格状态。

## AC 与证据

| 用例 | 已执行证据 |
|---|---|
| AC-012 保存意图而不执行 | `TestReleaseDraftSaveAndReload`、`TestReleaseDraftDiffAndServerBaseline`、`TestReleaseDraftMissingIdentityAndTombstone` 通过认证 HTTP 保存 ADD/MODIFY/DELETE，校验永久申请人、服务器 before、独立版本及重读；草稿保存不改变业务行，不创建记录版本、不占用目标。真实浏览器从数据页保存后直接查询 MySQL 对应业务行仍为 Alpha、版本仍为 0。 |
| AC-013 所有权与多窗口 | `TestReleaseDraftCurrentAuthorizationAndListing` 证明 EDITOR/ADMIN、VIEWER、仅 PUBLISHER、他人及撤权后的授权；`TestReleaseDraftCASCancelAndIdempotency` 和真实并发测试证明旧版本不能覆盖。Web 与真实两窗口验证冲突保留输入、先查看最新单据、明确重建后另发请求。 |
| AC-014 完整差异 | `TestReleaseDraftDiffAndServerBaseline` 证明 value、SQL NULL、空字符串、未提交、不存在、生成列、自动字段待执行的不同语义；before 和自动字段伪造被拒绝。`TestReleaseDraftRejectsLossySnapshotAndRetainsSavedSchema` 验证无法无损快照的 BLOB 表前置拒绝、操作人字段容量不足拒绝，以及保存后 Schema 改名仍可读完整旧差异。 |
| AC-015 筛选与当前动作 | 授权/列表测试覆盖表名、申请人、状态、单号、游标及 404。`TestReleaseDraftReplayUsesCurrentActionsAndRejectsChangedDigest` 证明原结果重放保留原业务版本，但动作按当前单据状态重新求值。页面提供正式列表/详情、筛选、历史和状态动作。 |
| AC-016 取消与持久历史 | CAS/取消测试验证原因、版本推进、原键重放、再次取消新键拒绝、无物理 DELETE 接口；真实浏览器取消后刷新，仍能筛选和查看历史，VIEWER 无编辑/取消动作。 |
| AC-017 幂等和不确定恢复 | 创建/编辑/取消都按永久账号、操作与请求键去重，内容摘要冲突明确拒绝；`TestReleaseDraftConcurrentAndAtomicStorage` 验证同键并发只有一单、不同键 CAS、数据库故障整笔回滚及 Admin 新实例恢复。真实浏览器让服务器实际成功后中断响应，刷新使用原键原内容找回同一单。Web 验证账号隔离、后续 403 仍保留原请求、明确版本/状态拒绝后才解除未知状态并要求核对重建。 |

## 新增边界的真实验证

已知但不存在的 ADD id 复用 T2 的 MySQL 主键身份机制。真实 `utf8mb4_0900_ai_ci` 大小写/重音等价、PAD SPACE、DECIMAL 和 TIMESTAMP 缺行身份与随后实际插入身份一致；初始 0、删除墓碑 2、维护代际 `9007199254740993` 均正确保留。`TestReleaseDraftKnownAddUpdateRequiresOriginalBaseline` 证明已保存 ADD 编辑不能省略记录令牌偷偷采用新墓碑。`TestReleaseDraftPreviewRebuildsMissingAddBaseline` 证明只读 preview 返回新缺行基线而不创建草稿，用户明确采用后可成功保存；编辑换 id 也可沿此入口读取目标基线。Web 进一步防止查看预览后再改目标仍采用旧预览。

`TestReleaseDraftSchemaReadiness` 验证缺控制表、错误存储引擎不能 Ready，010 迁移可重跑。历史升级 fixture 已补齐 010，完整旧库升级与新库 Schema 比较纳入正式全套。

测试先行发现并修复：初始草稿路由缺失；旧 ADD 编辑省略版本会接受最新墓碑；缺少只读新基线入口；自动操作人字段容量不足仍保存；原创建请求重放误提供已取消单据的编辑动作；浏览器列表状态选择器缺少稳定可访问名称；预览后改 id 未清除旧预览；未知请求得到明确冲突后仍永久锁定原键。各项保留对应红测及转绿记录。两轴评审又以有效红测证明并修复：刷新后的恢复卡片丢唯一原申请、普通新编辑绕过明确重建并覆盖原申请、重放旧结果掩盖当前详情、同名字符串冒充特殊状态、取消恢复误称保存。

## 验证状态

- `make test`、`make build`：全部 Go 模块、构建与既有依赖方向检查通过，新增草稿服务不得保存请求身份、草稿事务不得暴露业务写端口的机器检查通过。
- `pnpm --dir web test:run`：232 项通过（23 文件）；`typecheck` 和 `build` 均通过。全套曾有一处既有账号测试在导航过渡期间读到资料页同名输入框，修正为等待原工作区路径恢复后断言，再跑全部测试通过。
- `make test-integration`：第一轮完整命令实际退出 0（Admin 集成包 1178.173s、HTTP 包 116.541s，其余包均通过）。此轮启动后 HTTP 原结果重放动作逻辑发生修复，因此另做最终完整重跑。最终冻结后 `make test-integration` 已实际退出 0：Admin 集成包 1226.943s、HTTP 包 126.818s，其余包全部通过。最终后端/迁移 diff SHA-256 为 `e57143463d87e9221a65563b4b7c0a1cf0f2569382d60f79d46c085702b8f997`；运行期间只有 Web 评审修复，后端快照未改变。
- `make test-browser`：最终正式入口完整通过（53.265s），实际包含 `release-drafts.cjs`（10.17s）。除成功提交后丢响应找回原结果外，还真实验证未知编辑遇到另一窗口改版本、刷新原键 409、再次刷新仍保留原意、查看当前差异后明确重建。详情和 390px 列表截图已检查，表内横向滚动保留长 ID 可读性。中途一轮原账号脚本注册导航超时；后续正式全入口重跑成功，未以跳过替代。
- 两位只读子代理完成 Standards / Spec 初评及修复复审，最终均为 0 未解决。Standards 原 3 项成文问题（详情重读、GET 有限重试、请求封装）和 1 项重复逻辑建议已解决；Spec 原 3 项问题（刷新后拒绝保留、原结果后当前详情、特殊状态同名值）及普通保存覆盖待重建申请的旁路全部修复。取消恢复按钮按实际动作明确表达。
- 评审最终代码快照为 `58e4a129e45f7612e8b9e23a2d07a26b0a770901eb012b0e7d8728272ac143ea`（完整 diff 的 SHA-256），复审后只补充验证文档/截图及外部收尾记录。

截图：[取消后完整详情](2026-09-07-release-drafts-detail.png)、[390px 列表](2026-09-07-release-drafts-mobile.png)。

## 架构和退出责任

草稿只通过 Application 的 `ReleaseOrderSession` 读取规则、live Schema 和一致记录基线，并写控制数据；接口没有业务行写能力。Infrastructure 持有 MySQL 事务、比较规则身份、请求去重、草稿及历史的原子持久化。HTTP 只消费 Application 契约；内部 `RecordKey` 不公开，客户端无法提供。业务数据读取及独立版本沿用 T2。

控制表 `rcc_release_orders` 和 `rcc_release_requests` 是后续审批/发布的持久基础。当前仅 DRAFT/CANCELLED 写动作；没有批准、执行、Runtime 分发或伪造成功能力。保存的字段类型/空值语义用于草稿历史，不宣称它等同于最终发布 Codec 或完整冻结的执行 Schema。

临时单明细上限及对应单项编辑界面由 T6 #53 移除；旧默认确认/直接写 API 和既有旧入口验收由 T5 #52 统一迁移删除；最终值 Codec（含二进制固定样例）仍由 T5 交付。没有新 Feature Flag、双写、历史删除或自动清理入口。

## T4 继承

T4 #51 复用 `ReleaseOrderSession` 事务和幂等请求能力扩展状态转换、审批历史及目标占用。`ReadRecordBaseline` 返回业务行或缺行、真实数据库比较身份和含墓碑/维护代际的版本；已保存的内部 RecordKey 可用于同一数据库身份下的占用，但提交时必须重新核对基线/规则并冻结内容。新增 `/release-orders/preview` 只服务明确重建，不分配目标或保存结果。历史详情从持久 document 渲染，不依赖后来 live Schema/Policy。

Notion PM-020 仍为进行中，只记录草稿完成、审批与执行待后续工单；PM-012 只追加草稿事件审计的实际部分交付。提交、远端核验、#50 关闭及 Notion 局部更新结果由 #50 完成评论交叉记录。父 #48 保持 OPEN，未合并 main 或部署。
