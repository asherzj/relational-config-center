# T6 / #53：同表混合批量验收

固定基点 `39c632aa7560f900d2a76724898efe4f117d4145`；独立工作树 `.worktrees/release-order-t6`，分支 `codex/issue-53-mixed-batch`。主要接缝为已认证 Admin HTTP，真实 MySQL 独立证明持久化/并发，浏览器证明组织与整单审批发布；没有依赖缺失 skip 或模拟内部服务。

## AC 映射

| AC | 可重复证据 |
| --- | --- |
| AC-037 | `TestReleaseMixedBatchPublication`、`TestReleaseBatchExistingEnumPrimaryKeys`、`TestReleaseThousandItemsThroughExecutable`：同表 MODIFY/DELETE/ADD 集合、正式进程默认 4s 下 1,000 项。`release-batches.cjs`：数据页加入本人同表草稿、明确多行删除、任意明细编辑/移除、完整大单分页与审批发布。 |
| AC-038 | `TestReleaseBatchDuplicateIdentityDoesNotReplaceDraft`；`TestReleaseBatchEdgeDraftValidation` / `RequestLimitsPreserveDraft`：非法后项、数值/字符/PAD SPACE 等价重复、跨表、0/1,001 项与正文限制，原草稿逐字不变；`CopyInvalidItemIsLocated` / `FieldBudget` / `ExpandedResultBudgetRollsBack` / `BudgetRetainsCancellationHeadroom`：64 KiB 字段、8 MiB 实际结果及终止余量。HTTP `error.item_index`，Web 错误定位与发送前浏览器容量拒绝。 |
| AC-039 | `TestReleaseBatchEdgeOverlappingSubmissions`：交叉目标竞争唯一赢家、失败零残留、取消释放及原失败键重试；`StaleMemberRollsBack` / `MidwayConstraintRollback`：后项陈旧、真实 CHECK/业务唯一性中途失败，无业务/记录版本/Command/表版本/通知部分生效，APPROVED 与占用保留。 |
| AC-040 | `TestReleaseBatchEdgeAutoIncrementAndIndependentRequests` / `IndependentUniqueConflict`：真实 increment=3/offset=2 的逐项 id，原键结果相同，独立相似请求不合并，业务唯一约束真实竞争全批回滚；正式 1,000 项和浏览器丢执行响应恢复不重复新增。 |

## 垂直 red → green

- 混合三项创建原本 422 release_invalid；去除容量阶段后完整发布通过（9.004s）。
- 同一 id `1` / `01` 原本返回 200 且覆盖草稿；现在 422 release_duplicate_target、item_index=1，原草稿逐字不变（9.012s）。
- 正式 1,000 项旧路径 create=4.020s、submit=4.416s、execute=4.000s 返回 504；同表批量基线/前后行/版本及 Command 后全流程成功。每个新增仍单独取得真实自增 id。
- 65,537 字节字段原本可保存；现在 422 release_field_limit（9.299s）。130 项的小 ADD 请求由数据库默认值扩张为 8,700,890 字节结果，原本 200 提交；现在 422 release_result_limit，业务行/版本/Command/执行请求全为 0，原单仍 APPROVED（10.096s）。
- 接近上限的长历史冻结单旧审批门禁允许侵入所需的 64 KiB 预留（8,333,167 字节）；新门禁拒绝预留不足的审批，最长转义取消理由仍可正常取消并释放目标。顺序 EDIT 历史加真实 SUBMIT 夹具最终通过（10.761s）；没有声称复现旧取消锁死。
- 提交目标冲突原本只有通用 409；增加原索引后指明双方共同的第二项，失败仍零残留。
- Web 任意明细编辑/移除、已有草稿批量加入、千项定位和 sessionStorage 超额发送前拒绝分别先失败后通过。已有恢复请求和当前输入保留。

评审新增的真实 red → green：

- 原批量读取拒绝现存 ENUM 主键（create422）；保留已有 MODIFY/DELETE 与真实身份墓碑，大小写等价输入返回实际 alpha/beta，原键重放一致；缺行 ENUM ADD 仍按原边界拒绝（9.457s）。
- `__proto__` / `constructor` 原本被普通对象错误显示为已选；改为 Map 后两项能明确勾选并携带原记录版本提交。
- 千项编辑器原本渲染1,000个选项，删除确认原本渲染全部选择；共同分页器现在每页最多20项、直接定位任意序号，保存仍提交整个集合（相关19/16项测试均通过）。
- Copy第二项的申请变更或非法版本原本缺错误序号；现在error.item_index=1，来源单逐字不变、失败copy请求无残留，合法两项复制仍成功（9.098s）。
- 共享请求序号/候选值SQL构造后，真实FLOAT、ENUM、排序规则/PAD SPACE重复身份及正式千项再次全部通过（39.917s）。

所有 red 日志均保留在 `/private/tmp/t6-*-red.log`，不会列为成功证据。

## 正式进程代表样本

