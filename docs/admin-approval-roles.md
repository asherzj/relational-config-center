# 审批角色管理

管理员在“平台人员管理 → 角色管理”创建审批角色、维护名称和说明、选择本地账号、启停角色或删除从未引用的角色。角色的永久 UUID 不随改名改变；一个账号可以属于多个角色，角色可以有多个成员。选择器显示用户名、显示名称、账号 ID 和启用状态，支持检索与分页；停用账号可以保留在成员关系中。

角色维护见 [#93](https://github.com/asherzj/relational-config-center/issues/93)，表分配和多表审批见 [#94](https://github.com/asherzj/relational-config-center/issues/94)。角色成员关系不额外授予编辑、发布或管理权限。实际审批仅来自对应表的冻结角色与当前成员，或独立 ADMIN 默认资格；旧全局 APPROVER 已退出当前授权，Goose 00009 完成账号存储收缩；它仅由不可变授权历史的独立解释器读取。

## HTTP 契约

全部接口要求已登录且当前具有 ADMIN 能力。写入继续要求同源请求、会话绑定的 `X-CSRF-Token` 和 `Idempotency-Key`；账号及角色授权从服务器身份取得，不接受正文指定操作者。

| 方法与路径 | 请求 | 返回 |
| --- | --- | --- |
| `GET /api/v1/approval-roles` | `q` 为名称或 ID；`after` 为上一页最后 UUID；`limit` 默认 25，范围 1～100 | `roles`、`next_cursor` |
| `GET /api/v1/approval-roles/:id` | 永久角色 ID | 当前角色与真实成员 |
| `POST /api/v1/approval-roles` | `name`、`description`、`enabled`、`member_ids` | 201，角色版本 1 |
| `PUT /api/v1/approval-roles/:id` | 同创建，另有字符串 `expected_version` | 200，完整替换内容和成员，版本加一 |
| `DELETE /api/v1/approval-roles/:id` | 字符串 `expected_version` | 200，保存的角色结果中 `deleted=true` |

人员检索复用 `GET /api/v1/account-roles?q=...&after=...`，接口保持原 ADMIN 授权。角色返回 `id`、`name`、`description`、`enabled`、字符串 `version`、`members`（每项含 `id/username/display_name/enabled`）、`referenced`、`deleted`、`creator/modifier` 与 UTC 微秒时间 `created_at/updated_at`。数字版本以字符串传输，避免 JavaScript 精度损失。

名称去除首尾空白后为 1～200 个 Unicode 字符，说明最多 2000 个字符；`enabled` 和 `member_ids` 必须显式提供。每次最多 1000 个成员输入，UUID 必须对应真实账号；重复成员按永久 ID 去重并排序。请求键为 8～64 个 ASCII 字符，首字符为字母或数字，后续允许字母、数字、点、下划线和连字符。名称、成员等经过上述规范化后的相同意图视为相同请求；同账号的同一请求键不能用于另一操作或内容。

主要稳定错误码：`invalid_approval_role`（422）、`account_not_found`（404）、`approval_role_not_found`（404）、`approval_role_conflict`（409）、`idempotency_conflict`（409）、`approval_role_referenced`（409）、`approval_role_not_saved`（503，确认本次事务未提交），以及已有认证、权限和不可用错误。响应保留 Request ID，数据库原始错误不返回客户端。确定回滚时保留可编辑输入；COMMIT 或响应无法确认时保留原请求键和内容，只允许原请求重试。若先前已有未知请求，本次未提交或认证失败不能证明先前结果，仍保留原请求。

## 事务、重推与输入保护

每次修改在既有 `rcc_auth_control_lock` 锁内再次核对操作者已启用且拥有 ADMIN。账号启停、全局授权和审批角色修改采用一致事务顺序，成员替换、角色版本与原请求结果一起提交。返回结果丢失时使用同账号、原请求键和原正文重推，获得第一次保存的结果；重推仍检查当前授权。失败保存请求记录会回滚整个修改。

Web 遇到版本冲突保留输入，先显示最新角色内容与版本，再由管理员重新保存。网络或服务异常后的未知结果锁定原请求，提供“使用原请求重试”，不会自动写入；未知操作未解决时角色的其他写入禁用。会话中断遮住受保护浮层，同账号登录恢复保留输入，切换账号清除之前的私有草稿。撤权后输入保留，保存禁用。

## 控制结构与后续引用职责

Goose `00006_approval_roles.sql` 新增角色、成员、请求结果和永久引用四张表；目标结构由同版完整 manifest 及 Admin 只读 readiness 校验。所有 `rcc_` 控制表继续被通用受管表发现、配置和变更接口保护。

`rcc_approval_role_references` 保存永久引用事实：角色 ID、`table` 或 `snapshot` 来源、来源标识、引用时角色名称和时间。外键阻止删除已有引用的角色，管理删除同时检查历史引用，成员关联仅在合法删除角色时级联删除。表绑定和提交快照事务写入这些真实事实，并与角色修改使用相同授权锁；解绑不得删除引用事实。此表没有测试专用或用户直写接口。

新库直接执行 `schema-migrate up`。未接管旧库仍按历史步骤达到冻结的 Goose 00005 基线，`baseline` 只登记 1～5，在本构建返回 `pending`；随后显式 `up` 执行 00006。已接管版本 5 同样显式 `up`。已发布 00001～00005 的 SQL/manifest 均不变，迁移不授予角色、不改变旧账号权限、不写业务数据。部分 DDL 或确认失败遵循[迁移维护手册](schema-migrations.md)的显式恢复流程。


## 表审批分配

管理员从「表规则 → 审批角色」维护一张表的完整角色集合。空集合明确使用独立 ADMIN 默认规则；角色停用时仍保留选择和永久身份。分配有自己的版本，保存不修改查询／变更策略、管控键或其他表配置。

| 契约 | 内容 |
| --- | --- |
| `GET /api/v1/table-policies/:table_name/approval-roles` | 返回 `table_name`、字符串 `version`、`role_ids` 和角色身份／名称 `roles`；首次未保存为版本 `0` 和空集合 |
| `PUT /api/v1/table-policies/:table_name/approval-roles` | 完整 `role_ids`、`expected_version` 与 `Idempotency-Key`；重复或不存在身份拒绝 |
| `409 table_approval_conflict` | 读取最新选择并保留本窗口输入，明确确认后用新版本保存 |
| `503 approval_role_not_saved` | 事务确认未提交，可修正后保存；无法确认提交结果时仍须原键原正文重推 |

提交时永久保存各表的角色身份和名称，包括空集合。原单保留快照角色的当前成员资格；之后改绑只影响新提交的单据。已有正常合格成员时 ADMIN 无通用审批权；无合格独立成员时，当前启用且非申请人的 ADMIN 默认接手。默认已完成的决定和普通角色决定均不因后来撤权或停用失效。

迁移 `00007_table_approvals.sql` 新增独立分配与请求结果表，逐表快照／决定属于主单工作流文档，不读取全部明细才能计算资格。未接管基线仍冻结为 5，已到 6 的库显式向前升级到 7；不改写历史发布单、不创建全表角色、不清理环境数据。
