# 发布单提交、审批与在途目标

编辑者保存同一数据源多表合计 0～1,000 项变更为草稿；提交至少 1 项；常规由每张表的合格独立人员审批，全部表通过后由发布者手动执行。应急提交填写原因后进入真实待发布，由当前发布者手动执行。保存、提交和审批都不修改业务配置，只有执行成功后数据才生效。普通发布成功保持待完结并保护目标；未完结期间可在原单预览后一次确认回滚；完结后不可回滚。旧记录直写路由已删除，规则目录仍由 ADMIN 直接管理。

草稿组织和容量限制见[发布草稿](admin-release-drafts.md)，最终结果及数据库权限见[发布结果契约](design-notes/publication-contract.md)，原单恢复规则见[原单回滚](admin-release-rollbacks.md)。提交与审批的交付证据见 [T4 #51](https://github.com/asherzj/relational-config-center/issues/51)。

## 流程实例与审批分配

常规提交要求每张参与表已有完整的已保存流程。提交保留节点实例和来源版本；它仍冻结业务申请、执行语义及各表审批角色身份，角色成员与账号资格继续实时检查。节点保存时点与审批分配冻结时点不同，草稿上的审批安排仍是提交前安排。模板节点中的 `TABLE_APPROVER` 不提供旧全局 APPROVER 旁路，全部表获得合法批准后才能整单发布。

| 拒绝条件 | 公开错误 | 后续操作 |
| --- | --- | --- |
| 非空草稿的逐表流程不完整 | `409 release_flow_incomplete` | 查看主单 `missing_flow_tables`；修复常规配置后显式保存草稿，再提交 |
| 流程完整但缺少独立审批人 | `422 release_approver_unavailable` | 查看问题表及当前审批安排，补充合格人员后提交 |
| 流程配置或审批目录无法可靠读取 | `503 release_unavailable` | 保留内容与原请求，恢复依赖后按原操作处理；不改用默认流程或默认授权 |

缺少流程时，`allowed_actions` 不提供 `submit`，直接提交也由服务器拒绝；空草稿继续返回既有明细数量错误。上述失败不会把草稿推进到待审批，也不会修改业务配置。常规发布接口同时要求完整流程与真实逐表批准事实，沿用现有发布权限、数据基线、规则及结构校验。

逐表节点的状态随真实业务写入持久保存，详情直接展示这些事实：`PENDING` 为待开始，`ACTIVE` 为进行中，`COMPLETED` 为已完成，`REJECTED` 为实际拒绝，`STOPPED` 为整单终止后未完成的节点。某表批准后只完成该表审批节点，其他表可继续待审；整单发布成功完成各表发布节点并等待人工完结，人工完结后完成完结节点。取消、拒绝或快速回滚保留已经完成的节点，未完成节点终止；没有实际操作的节点不填造人员或时间。

应急提交同样要求各表已有完整保存实例，原因 `emergency_reason` 必须为 1～2,000 个 Unicode 字符且不全为空白，错误为 `422 release_emergency_reason`。成功将原文和 SUBMIT 历史与冻结内容保存，进入 `PENDING_PUBLICATION`；不产生审批责任、批准人或批准事件，也不写业务数据。`approvals`、`approval_context.tables` 和 `approvable_tables` 均为空数组。应急发布节点为 ACTIVE、无完成人员，直到真实执行成功。当前 PUBLISHER/ADMIN 可执行，包括具备该权限的申请人；保存的节点不替代当前角色检查，撤权后的原请求重放仍会拒绝。

## HTTP 操作

所有请求沿用当前登录会话、同源与 CSRF 检查。写操作要求 `Idempotency-Key`，版本为十进制 JSON 字符串。

| 操作 | 请求体 | 当前授权与状态 |
|---|---|---|
| `POST /api/v1/release-orders/:id/submit` | `expected_version`，应急另需 `emergency_reason` | 本单申请人且当前 EDITOR/ADMIN，DRAFT；各表已保存所选方式流程完整 |
| `POST /api/v1/release-orders/:id/approve` | `expected_version`, 必填 `reason`, `confirmed_tables`, `expected_approval_revision` | 当前仍有待审批表资格的非申请人，PENDING_APPROVAL |
| `POST /api/v1/release-orders/:id/reject` | 同批准 | 同批准；释放目标 |
| `POST /api/v1/release-orders/:id/cancel` | `expected_version`, 必填 `reason` | 当前仍有编辑权限的申请人或 ADMIN，DRAFT/PENDING_APPROVAL/APPROVED/PENDING_PUBLICATION；释放目标 |
| `POST /api/v1/release-orders/:id/copy` | `expected_version`, `confirmed: true`, `items` | 当前 EDITOR/ADMIN，普通源单 REJECTED/CANCELLED；返回新 DRAFT（201） |
| `POST /api/v1/release-orders/:id/reprepare` | `expected_version`, `confirmed: true`, `items` | 原申请人且当前 EDITOR，或当前 ADMIN；正向源单 APPROVED/PENDING_PUBLICATION；返回新 DRAFT（201） |
| `POST /api/v1/release-orders/:id/execute` | `expected_version` | 当前 PUBLISHER/ADMIN，常规 APPROVED 或应急 PENDING_PUBLICATION；整单成功进入 SUCCEEDED 待完结并保留真实目标 |
| `POST /api/v1/release-orders/:id/complete` | `expected_version`，无必填意见 | 当前 PUBLISHER/ADMIN，SUCCEEDED 普通单；COMPLETED 并释放目标，配置和原发布结果不变 |
| `POST /api/v1/release-orders/:id/quick-rollback` | `expected_version`, `preview_digest`，可选 `reason` | 当前 PUBLISHER/ADMIN，SUCCEEDED；预览后一次确认，原单进入 ROLLED_BACK 并释放目标 |
| `POST /api/v1/release-orders/:id/rollback-reason` | `reason`，可为空 | ROLLED_BACK；本次实际回滚人或当前 ADMIN，可事后补填/修改并留痕 |

任何人均不能审批自己的单据。每张表由任一合格成员批准即可，一次行动必须覆盖操作者当前全部有权且待审批表；整单全部表通过才获批。已通过表不能再次批准，也不能凭该表资格拒绝整单。合法拒绝终止整单并保留此前逐表决定。申请人可持有 PUBLISHER 并执行他人已批准的本人单据；审批人也可兼任发布人。历史合法审批不会因审批人后来停用或撤权而被改写；之后的新请求和成功重试仍按当前身份校验。批准继续持有目标，没有到期自动释放或强制覆盖。

状态动作在锁定订单后检查调用者看到的版本，旧版本为 `409 release_version_conflict`，当前版本上的非法动作是 `422 release_state_invalid`。目标冲突是 `409 release_target_conflict`。同一账号/动作/键的成功请求先找回原业务结果，再对新动作做状态 CAS；同键不同请求摘要为 `409 idempotency_conflict`。成功重放可以返回旧状态的原业务结果，当前 `allowed_actions` 和 `approval_context` 来自最新单据；Web 重新查询当前详情，不将重放结果作为当前详情缓存。

复制前用已有只读 `/release-orders/preview` 读取原申请与当前记录的差异。复制 `items` 的操作、id 和申请内容必须与原单一致，但每个已知目标要明确携带刚核对的 `expected_record_version`。记录在预览与复制之间变化就拒绝。复制保持源单终态和原内容，在源单追加指向新草稿的 `COPY` 关联历史；新草稿保存永久申请人、自身 `COPY` 历史和指向源单的 `copied_from_id`，须重新核对并按所选方式提交：常规重新审批，应急填写新原因。不再创建反向草稿。

重新准备同样先用 `/release-orders/preview` 核对最新配置，并逐项提交原操作、原 id、原申请内容和新读取的 `expected_record_version`；`confirmed` 必须为 `true`。确认成功后，源单改为 CANCELLED 并释放其全部在途目标，同时创建继承原标题且可编辑的新 DRAFT，并为新草稿重新取得完整目标。新单申请人是实际操作者，源单与新单各保存一条 `REPREPARE` 关联历史，`copied_from_id` 指向源单；新单保留发布方式，按当前关联重新实例化；常规重新提交并由另一人审批，应急重新填写原因提交，旧批准与旧应急原因不复用。上述源单取消、目标释放、新单与双方历史、成功幂等结果在一个 MySQL 事务内提交；任何明确失败都保留源单原状态、审批事实和目标占用。

## 冻结与真实数据库身份

提交在同一 MySQL 事务内先对实际业务表执行 `SELECT id ... LIMIT 0`，保持元数据锁，然后重新解析规则、校验内容并读取记录基线。即使新增省略自增 id，也先稳定表定义再准备内容。提交不写业务记录、不分配 Record Version。

`RecordBaseline.TableName` 和 `ReleaseItem.RecordTable` 保留 T2/T3 的真实物理表名，`RecordKey` 继续使用同一 MySQL 主键比较权重、墓碑和维护基线。申请参数中的表名及字符串 id 不作为独立身份。`rcc_release_targets` 的唯一键是实际表名与同一记录键；草稿保存按固定顺序取得全部主键及已配置管控键目标，提交继续核实本单所有权，与状态、冻结内容、历史和成功请求结果共同提交。任意失败整体回滚。未知自增 id 不建立推测目标。自增列显式输入 `0` 且当前 SQL mode 没有 `NO_AUTO_VALUE_ON_ZERO` 时，数据库仍会生成新身份；草稿/预览/提交返回 `422 release_auto_id_ambiguous`，调用者应省略 id。开启该模式后，0 是真实已知身份，按普通记录基线和目标唯一性校验。

持久 `frozen_tables` 按表保存 `mysql-8.4-execution-v1` 格式的完整固定投影和变更规则执行字段。`frozen_digest` 覆盖准备后的明细和执行快照。内部记录身份、原始执行元数据不会从详情、列表或 preview 输出；公开主单提供流程摘要；明细与实际结果按同一整单版本分页读取。

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

尚未接管且已有 007 本地账号结构的部署，在停写维护窗口完成适用历史迁移 008～017 及 Policy 收缩后，显式运行 `schema-migrate baseline` 登记固定 1～5，再 `schema-migrate up` 并确认 current。015 建立原单明细/执行，016 增加管控键和表引用，017 删除主单默认表；这些步骤不转换旧发布单业务数据。已经纳入 Goose 的库使用 `schema-migrate up` 至当前构建包含的最新版本；新库同样只使用嵌入式 `up` 初始化。历史接管边界保持 00005，常规与应急流程实例使用现有主单文档，不新增 Schema 迁移。未确认操作按手册显式 `recover`，不能混用历史人工链和 Goose。Ready 只读核对完整版本前缀及每张控制表的完整定义。具体命令和历史边界见[迁移说明](../deploy/mysql/migrations/README.md)及[接管手册](schema-migrations.md)。

Web 将草稿和发布动作（含提交、批准、拒绝、取消、复制、重新准备、执行、完结、快速回滚）的原请求内容、键及账号在发送前通过单一 IndexedDB 事务持久保存。事务完成前不发送，同账号窗口同步存储状态而不自动重推；存储失败保留输入并只阻止发布写入。未知结果保持原键和完整正文；重新准备未知时同一账号恢复同一请求，只在服务端返回明确失败后才重新核对并生成新键。仅持有 VIEWER 全局权限的表角色成员也能恢复自己的审批请求，账号切换不会重放别人的请求。明确状态/记录/目标冲突后仍保留原意，读取最新状态与差异、显式确认后才生成新请求。普通草稿提交基线陈旧时须先明确更新草稿基线；回滚恢复基线不能更新。

详情区分草稿、待审批、已批准、应急待发布、已发布待完结、已完结、已拒绝、已取消和已回滚，显示当前允许动作、冻结差异、永久身份、历史意见及关联单。执行结果保存实际发布人、最终行和新版本；普通执行成功时单据为 SUCCEEDED 并保留全部目标占用，人工完结变为 COMPLETED 后释放占用。回滚成功原单标记 ROLLED_BACK，保存第二次成功执行并释放占用。

执行前内容、规则、表结构或记录版本冲突会拒绝整单，单据保持常规 APPROVED 或应急 PENDING_PUBLICATION 并保留目标。业务数据、记录/表版本、Command、单据状态及历史、普通发布的实际自增目标占用或反向发布的目标释放、通知和成功请求结果在同一事务提交；回滚执行与原单状态、逐项恢复结果及成功执行摘要共同提交。执行结果未知时，刷新或同账号恢复登录后继续使用原键和原内容，不能根据一次查询未找到或 401/403 换键重做。只有确认成功才显示已发布；通知状态为 NOT_CONNECTED，表示分发尚未接入。

## 详情审阅与原操作重推

正向常规与应急发布单详情分别展示整单阶段和已保存逐表流程。准备完成取真实 SUBMIT；各表节点使用自身实例的名称、状态、人员和时间，来源模板及版本可展开核对。部分表批准时整单仍待审批，普通发布成功仍为 SUCCEEDED 待人工完结。原单恢复预览显式保存各表应急实例，回滚结果展示真实恢复发布节点与执行人；成功只终止尚未完成的正向和恢复完结节点，不补造人工完结。所有终态共用整单阶段与真实逐表实例展示，不使用固定四阶段总览或状态序数推断完成。历史默认倒序最近 5 条，可展开全部，姓名解析失败保持永久身份兜底。

申请差异每页 20 项，初次仅展开首项，定位展开选中项。默认仅看变更过滤 MODIFY 中未提交（申请未写入）及状态和值均确定相同的字段，自动填写和数据库生成说明仍可见；完整视图分别表达 SQL NULL、JSON null、空字符串、未提交、不存在和发布时生成。申请筛选不承诺触发器执行后的实际值，最终结果仍取数据库发布结果。ADD/DELETE 保留完整字段。所有操作仍针对整单。成功初次默认可信实际发布结果，可切换申请差异；已回滚原单可分页读取原明细 `rollback` 的真实恢复结果，不从申请内容生成实际值。

当前标签页只对真实 processing 请求保护同单操作；报错后的未知请求不构成额外确认门禁。用户从原操作再次提交，原账号、键与完整正文保持不变，同键处理中禁止重复发送。刷新后普通主单查询仍显示当前状态；旧请求可以重放原业务结果，不能将当前主单倒退。首次认证中断和后续权限错误保留原键及正文，同账号恢复后继续原请求，其他账号不显示或重放这份申请。明确版本、状态、目标或恢复冲突可进入最新审阅流程，经显式确认才重建新键；浏览器守卫不替代服务端授权、版本检查和幂等事务。


## 执行失败与失败历史（#86）

发布与快速回滚仅在事务已返回且没有发起 COMMIT、确认未提交时记录失败历史；MySQL COMMIT 响应丢失只返回 `release_result_unknown`，不记录确定失败。原错误码、全局 `item_index` 和 Request ID 保留。确认未提交的执行错误附加 `error.execution_outcome: "not_committed"`，以及 `error.failure_history: "saved" | "unavailable"`；后者为 unavailable 时不能确认历史已落库，Web 如实显示这一点。

失败业务事务全部回退后，在一个独立且有期限的控制事务中先绑定原 actor/operation/key/digest，再锁当前主单并仅追加 `EXECUTE_FAILED` 或 `QUICK_ROLLBACK_FAILED` 事件。事件记录真实执行账号、数据库时间和尝试的主单版本。不改 state、version、updated_at、明细、成功执行、目标、业务/表版本、命令或通知；并发合法状态变化不会被旧快照覆盖。失败审计不能推进 CAS，使未改变条件下的原 expected_version 能安全重推。绑定成功的失败请求在成功之前也拒绝同键异内容；审计存储不可用时不伪造该绑定或历史。

不存在自动写重试与独立结果确认流程。用户再次点击原业务按钮时，相同请求已提交则返回原结果、尚未提交则按当前权限及原 CAS 执行。正常读取当前主单是页面状态的依据。HTTP 成功后的 IndexedDB 清理失败仍触发真实主单读取，不回报确定业务失败，也不自动发送替代请求。


## 冻结责任与当前确认范围

主单详情、列表和写响应提供 `approvals` 与 `approval_context`。前者包括每张表的提交角色身份／名称、状态和真实决定；后者包括当前资格修订值 `revision`、各表的 `ROLE`／`ADMIN`／`UNAVAILABLE`／`COMPLETED` 说明、`can_approve` 和完整 `approvable_tables`。草稿展示当前提交安排；已提交单据保持冻结责任。普通详情继续读取工作流摘要，申请和执行结果仍通过同版本明细分页读取。

批准和拒绝提交打开确认时的发布单版本、资格修订值和完整表集合。主单进度变动返回 `409 release_version_conflict`；角色、成员、账号状态、ADMIN 或确认范围变动返回 `409 release_approval_conflict`。意见和原请求保留，用户读取最新范围后明确确认才创建新请求。成功请求重放对原表范围重新检查当前资格，但不会重新决定已通过表。

提交时缺少独立审批人返回 `422 release_approver_unavailable` 并指出问题表，草稿和目标保留。已提交单据后来无人可审时保持当前状态和已通过进度，详情提示补充人员。角色目录读取失败返回不可用，绝不推导默认 ADMIN 授权。

角色维护、表分配、账号启停／授权和发布单写操作共用授权控制锁。创建、复制与重新准备也在取得该锁后重读当前操作者并复核编辑权限，成功原请求重推同样不能绕过当前授权。发布单按授权锁 → 原请求身份 → 主单 → 明细取得锁，并在锁内重新读取操作者与审批资格；操作者读取使用当前锁定读，避免提前建立业务规则的旧快照。当前实现串行化这些写事务，延续单部署的取舍；高频会话访问也使用该锁，未来调整锁粒度须保持资格竞争和业务冻结保护。逐表决定、真实历史、状态和幂等结果一起提交；审批分配不进入冻结的业务执行规则摘要。

表审批历史记录操作者、数据库时间、意见、表范围、`ROLE` 或 `ADMIN` 来源和当时角色名称。已完成审批不再次按实时成员资格重算；常规发布仍要求所有实际涉及表都有批准事实，并继续校验原有数据、业务规则和结构。

`release_approver_unavailable` 的 422 在原请求去重后返回，表示该提交没有成功，草稿和版本保留。即使此前结果未知，同键重试获得这个明确拒绝也会结束该原提交的恢复状态；页面保留当前审批安排并显示问题表，补充合格人员后可重新提交。审批确认窗口中的原表范围和意见仍只在显式审阅后更新。
