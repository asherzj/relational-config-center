# 全局账号角色

对应 [发布单 T1 #49](https://github.com/asherzj/relational-config-center/issues/49)，父规格 [#48](https://github.com/asherzj/relational-config-center/issues/48) 的 AC-001～AC-006。本阶段交付账号角色；配置记录仍使用过渡写入口，T5 #52 负责迁移为发布单并关闭旧入口。

## 使用与权限

新注册账号和升级后的存量账号默认只有 VIEWER，不按注册顺序、用户名或邮箱授予管理员。账号启用状态与角色独立。每个角色都包含查看能力；角色覆盖当前部署的全部受管表，查询和写入仍须满足表规则。

| 角色 | 能力 |
|---|---|
| VIEWER | 查看规则目录、受管表及配置 |
| EDITOR | 编辑配置；后续 T3 接入发布草稿 |
| APPROVER | 保留审批授权，后续 T4 接入审批他人单据 |
| PUBLISHER | 保留发布授权，后续 T5 接入执行已批准单据 |
| ADMIN | 管理角色、规则目录及配置，包含上述业务能力；后续审批仍不得自批 |

EDITOR、APPROVER、PUBLISHER 不互相隐含，可以组合分配。规则目录直接修改只允许 ADMIN；过渡记录 POST/PATCH/DELETE 只允许 EDITOR 或 ADMIN。`POST /api/v1/tables/:table_name/query` 是只读查询，但仍沿用 CSRF 与同源校验。`X-RCC-Roles` 等客户端身份头不授予权限。

每个业务请求从 MySQL 读取当前账号角色，原登录会话下一次请求立即按新授权执行；不承诺撤回已经认证的在途请求。Web 从当前身份显示角色，返回页面时复核；收到权限拒绝后刷新身份并保留尚未提交的输入，不自动重试写入。

## 初始化与恢复

1. 存量部署先停止全部旧 Admin 写入口，备份数据库，完成迁移 007 后应用 `deploy/mysql/migrations/008-account-roles.sql`。新安装直接使用 `deploy/mysql/init/001-schema.sql`。
2. 启动新版 Admin。Schema 就绪但没有 ADMIN 时，注册、登录和只读功能仍可用。
3. 在管理台注册明确指定的账号，然后由有数据库维护权限的人运行：

```bash
make build
bin/admin/account-maintain lookup --username alice.one
bin/admin/account-maintain grant-admin --id 550e8400-e29b-41d4-a716-446655440000
```

命令复用既有 `MYSQL_*` 连接配置，也可以通过 `--username alice.one` 选择账号。授予 ADMIN 保留其他角色；重复执行不会推进版本或增加重复授权历史。停用账号必须先显式 `enable`，再 `grant-admin`。

4. 使用该账号进入“平台管理 → 账号角色”，检索账号、组合选择角色并保存。页面显示当前授权和历史。

HTTP 撤权和 `account-maintain disable` 共同保护最后一个启用 ADMIN：先为另一启用账号授予 ADMIN，才可移除旧管理员。维护者忘记密码时使用既有 `reset-password`；账号全被异常外部操作停用时，可先 `enable` 已知账号，再 `grant-admin` 恢复。维护命令不提供绕过最后管理员检查的停用选项。

迁移 008 可重跑，并可在两个新增列之间中断后继续；已有角色、版本和历史不会被重置。Admin 启动和 readiness 检查角色控制结构，缺失时提示迁移 008。不要将缺结构或无权限视为授予 ADMIN 的理由。回退旧二进制会重新开放旧同权行为，必须在维护窗口处理，不能混跑。

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
