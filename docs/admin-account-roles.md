# 全局账号角色

本指南说明当前管理台的全局角色、管理员初始化及权限恢复。配置记录的新增、修改和删除均通过[发布草稿](admin-release-drafts.md)、[独立审批与执行](admin-release-approvals.md)生效；恢复已发布变更可在待完结阶段由当前发布人[快速回滚](admin-release-rollbacks.md#免审批快速回滚t4--63)，或在完结后重新申请并审批回滚。角色验收来源为 [T1 #49](https://github.com/asherzj/relational-config-center/issues/49)，完整发布范围见[规格 #48](https://github.com/asherzj/relational-config-center/issues/48)。

## 使用与权限

新注册账号默认只有 VIEWER；首次通过 008 引入角色时，存量账号初始化为 VIEWER。已有角色的升级或迁移重跑保留原授予。不按注册顺序、用户名或邮箱授予管理员。账号启用状态与角色独立。每个角色都包含查看能力；角色覆盖当前部署的全部受管表，查询和写入仍须满足表规则。

| 角色 | 能力 |
|---|---|
| VIEWER | 查看规则目录、受管表、配置及本部署全部发布历史 |
| EDITOR | 创建发布草稿，编辑并提交本人普通草稿，复制已拒绝/取消的普通单据，申请回滚；尚未发布的本人单据可按状态取消 |
| APPROVER | 批准或拒绝他人提交的发布单，必须填写意见 |
| PUBLISHER | 手动执行已批准的发布单，包括获批的回滚单；审阅整单恢复预览、填写原因后免审批快速回滚，或明确完结已发布待完结的普通单；两者均不限定原发布人 |
| ADMIN | 管理角色与规则目录，包含上述业务能力，可取消尚未发布的单据；同样不得审批自己的单据 |

EDITOR、APPROVER、PUBLISHER 不互相隐含，可以组合分配。规则目录直接修改只允许 ADMIN，不进入发布审批；配置记录的旧 POST/PATCH/DELETE 写路由已删除。申请人持有 PUBLISHER 时可以执行他人已批准的本人单据，审批人也可以兼任发布人。`POST /api/v1/tables/:table_name/query` 是只读查询，但仍沿用 CSRF 与同源校验。`X-RCC-Roles` 等客户端身份头不授予权限。

每个业务请求从 MySQL 读取当前账号角色，原登录会话下一次请求立即按新授权执行；不承诺撤回已经认证的在途请求。Web 从当前身份显示角色，返回页面时复核；收到权限拒绝后刷新身份并保留尚未提交的输入，不自动重试写入。

## 初始化与恢复

1. 存量部署按[发布单升级指南](admin-release-upgrade.md)先备份并处理旧 FLOAT 身份在途单，再停止全部旧 Admin 及其他写入者，按[迁移说明](../deploy/mysql/migrations/README.md)确认既有迁移状态。已有 007 本地账号结构的部署继续应用 008～012；不要重跑 007 的一次性 DDL。008 初始化角色，009～012 建立记录版本和完整发布控制结构。新安装直接使用 `schema-migrate up`。角色初始化不能代替后续迁移或[发布所需数据库权限](design-notes/publication-contract.md#写入能力边界)。
2. 完成适用的 013 及当前 Policy 收缩，按[接管手册](schema-migrations.md#校验并接管现有库)显式执行 `schema-migrate baseline`，确认 `schema-migrate status` 为 `current` 且无未确认操作后再启动新版 Admin。Schema 就绪但没有 ADMIN 时，注册、登录和只读功能仍可用。
3. 在管理台注册明确指定的账号，然后由有数据库维护权限的人运行：

```bash
make build
bin/admin/account-maintain lookup --username alice.one
bin/admin/account-maintain grant-admin --id 550e8400-e29b-41d4-a716-446655440000
```

命令复用既有 `MYSQL_*` 连接配置，也可以通过 `--username alice.one` 选择账号。授予 ADMIN 保留其他角色；重复执行不会推进版本或增加重复授权历史。停用账号必须先显式 `enable`，再 `grant-admin`。

4. 使用该账号进入“平台管理 → 账号角色”，检索账号、组合选择角色并保存。页面显示当前授权和历史。

HTTP 撤权和 `account-maintain disable` 共同保护最后一个启用 ADMIN：先为另一启用账号授予 ADMIN，才可移除旧管理员。维护者忘记密码时使用既有 `reset-password`；账号全被异常外部操作停用时，可先 `enable` 已知账号，再 `grant-admin` 恢复。维护命令不提供绕过最后管理员检查的停用选项。

迁移 008 可重跑，并可在两个新增列之间中断后继续；已有角色、版本和历史不会被重置。Admin 启动和 readiness 检查角色及其余发布控制结构，缺失或不兼容时拒绝就绪，并提示对应迁移。不要将缺结构或无权限视为授予 ADMIN 的理由。回退旧二进制会重新开放旧同权或直写行为，必须在维护窗口处理，不能混跑；记录身份算法变更还须遵循[版本维护流程](admin-record-versions.md)。

## HTTP 契约

接口继续使用本地 Cookie、CSRF、同源校验及 `error.code/message/request_id`。`auth/register`、`auth/login`、`auth/session` 和返回当前身份的账号动作新增 `account.roles` 字符串数组。客户端不能向注册或资料更新传入角色。

仅 ADMIN 可使用以下管理接口。账号列表只返回 `id,username,display_name,enabled,roles,version`，不包含邮箱、口令散列或会话信息。

| 接口 | 行为 |
|---|---|
| `GET /api/v1/account-roles?q=&after=&limit=25` | 按用户名/显示名称包含匹配，或精确 Account ID 检索；按永久 ID 排序，`after` 为上一页最后 ID，limit 为 1～100 |
| `PUT /api/v1/account-roles/:id` | 保存完整角色集合，必须携带 `Idempotency-Key` 和读取到的 `expected_version` |
| `GET /api/v1/account-roles/:id/history?before=` | 每页至多 50 条历史，倒序读取，`before` 为上一页最后历史 ID |

列表响应为 `{accounts:[...],next_cursor:"..."}`，历史为 `{events:[...],next_cursor:"..."}`；空游标表示没有下一页。完整页可能返回一个游标，其下一页为空。数字版本和历史 ID 均为 JSON 字符串。

```json
{"roles":["EDITOR","APPROVER"],"expected_version":"1"}
```

角色不得为空、重复或包含未知值。请求标识为 8～64 个 ASCII 字母、数字、点、下划线或连字符，首位必须是字母/数字。建议 UUID。成功返回保存时的账号角色快照及新版本。

同一操作者、同一请求标识与同一规范内容返回原结果，即使携带的是原版本。不同内容复用请求标识返回 `409 idempotency_conflict`；新标识携带旧版本返回 `409 account_roles_conflict`，不覆盖其他管理员的修改。并发保存由同一事务边界保护。响应不确定时保持原标识重试，或读取当前授权与历史，不能自动换标识重放。此后若重试遇到 401/403，之前的结果仍未知；同账号恢复登录或权限后继续使用原标识与内容。明确版本冲突后的读取失败只重试读取，保留用户选择。

权限不足为 `403 permission_denied`，非法角色为 `422 invalid_account_roles`，最后管理员保护为 `409 last_administrator`，赋权目标不存在为 `404 account_not_found`；格式错误、依赖超时/不可用沿用账号错误映射。

## 持久化与审计

账号角色、授权版本和 `rcc_account_role_history` 的成功请求结果在一个事务提交。授权变更与账号启停复用既有账号控制锁，避免两个管理员并发互相撤权或同时停用后无人可管理。数据库异常回滚角色、版本和历史，不保留半完成请求。

历史保存操作者永久 Account ID、目标永久 ID、前后角色、时间和授权版本；显示名称变化不改变归属。维护命令以 `actor_kind=maintenance`、空 `actor_id` 明确区分数据库维护身份，不伪装成某个本地账号。历史及成功请求结果不自动清理，也无编辑、删除接口。它们只通过角色管理 API 可读，通用表发现和数据接口继续拒绝全部 `rcc_` 控制表。

普通访问日志只记路由模板、状态和请求编号，不记录角色请求正文、配置内容、口令或 Cookie。数据库维护权属于部署维护边界；任意外部 SQL 不受 HTTP 权限保护。
