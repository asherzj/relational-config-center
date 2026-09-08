# 发布单提交、审批与在途目标

编辑者保存同表 1～1,000 项变更为草稿，提交后由另一人审批，再由发布者手动执行。保存、提交和审批都不修改业务配置，只有执行成功后数据才生效。已发布内容通过新的反向单重新审批回滚。旧记录直写路由已删除，规则目录仍由 ADMIN 直接管理。

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
| `POST /api/v1/release-orders/:id/execute` | `expected_version` | 当前 PUBLISHER/ADMIN，APPROVED；整单成功后释放目标 |
| `POST /api/v1/release-orders/:id/rollback` | `expected_version`, 必填 `reason` | 当前 EDITOR/ADMIN，原单 SUCCEEDED 且没有在途反向申请；返回反向 DRAFT（201） |

ADMIN 也不能审批自己的单据。一位独立审批人决定一次即足够。申请人可持有 PUBLISHER 并执行他人已批准的本人单据；审批人也可兼任发布人。历史合法审批不会因审批人后来停用或撤权而被改写；之后的新请求和成功重试仍按当前身份校验。批准继续持有目标，没有到期自动释放或强制覆盖。

状态动作在锁定订单后检查调用者看到的版本，旧版本为 `409 release_version_conflict`，当前版本上的非法动作是 `422 release_state_invalid`。目标冲突是 `409 release_target_conflict`。同一账号/动作/键的成功请求先找回原业务结果，再对新动作做状态 CAS；同键不同请求摘要为 `409 idempotency_conflict`。成功重放可以返回旧状态的原业务结果，当前 `allowed_actions` 来自最新单据；Web 重新查询当前详情，不将重放结果作为当前详情缓存。

复制前用已有只读 `/release-orders/preview` 读取原申请与当前记录的差异。复制 `items` 的操作、id 和申请内容必须与原单一致，但每个已知目标要明确携带刚核对的 `expected_record_version`。记录在预览与复制之间变化就拒绝。复制不修改源单，创建新的永久申请人和 `COPY` 历史，通过 `copied_from_id` 关联原单；新单须重新核对、提交和审批。反向草稿不能编辑、复制或追加明细；取消/拒绝反向单后，应从原成功单重新申请回滚，仍使用原发布后的记录版本。

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

Web 将草稿和发布动作（含提交、批准、拒绝、取消、复制、执行及回滚申请）的原请求内容、键及账号在发送前保存到当前标签页的 sessionStorage。未知结果保持原键；只有 APPROVER 的账号也能恢复自己的审批请求，账号切换不会重放别人的请求。明确状态/记录/目标冲突后仍保留原意，读取最新状态与差异、显式确认后才生成新请求。普通草稿提交基线陈旧时须先明确更新草稿基线；反向单的原发布后版本不能更新。

详情区分草稿、待审批、已批准、已发布、已拒绝、已取消和已回滚，显示当前允许动作、冻结差异、永久身份、历史意见及关联单。执行结果保存实际发布人、最终行和新版本；反向执行成功时原单标记 ROLLED_BACK，反向单为 SUCCEEDED。

执行前内容、规则、表结构或记录版本冲突会拒绝整单，单据保持 APPROVED 并保留目标。业务数据、记录/表版本、Command、单据状态及历史、目标释放、通知和成功请求结果在同一事务提交；反向执行还同时更新原单的回滚关联。执行结果未知时，刷新或同账号恢复登录后继续使用原键和原内容，不能根据一次查询未找到或 401/403 换键重做。只有确认成功才显示已发布；通知状态为 NOT_CONNECTED，表示分发尚未接入。
