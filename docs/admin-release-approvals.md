# 发布单提交、审批与在途目标

T4 / #51 在 T3 持久草稿上增加提交、审批、拒绝、取消和复制。实际发布由 T5 / #52 交付；本阶段没有业务执行处理器。单明细限制由 T6 / #53 删除，旧记录写入口仍由 T5 统一删除。

## HTTP 操作

所有请求沿用当前登录会话、同源与 CSRF 检查。写操作要求 `Idempotency-Key`，版本为十进制 JSON 字符串。

| 操作 | 请求体 | 当前授权与状态 |
|---|---|---|
| `POST /api/v1/release-orders/:id/submit` | `expected_version` | 本单申请人且当前 EDITOR/ADMIN，DRAFT |
| `POST /api/v1/release-orders/:id/approve` | `expected_version`, 必填 `reason` | 另一位 APPROVER/ADMIN，PENDING_APPROVAL |
| `POST /api/v1/release-orders/:id/reject` | 同批准 | 同批准；释放目标 |
| `POST /api/v1/release-orders/:id/cancel` | `expected_version`, 必填 `reason` | 当前仍有编辑权限的申请人或 ADMIN，DRAFT/PENDING_APPROVAL/APPROVED；释放目标 |
| `POST /api/v1/release-orders/:id/copy` | `expected_version`, `confirmed: true`, `items` | 当前 EDITOR/ADMIN，源单 REJECTED/CANCELLED；返回新 DRAFT（201） |

ADMIN 也不能审批自己的单据。一位独立审批人决定一次即足够。历史合法审批不会因审批人后来停用或撤权而被改写；之后的新请求和成功重试仍按当前身份校验。批准继续持有目标，没有到期自动释放或强制覆盖。

状态动作在锁定订单后检查调用者看到的版本，旧版本为 `409 release_version_conflict`，当前版本上的非法动作是 `422 release_state_invalid`。目标冲突是 `409 release_target_conflict`。同一账号/动作/键的成功请求先找回原业务结果，再对新动作做状态 CAS；同键不同请求摘要为 `409 idempotency_conflict`。成功重放可以返回旧状态的原业务结果，当前 `allowed_actions` 来自最新单据；Web 重新查询当前详情，不将重放结果作为当前详情缓存。

复制前用已有只读 `/release-orders/preview` 读取原申请与当前记录的差异。复制 `items` 的操作、id 和申请内容必须与原单一致，但每个已知目标要明确携带刚核对的 `expected_record_version`。记录在预览与复制之间变化就拒绝。复制不修改源单，创建新的永久申请人和 `COPY` 历史，通过 `copied_from_id` 关联原单；新单须重新编辑/提交/审批。

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

该快照为 T5 的执行重验提供依据，并不表示所有可见触发器、外键或表达式已获执行许可。T5 还必须在业务写入前独立检查能否完整追踪附带变化，包括不可见/跨 Schema 外键、触发器调用链和完整最终行编码；无法证明时拒绝，不能把本单采集到的可见元数据当作不存在隐藏副作用的证明。元数据锁应延续到真实写入提交。

## 升级与恢复

在 007/008/009/010 后执行 `deploy/mysql/migrations/011-release-targets.sql`，重复执行不改写已有目标。新安装的 001 包含同一定义。Ready 校验目标表列、InnoDB 和完整唯一主键；显式存量升级 fixture 同步执行 011。

Web 将提交、批准、拒绝、取消和复制的原请求内容、键及账号在发送前持久保存。未知结果保持原键；只有 APPROVER 的账号也能恢复自己的审批请求，账号切换不会重放别人的请求。明确状态/记录/目标冲突后仍保留原意，读取最新状态与差异、显式确认后才生成新请求。提交基线陈旧时须先更新草稿基线，再提交。

详情分别显示草稿、待审批、已批准、已拒绝和已取消，提供当前允许动作、冻结差异、永久身份和历史意见。没有“已发布”桩状态或自动发布按钮。
