# 审批角色管理

管理员在“平台人员管理 → 角色管理”创建审批角色、维护名称和说明、选择本地账号、启停角色或删除从未引用的角色。角色的永久 UUID 不随改名改变；一个账号可以属于多个角色，角色可以有多个成员。选择器显示用户名、显示名称、账号 ID 和启用状态，支持检索与分页；停用账号可以保留在成员关系中。

本页交付 [#93](https://github.com/asherzj/relational-config-center/issues/93) 的角色管理切片。角色成员关系不授予全局查看以外的编辑、发布或管理员权限。表绑定、按表审批与引用生命周期的完整接线由 #94 交付；旧全局 APPROVER 在本切片继续按原行为运行，最终退出由 #98 负责。

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

`rcc_approval_role_references` 保存永久引用事实：角色 ID、`table` 或 `snapshot` 来源、来源标识、引用时角色名称和时间。外键阻止删除已有引用的角色，管理删除同时检查历史引用，成员关联仅在合法删除角色时级联删除。#94 的表绑定/快照事务负责写入这些真实事实，并与角色修改使用相同授权锁；解绑不得删除引用事实。此表没有测试专用或用户直写接口，本切片不声称真实表/审批引用已接通。

新库直接执行 `schema-migrate up`。未接管旧库仍按历史步骤达到冻结的 Goose 00005 基线，`baseline` 只登记 1～5，在本构建返回 `pending`；随后显式 `up` 执行 00006。已接管版本 5 同样显式 `up`。已发布 00001～00005 的 SQL/manifest 均不变，迁移不授予角色、不改变旧账号权限、不写业务数据。部分 DDL 或确认失败遵循[迁移维护手册](schema-migrations.md)的显式恢复流程。