`go test -v -count=1 -tags=integration ./cmd/admin -run '^TestReleaseThousandItemsThroughExecutable$'` 构建并启动真实 `cmd/admin`。MySQL 8.4；未覆盖 MYSQL_* 超时，默认 socket 5s、实际发布请求 4s、HTTP read/write 各 10s。样本为同表 333 MODIFY、333 DELETE、334 无 id ADD，10 列（含默认值、生成列、SQL NULL 和 JSON）。请求 78,923 字节，原数据库 666 行。

完成容量/身份回归后的正式进程样本（`/private/tmp/t6-shared-lookup-regression.log`；默认启动配置保持不变）：

| 动作 | 请求字节 | 响应字节 | 端到端时间 |
| --- | ---: | ---: | ---: |
| 创建 | 78,923 | 1,627,879 | 0.210s |
| 提交 | 24 | 1,628,086 | 0.652s |
| 批准 | 59 | 1,628,226 | 0.148s |
| 执行 | 24 | 3,215,171 | 1.247s |
| 原键恢复 | 24 | 3,215,171 | 0.149s |

实际结果：667 业务行、1,000 条 Command、1,000 个记录版本=1、一个 Table Version、一条 NOT_CONNECTED 通知、零占用，逐项序号与真实 id 完整唯一。原键重放响应逐字相同。代表样本不构成任意大小 1,000 项或任意并发负载承诺。

## 架构与退出责任

- HTTP 只依赖 Application；真实机器检查捕获过直接导入 Domain 的中间错误，已修复为 Application 摘要契约。
- `ReleaseOrderSession` 仅批量读取基线与写控制数据，没有业务行写能力；`PublicationSession` 唯一拥有完整发布事务。版本仍为原 `rcc_record_versions` 与维护 floor。
- 单项数量限制、旧逐行基线读取/版本推进路径及编辑器 `items[0]` 限制已删除；旧直写路由和调用没有恢复。新增集合读取、版本整体推进与摘要为正式结构，无临时兼容开关。
- 领域概念/状态未新增；更新草稿和发布公开契约。保留 T5 FLOAT 身份维护要求。T7 必须使用 Command.before、实际 id/record_version、当前自动字段/生成列，并使用预留的终止/关联空间。
- 通知仍是正式持久记录，未接实际分发；没有 worker、部署、合并 main 或父 #48 关闭。

## 浏览器证据

正式 `RCC_E2E_OUTPUT=/private/tmp/t6-browser-final-evidence make test-browser` 在冻结后实际 exit 0，Go 包124.892s（命令端到端128.265s）；旧场景及新增 `release-batches.cjs` 全部通过，新增场景46.38s。真实 MySQL/Admin/Vite/Chrome，无依赖 skip。混合单三项从UI组织、独立审批、提交成功后故障注入丢响应、刷新原键恢复；另一大单由UI提交、独立审批和发布，返回1,000个唯一实际ID（5～1004）且与真实查询1,003总行逐项一致。明细与结果均能定位第1,000项，浏览器错误为零。

390px测试最初捕获动作行把文档撑至478px，修复按钮换行后，明细/结果 document=viewport=390，越界元素为空。注册表单就绪同步和测试账号总量夹具也经历真实失败后修复，未放宽业务断言或改生产限流配置。

已保存并目视的图：

- `2026-09-08-batch-draft-desktop.png`：混合草稿。
- `2026-09-08-batch-selected-delete.png`：明确选择的两行和原草稿去向。
- `2026-09-08-batch-item-1000-mobile.png`：滚动到末项的390px明细视口。
- `2026-09-08-batch-result-1000-desktop.png`、`2026-09-08-batch-result-1000-mobile.png`：滚动到末项的实际结果视口。

## 最终验证与评审

固定候选：2026-09-08 05:35:00+08，129个Admin/deploy mysql/正式Makefile/CI文件，hash `833c41f54b77807616ffb16d0d3c21d298745455bab35ba4c3a8c7b02af2b853`；清单 `/private/tmp/t6-backend-frozen.json`。

| 检查 | 实际结果 |
| --- | --- |
| `make test` | exit 0，全部Go模块，含架构、公共路由、服务身份和草稿不可写业务行检查 |
| `make build` | exit 0，全部模块和account-maintain |
| Web `test:run` | exit 0，23文件224项 |
| Web `typecheck` / `build` | 各 exit 0 |
| `make test-browser` | exit 0，124.892s；最终真实MySQL/Admin/Vite/Chrome全部场景通过 |
| `make test-integration` | exit 0，Admin1552.791s、HTTP96.224s，其余包全部通过；命令总1554.243s |
| Standards | sol/high独立只读；原3硬性+1主观项均解决，最终0 |
| Spec | astra/high独立只读；原2项均解决，最终0 |

两轴以固定基点检查完整工作变更及后续修复；报告 `/private/tmp/t6-standards-rereview.md`、`/private/tmp/t6-spec-final-review.md`。最终日志 `/private/tmp/t6-*-final.log`，各命令真实退出写入对应 `*-final-result.json`；依赖缺失不能skip当通过。正式完整MySQL入口已实际收取session84625的exit0；回归后逐文件核对129项与冻结字节完全一致。最终仅填写验收文档和复制正式浏览器截图，后端未改。
