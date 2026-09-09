# 发布单提交、审批与在途目标

编辑者保存同表 1～1,000 项变更为草稿，提交后由另一人审批，再由发布者手动执行。保存、提交和审批都不修改业务配置，只有执行成功后数据才生效。普通发布成功保持待完结并保护目标；人工完结后通过新的反向单重新审批回滚。旧记录直写路由已删除，规则目录仍由 ADMIN 直接管理。

草稿组织和容量限制见[发布草稿](admin-release-drafts.md)，最终结果及数据库权限见[发布结果契约](design-notes/publication-contract.md)，反向单规则见[审批回滚](admin-release-rollbacks.md)。提交与审批的交付证据见 [T4 #51](https://github.com/asherzj/relational-config-center/issues/51)。

## HTTP 操作

所有请求沿用当前登录会话、同源与 CSRF 检查。写操作要求 `Idempotency-Key`，版本为十进制 JSON 字符串。

| 操作 | 请求体 | 当前授权与状态 |
|---|---|---|
| `POST /api/v1/release-orders/:id/submit` | `expected_version` | 本单申请人且当前 EDITOR/ADMIN，DRAFT |
| `POST /api/v1/release-orders/:id/approve` | `expected_version`, 必填 `reason` | 另一位 APPROVER/ADMIN，PENDING_APPROVAL |
| `POST /api/v1/release-orders/:id/reject` | 同批准 | 同批准；释放目标 |
| `POST /api/v1/release-orders/:id/cancel` | `expected_version`, 必填 `reason` | 当前仍有编辑权限的申请人或 ADMIN，DRAFT/PENDING_APPROVAL/APPROVED；释放目标 |
| `POST /api/v1/release-orders/:id/copy` | `expected_version`, `confirmed: true`, `items` | 当前 EDITOR/ADMIN，普通源单 REJECTED/CANCELLED；返回新 DRAFT（201） |
| `POST /api/v1/release-orders/:id/reprepare` | `expected_version`, `confirmed: true`, `items` | 原申请人且当前 EDITOR，或当前 ADMIN；普通源单 APPROVED；返回新 DRAFT（201） |
| `POST /api/v1/release-orders/:id/execute` | `expected_version` | 当前 PUBLISHER/ADMIN，APPROVED；普通整单成功进入 SUCCEEDED 待完结并保留真实目标；反向成功直接 COMPLETED 并释放 |
| `POST /api/v1/release-orders/:id/complete` | `expected_version`，无必填意见 | 当前 PUBLISHER/ADMIN，SUCCEEDED 普通单；COMPLETED 并释放目标，配置和原发布结果不变 |
| `POST /api/v1/release-orders/:id/rollback` | `expected_version`, 必填 `reason` | 当前 EDITOR/ADMIN，原单 COMPLETED、非反向结果且没有在途反向申请；返回反向 DRAFT（201） |

ADMIN 也不能审批自己的单据。一位独立审批人决定一次即足够。申请人可持有 PUBLISHER 并执行他人已批准的本人单据；审批人也可兼任发布人。历史合法审批不会因审批人后来停用或撤权而被改写；之后的新请求和成功重试仍按当前身份校验。批准继续持有目标，没有到期自动释放或强制覆盖。

状态动作在锁定订单后检查调用者看到的版本，旧版本为 `409 release_version_conflict`，当前版本上的非法动作是 `422 release_state_invalid`。目标冲突是 `409 release_target_conflict`。同一账号/动作/键的成功请求先找回原业务结果，再对新动作做状态 CAS；同键不同请求摘要为 `409 idempotency_conflict`。成功重放可以返回旧状态的原业务结果，当前 `allowed_actions` 来自最新单据；Web 重新查询当前详情，不将重放结果作为当前详情缓存。

复制前用已有只读 `/release-orders/preview` 读取原申请与当前记录的差异。复制 `items` 的操作、id 和申请内容必须与原单一致，但每个已知目标要明确携带刚核对的 `expected_record_version`。记录在预览与复制之间变化就拒绝。复制不修改源单，创建新的永久申请人和 `COPY` 历史，通过 `copied_from_id` 关联原单；新单须重新核对、提交和审批。反向草稿不能编辑、复制或追加明细；取消/拒绝反向单后，应从原成功单重新申请回滚，仍使用原发布后的记录版本。

重新准备同样先用 `/release-orders/preview` 核对最新配置，并逐项提交原操作、原 id、原申请内容和新读取的 `expected_record_version`；`confirmed` 必须为 `true`。确认成功后，源单改为 CANCELLED 并释放其全部在途目标，同时创建继承原标题且可编辑的新 DRAFT。新单申请人是实际操作者，源单与新单各保存一条 `REPREPARE` 关联历史，`copied_from_id` 指向源单；新单须重新提交并由另一人审批，旧批准不能复用。上述源单取消、目标释放、新单与双方历史、成功幂等结果在一个 MySQL 事务内提交；任何明确失败都保留原 APPROVED、旧审批和目标占用。反向单不支持重新准备。

## 冻结与真实数据库身份

提交在同一 MySQL 事务内先对实际业务表执行 `SELECT id ... LIMIT 0`，保持元数据锁，然后重新解析规则、校验内容并读取记录基线。即使新增省略自增 id，也先稳定表定义再准备内容。提交不写业务记录、不分配 Record Version。

`RecordBaseline.TableName` 和 `ReleaseItem.RecordTable` 保留 T2/T3 的真实物理表名，`RecordKey` 继续使用同一 MySQL 主键比较权重、墓碑和维护基线。申请参数中的表名及字符串 id 不作为独立身份。`rcc_release_targets` 的唯一键是实际表名与同一记录键；提交按固定顺序取得全部已知目标，与状态、冻结内容、历史和成功请求结果共同提交。任意失败整体回滚。未知自增 id 不建立推测目标。自增列显式输入 `0` 且当前 SQL mode 没有 `NO_AUTO_VALUE_ON_ZERO` 时，数据库仍会生成新身份；草稿/预览/提交返回 `422 release_auto_id_ambiguous`，调用者应省略 id。开启该模式后，0 是真实已知身份，按普通记录基线和目标唯一性校验。

持久 `frozen` 是 `mysql-8.4-execution-v1` 格式的完整固定投影和变更规则执行字段。`frozen_digest` 覆盖准备后的明细和执行快照。内部记录身份、原始执行元数据不会从详情、列表或 preview 输出；公开详情保留完整字段差异及摘要。

固定投影包括：

- 列顺序、精确类型、NULL/default、字符集/排序规则、EXTRA、生成表达式及 SRS。
- 表引擎/排序规则/行格式/选项，索引定义、CHECK、可见的双向外键、分区定义。
- 触发器语句、顺序、执行时点、保存的 SQL mode、definer 及字符集上下文。
- MySQL 版本、当前 SQL mode/时区/连接字符集/排序规则、约束开关、时间名称、自增步长/偏移、时间戳默认设置和除法精度。
- Mutation Policy 的类型、允许操作和四个服务器自动字段。规则名称、描述、操作历史与更新时间不进入执行语义。

表统计、索引 cardinality、当前 AUTO_INCREMENT 游标、注释不纳入摘要。MySQL 的 ALTER 可能在重新序列化已有生成表达式/CHECK 时改变字面量字符集，即使请求只写 COMMENT；这种实际表达式变化仍保守视为语义变化，不通过删掉字符集标记来伪装等价。普通字段表纯 COMMENT、规则名称/描述、普通无关记录写入和 ANALYZE 有独立稳定正例。

触发器元数据可能被权限隐藏。提交要求可以证明的直接 TRIGGER 授权：全局、目标 Schema 或目标表；`partial_revokes=ON` 时不使用全局授权作为充分证明。Schema 授权模式匹配固定反斜线 escape，不受 NO_BACKSLASH_ESCAPES 影响。无法证明时返回 `422 release_metadata_permission`。仅通过数据库角色继承的授权目前不作为证明，部署可明确授予目标 Schema/表 TRIGGER；不要求读取 mysql 系统表。

授权元数据的普通字符串比较不能代替数据库身份：Schema/表名按 `lower_case_table_names` 明确比较，账号按 MySQL 返回的完整授权身份精确匹配。大小写不同对象或账号的授权不能证明当前账号能看见目标触发器；用户名中的 `@`、引号和反斜线按元数据原始格式保留。真实 fixture 分别验证这些负例、目标直接授权后的原键恢复，以及大小写不敏感实例的合法授权。

执行时重新比较上述快照，并在业务写入前检查能否完整追踪附带变化，包括隐藏的跨 Schema 外键、触发器副作用和完整最终行编码。仅能读取触发器定义不足以允许执行；部署账号还需显式全局 PROCESS 权限以检查完整 InnoDB 外键字典。允许的目标行 BEFORE 触发器及拒绝范围见[发布能力边界](design-notes/publication-contract.md#写入能力边界)。元数据锁保持至事务结束，无法证明安全的变更在业务写入前拒绝。

## 升级与恢复

已有 007 本地账号结构的部署在停写维护窗口按顺序应用 008～012。`011-release-targets.sql` 建立在途目标，`012-publication.sql` 建立发布进度、Command 和通知；不能只完成审批结构就恢复新版服务。新安装的 `001-schema.sql` 包含完整定义。008～012 可重跑且保留既有控制数据，007 一次性迁移按[迁移说明](../deploy/mysql/migrations/README.md)确认后处理。Ready 校验控制表列、InnoDB 和完整唯一主键，缺失或不兼容时拒绝就绪。

Web 将草稿和发布动作（含提交、批准、拒绝、取消、复制、重新准备、执行、完结、快速回滚及回滚申请）的原请求内容、键及账号在发送前保存到当前标签页的 sessionStorage。未知结果保持原键和完整正文；重新准备未知时同一账号恢复同一请求，只在服务端返回明确失败后才重新核对并生成新键。只有 APPROVER 的账号也能恢复自己的审批请求，账号切换不会重放别人的请求。明确状态/记录/目标冲突后仍保留原意，读取最新状态与差异、显式确认后才生成新请求。普通草稿提交基线陈旧时须先明确更新草稿基线；反向单的原发布后版本不能更新。

详情区分草稿、待审批、已批准、已发布待完结、已完结、已拒绝、已取消和已回滚，显示当前允许动作、冻结差异、永久身份、历史意见及关联单。执行结果保存实际发布人、最终行和新版本；普通执行成功时单据为 SUCCEEDED 并保留全部目标占用，人工完结变为 COMPLETED 后释放占用。反向执行成功时原单标记 ROLLED_BACK，反向单直接为 COMPLETED 并释放占用。

执行前内容、规则、表结构或记录版本冲突会拒绝整单，单据保持 APPROVED 并保留目标。业务数据、记录/表版本、Command、单据状态及历史、普通发布的实际自增目标占用或反向发布的目标释放、通知和成功请求结果在同一事务提交；反向执行还同时更新原单的回滚关联。执行结果未知时，刷新或同账号恢复登录后继续使用原键和原内容，不能根据一次查询未找到或 401/403 换键重做。只有确认成功才显示已发布；通知状态为 NOT_CONNECTED，表示分发尚未接入。

## 详情审阅与同单请求恢复

发布单详情按准备、审批、发布、完结组织进度，节点人员与时间来自真实事件；准备完成取 SUBMIT。取消、拒绝和回滚明确显示终止，快速回滚结果不产生虚构审批。普通成功仍为 SUCCEEDED 待人工完结。历史默认倒序最近 5 条，可展开全部，姓名解析失败保持永久身份兜底。

申请差异每页 20 项，初次仅展开首项，定位展开选中项。默认仅看变更过滤 MODIFY 中未提交（申请未写入）及状态和值均确定相同的字段，自动填写和数据库生成说明仍可见；完整视图分别表达 SQL NULL、JSON null、空字符串、未提交、不存在和发布时生成。申请筛选不承诺触发器执行后的实际值，最终结果仍取数据库发布结果。ADD/DELETE 保留完整字段。所有操作仍针对整单。成功初次默认可信实际发布结果，可切换申请差异；已回滚普通单可读取关联反向发布单的真实恢复结果，不从申请内容生成实际值。

当前标签页对同一账号、同一单的 processing/unknown 请求统一互斥，覆盖编辑和全部状态动作；恢复同一键时也禁止并行重复发送。首次认证中断和后续权限错误保留原键及正文，同账号恢复后继续原请求，其他账号不显示或重放这份申请。明确版本、状态、目标或恢复冲突可进入最新审阅流程，经显式确认才重建新键；浏览器守卫不替代服务端授权、版本检查和幂等事务。
