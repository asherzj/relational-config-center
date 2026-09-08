# 全局账号角色交付验证（2026-09-07）

工单：[T1 #49](https://github.com/asherzj/relational-config-center/issues/49)。父规格：[#48](https://github.com/asherzj/relational-config-center/issues/48)。本单只覆盖 AC-001～AC-006；完整发布单闭环尚未完成。

实现分支为 `codex/issue-49-account-roles`，隔离 worktree 从已确认的规格基点 `7c39d3dc369f1d3e1e4bee899250cc3eac4258b6` 建立。该基点来自 `origin/main` 的 `cd5df5845ea9b978f07639d7ac5d88243e400517`，仅增加发布单规格与工单文档。

## 验收证据

后端用例位于 `admin/cmd/admin/account_roles_integration_test.go`，通过真实 MySQL、正式 HTTP 路由和真实维护命令验证。

| 验收 | 证据与观察结果 |
| --- | --- |
| AC-001 默认只读 | `TestRegisteredAccountIsViewer` 和 `TestAccountRoleUpgradePreservesAccountsAndGrants`：新注册及迁移存量账号均为 VIEWER；旧会话可查询，无相应角色的写操作返回 403；迁移重跑不覆盖已授予角色。 |
| AC-002 显式初始化与组合角色 | `TestMaintainerBootstrapsAdministratorAndAssignsRoles`：真实 `grant-admin` 只授予指定账号，重复授予不重复变更。`make test-browser` 从默认 VIEWER 注册开始，经维护命令初始化 ADMIN，再在角色界面为独立账号组合分配 EDITOR、APPROVER。 |
| AC-003 原会话使用新权限 | `TestCurrentSessionUsesRolePermissionMatrix`：五角色逐项验证现有目录和记录入口，角色变化后复用原 Cookie/CSRF；浏览器独立账号也用原会话读到新角色。 |
| AC-004 服务端授权与控制表保护 | 同一权限矩阵验证仅 ADMIN 可管理规则和角色；伪造角色头无效；通用发现、查询和写入拒绝 `rcc_` 控制表。共享服务的请求身份和 HTTP 依赖方向由机器检查保护。 |
| AC-005 审计、幂等和失败原子性 | `TestRoleChangeIsAuditedAndIdempotent` 验证永久操作者/目标、前后角色、时间、原请求重放及同键不同内容冲突；`TestRoleChangeRaceAndAuditFailureDoNotLeavePartialGrants` 用真实 MySQL 触发器拒绝审计，证明 503 后角色、版本、历史无部分提交，并验证两个同版本请求仅一成功。浏览器验证历史中的永久操作者，并沿用进程日志与浏览器存储保密检查。 |
| AC-006 最后管理员与恢复 | `TestLastEnabledAdministratorCannotBeRemovedOrDisabled`、`TestRoleChangesAndAccountMaintenanceAreConcurrentSafe`：普通撤权和维护停用不能移除最后一个启用 ADMIN；并发仅一个操作成功；显式 enable/grant-admin 可按文档恢复。 |

Web 在公开 HTTP 边界使用契约响应。`AccountRolesPage.test.tsx` 共 9 个用例，覆盖组合赋权、只读入口、查询/变更规则撤权后保留输入、角色选择撤权后保留及离开保护、503 沿用原请求、冲突后的 GET 断网与重读，以及未知结果后遇到 401/403 的恢复。401 用例经过工作区遮罩和同账号重新登录，断言三次 PUT 的内容与请求标识相同。

## 测试先行与修复记录

逐个验收切片运行失败用例后实现。曾观察到的预期失败包括：未授权请求进入业务校验而非 403；维护 CLI 尚无 grant-admin；相同请求重放返回版本冲突；最后管理员可自我撤权；旧结构 readiness 未识别缺少角色列。对应切片在真实 MySQL 上逐项转绿。

评审补充的撤权表单用例先证实输入被重置；修复后保留输入并禁用提交。未知结果后重试遇到 401/403 的两个用例先因失去原请求重试入口失败；增加持续未知状态后，两例均通过。冲突后的读失败单独保存读取错误，不把一次失败 GET 误认为未知写入。

## 完整验证

环境：Go（仓库声明版本）、Node 24.19.0、pnpm 10.28.2、MySQL 8.4、真实 Chrome；浏览器通过 Vite 同源代理访问正式 Admin 进程与隔离数据库。Web 依赖按 frozen lockfile 安装。Colima 的 Docker 测试环境变量为：

```bash
export DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
```

| 命令 | 结果 |
| --- | --- |
| `make test` | admin、client、server、shared 全部通过；包含真实 Admin 子进程测试。 |
| `make build` | 所有模块及 admin、server、account-maintain 可执行文件构建通过。 |
| `make test-integration` | 全量通过；cmd/admin 1019.661 秒、HTTP 包 118.626 秒，其余包通过。新增审计故障/CAS 竞争用例在加入后另外真实运行通过（11.635 秒）。 |
| `cd admin && go test ./internal/interfaces/http -run 'TestHTTPDepends\|TestSharedBusiness' -count=1` | HTTP 只依赖 Application、共享业务服务不存储可变请求身份，均通过；保护集合已加入 AccountRoleManagement。 |
| `pnpm --dir web test:run` | 21 个测试文件、205 条测试通过。 |
| `pnpm --dir web build` | TypeScript 检查和 Vite 生产构建通过。 |
| `make test-browser` | 全部通过，44.287 秒。角色初始化/赋权/历史/原会话、390px 页面和抽屉、账号恢复、未保存保护、规则清晰度及真实业务写入均通过。 |
| `git diff --check` | 通过。 |

首次沙箱内运行 Go 进程测试受本机缓存/回环端口限制；在获准的本机测试环境重跑已通过，未以跳过测试代替。全量集成测试使用 Go 的包级汇总输出，等待完整退出后才记录成功。

真实浏览器检查了桌面角色列表及 390px 抽屉：长账号 ID 可换行，角色描述完整，保存/关闭可达，页面无横向溢出。截图在抽屉动画结束后采集并人工查看。可通过设置 `RCC_E2E_OUTPUT` 保留 `account-roles-desktop.png` 与 `account-roles-mobile.png`。

## Standards

规范轴复审通过，未留未解决发现。修复了撤权导致输入丢失的问题，角色行内操作改为 ghost，成功反馈使用现有 Sonner；均符合 `web/DESIGN.md` 和角色契约。

## Spec

规格轴复审通过，未留未解决发现。修复了 503 及后续认证拒绝可能丢失原请求标识的问题，分离 CAS 冲突后的读取错误；验收只覆盖本单 AC-001～AC-006，没有以角色基础交付代替发布审批闭环。

## 使用与后续边界

使用、升级、恢复和 HTTP 契约见 [全局账号角色](../admin-account-roles.md)，领域术语已同步到 `admin/CONTEXT.md`。存量部署应用迁移 008 后，维护者须明确授予首位 ADMIN；不允许混跑保留旧同权行为的 Admin 二进制。

旧记录写 API 仍是过渡入口，仅 EDITOR/ADMIN 可用；T5 #52 负责连同 Web、脚本和验收调用一起删除或迁移。APPROVER/PUBLISHER 的发布动作随 T4/T5 接入。下一张可执行工单是 T2 #33（记录版本/CAS），之后再推进发布草稿、审批、执行、批量和回滚；父规格 #48 保持打开。
