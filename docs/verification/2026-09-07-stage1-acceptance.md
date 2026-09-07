# 阶段 1：完整流程验收与修复

- 日期：2026-09-07
- 分支：`codex/management-work-package-20260907`
- 隔离 worktree：`/private/tmp/rcc-management-work-package-20260907`
- 规格来源：GitHub Issue #22（只读）

## 验收结论

真实 MySQL 8.4、Admin 与 Web 的管理流程已走通。浏览器实测覆盖 Query / Mutation 规则创建与激活、表分配与启停、八种查询运算、分页排序、ADD / MODIFY / DELETE、Change Set 确认、失败后返回修改，以及数据库最终值回查。

本轮发现并修复两个问题：叠层确认框没有取得焦点且键盘可落到下层抽屉；MySQL 对非法 ENUM 值报 1265 时被错误归为服务不可用。两项都先得到失败证据，再以自动化回归和真实浏览器复测验证修复。

## 隔离环境

- Compose project：`rcc-stage1-20260907`。
- 数据库：独立 `mysql:8.4` 容器、独立数据卷、随机发布端口；没有复用或修改用户已有数据库。
- Admin：从本 worktree 本机运行，监听 `127.0.0.1:18081`，只接受本轮 Web origin；隔离验收采用本机禁用认证模式。
- Web：从本 worktree 运行 Vite，监听 `127.0.0.1:15173`。
- fixture 表：`stage1_acceptance_items`，含 5 行隔离种子数据，以及 nullable、空字符串、ENUM、默认值和审计字段。

可按以下方式重建环境；`deploy/.env.stage1` 为 git ignored 的本地文件，不应提交或打印其内容：

```sh
cp deploy/.env.example deploy/.env.stage1
MYSQL_PUBLISHED_PORT=0 docker compose -p rcc-stage1-20260907 \
  --env-file deploy/.env.stage1 -f deploy/docker-compose.yml \
  up -d mysql mysql-local-fixture
docker compose -p rcc-stage1-20260907 \
  --env-file deploy/.env.stage1 -f deploy/docker-compose.yml \
  port mysql 3306
docker compose -p rcc-stage1-20260907 \
  --env-file deploy/.env.stage1 -f deploy/docker-compose.yml \
  exec -T mysql sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -u"$MYSQL_USER" "$MYSQL_DATABASE"' \
  < docs/verification/fixtures/stage1_acceptance.sql
```

`mysql-local-fixture` 只创建默认的 notification fixture；阶段 1 的表必须额外执行 `docs/verification/fixtures/stage1_acceptance.sql`。取得随机发布端口后，以 `MYSQL_HOST=127.0.0.1`、该发布端口、`MYSQL_TLS_MODE=false`、`ADMIN_HTTP_ADDR=127.0.0.1:18081`、`ADMIN_AUTH_DISABLED=true` 和 `ADMIN_CORS_ORIGINS=http://127.0.0.1:15173` 运行 `go run ./admin/cmd/admin`。Web 以 `RCC_ADMIN_URL=http://127.0.0.1:18081 pnpm dev --port 15173` 启动。

本轮 worktree 位于 `/private/tmp`，Colima 没有共享该目录，Compose 的初始化 SQL bind mount 因而不可见。实际验收通过上述 `exec -T ... < file.sql` 方式依次输入 `deploy/mysql/init/001-schema.sql`、`deploy/mysql/local-fixture/002-notification-templates.sql` 和阶段 1 fixture；这是本机目录共享限制，不是应用故障。在 Colima 已共享的 `/Users/...` checkout 中，前两项由 Compose 初始化和 `mysql-local-fixture` 自动完成，只需额外输入阶段 1 fixture。

## 浏览器实跑

| 用例 | 实跑结果 |
| --- | --- |
| 创建 Query 规则 | 创建 `stage1_query_v1`，默认 `id DESC`，分页 20 / 200，激活成功。第一次故意使用错误 CORS origin 得到带 Request ID 的 403，表单内容保留；修正 Admin origin 后原表单重试成功。 |
| 创建 Mutation 规则 | 创建 `stage1_mutation_v1`，允许 ADD / MODIFY / DELETE，为 `created_by`、`created_at`、`updated_by`、`updated_at` 配置自动填写，激活成功。 |
| 发现与分配表 | discovery 找到未分配且兼容的 `stage1_acceptance_items`；以禁用状态创建 Table Policy，再启用成功。 |
| 精确查询 | `id = 3` 返回 Gamma，共 1 行。 |
| 包含查询 | `name contains mm` 返回 Gamma，共 1 行。 |
| 开区间 | `priority in (20, 50)` 返回 Gamma、Delta，共 2 行。 |
| 闭区间 | `priority in [20, 50]` 返回 Beta、Gamma、Delta、Epsilon，共 4 行。 |
| 集合包含 / 排除 | `name in [Alpha, Delta]` 返回 2 行；`not_in` 返回其余 3 行。 |
| NULL 判断 | `category is_null` 返回 Delta；`is_not_null` 返回其余 4 行。 |
| 排序与分页 | `name ASC`、每页 2 行；第一页 Alpha/Beta，下一页 Delta/Epsilon；显示总计 5 行、第 2/3 页。 |
| ADD | 新增 Zeta：`category=NULL`、`note=""`、`state=active`，省略 `priority`。Change Set 展示全部字段，区分未提交、NULL、空字符串及 Auto Fill。成功回查 id=6、priority=0、审计字段已填充。 |
| MODIFY | id=6 改名为 Zeta Updated，`note` 从空字符串改为 NULL，同时提交未变化的 `state`。Change Set 只高亮真实变化，审计字段显示 Auto Fill；回查值正确且 `updated_at` 前进。 |
| DELETE | Change Set 显示完整原值到不存在；执行后成功摘要包含 id=6，按该 id 查询为空。 |
| 失败后保留输入 | ADD 使用非法 `state=not-a-valid-state`。修复并重启本 worktree Admin 后，同一浏览器 Change Set 返回 400 `invalid_mutation_content`（Web 文案“写入内容不符合实时字段 Schema。”）及 Request ID；返回修改后名称、非法值和字段勾选状态仍完整保留。 |
| 停用表规则 | Web 停用表规则后 Managed Data 候选表立即消失；loopback API 查询得到 403 `table_policy_disabled` 及 Request ID；随后在 Web 重新启用。 |
| 启用中原子替换 | 将 Query 引用由 `stage1_query_v1` 替换为已激活的 `notification_page_query_v1`，确认框明确提示下一次请求立即使用新完整快照；提交后引用已更新且策略仍启用。 |
| 已分配规则弃用 | 在 Web 弃用 `stage1_mutation_v1` 后，既有表分配仍可引用这条规则；每次请求仍重新读取所引用规则的完整快照。Managed Data 仍可查，ADD 入口仍可用。 |
| 键盘与焦点 | 修复前真实浏览器打开“激活查询规则”时焦点留在下层 Drawer，Tab 可遍历下层控件。修复后取消按钮初始聚焦，Tab / Shift+Tab 在顶层确认框循环；Escape 只取消顶层框。Change Set 初始聚焦且 Shift+Tab 循环到“确认并执行”；Drawer 的 Escape 关闭并把焦点还给“新增记录”。 |
| 移动视口 | 在 390×844 实测移动菜单开合、全部导航项、查询表单的纵向布局与可操作性；随后恢复桌面视口。 |

连续输入时的焦点重置没有在真实浏览器中复现。确定性组件测试把 `requestAnimationFrame` 设为立即执行后，证明父组件重渲染会让旧 Drawer effect 抢回焦点；修复后测试通过。因此这里只把叠层确认框问题记录为真实浏览器复现，把重渲染抢焦点记录为自动化测试复现。

## 数据库终态

直接从隔离 MySQL 回查：

- `stage1_acceptance_items` 恢复为 5 行种子数据，新增后删除的 id=6 不存在。
- `stage1_query_v1` 为 `ACTIVE`。
- `stage1_mutation_v1` 为 `DEPRECATED`。
- `stage1_acceptance_items` 的 Table Policy 仍启用，Query 引用为 `notification_page_query_v1`，Mutation 引用为 `stage1_mutation_v1`。

## 问题、修复与证据

### 叠层 modal 焦点逃逸

旧 Drawer 在每次 `onClose` 闭包变化时重复执行聚焦 effect；ConfirmDialog 和 Change Set 没有一致的初始焦点、焦点循环和顶层 Escape 规则。真实浏览器可见确认框打开后焦点仍在下层 Drawer。

先增加 `ModalFocus.test.tsx`，初次运行 2/2 失败，焦点实际落在 Drawer。随后增加共享 `useModalFocus`：只在打开状态变化时设置初始焦点，记录并恢复前一焦点，只让 DOM 中最上层 modal 接收 Escape，并在其中循环 Tab。Button 改为 `forwardRef`，让确认框能直接聚焦取消按钮。组件测试变为 2/2 通过，真实浏览器键盘路径也通过。

### 非法 ENUM 被错误归为服务不可用

真实浏览器提交非法 ENUM 成员时，Admin 起初返回 503 `mutation_unavailable`，但这是用户提交内容与实时 schema 不符，不应被当成临时数据库故障。

先在真实 MySQL integration 中增加用例，初次得到 503。随后按 MySQL 错误码分类：1062 保持 duplicate 映射；1265 映射到 `ErrInvalidMutation`，API 返回 400 `invalid_mutation_content`；其他数据库错误仍保持 unavailable。integration 用例转绿，并在重启本 worktree Admin 后用同一浏览器 Change Set 复测 400 及输入保留。

## 改动文件

本轮针对验收证据新增或修改：

- `admin/cmd/admin/mutation_integration_test.go`
- `admin/cmd/admin/testdata/006-mutation-fixture.sql`
- `admin/internal/infrastructure/mysql/adapter.go`
- `web/src/components/ui/Button.tsx`
- `web/src/components/ui/ConfirmDialog.tsx`
- `web/src/components/ui/Drawer.tsx`
- `web/src/components/ui/ModalFocus.test.tsx`
- `web/src/components/ui/useModalFocus.ts`
- `web/src/features/managed-data/ChangeSetDialog.tsx`
- `web/src/features/managed-data/MutationSuccessDialog.tsx`
- `docs/verification/2026-09-07-stage1-acceptance.md`
- `docs/verification/fixtures/stage1_acceptance.sql`

此外，本阶段验收覆盖 worktree 基线中未提交的 Admin policy snapshot 重构、Web mutation workflow / lifecycle 重构和管理台页面改动；这些文件由总工作包基线统一记录，不在此重复列为本轮问题修复。

## 自动化回归

| 命令 | 结果 |
| --- | --- |
| `pnpm vitest run src/components/ui/ModalFocus.test.tsx` | 修复前 2/2 失败；修复后 2/2 通过。 |
| `pnpm test:run` | 16 个测试文件、106 个测试全部通过。 |
| `pnpm typecheck` | 通过。 |
| `pnpm build` | TypeScript no-emit 与 Vite production build 通过，共转换 1974 个模块。 |
| `make test` | Go 全模块测试通过。 |
| `make build` | `client`、`shared` 库和 `admin`、`server` 命令模块全部构建通过；第一次仅因 sandbox 不允许写宿主 Go build cache 失败，使用已授权的本机执行后通过。 |
| `go test -count=1 -timeout=20m -tags=integration -run '^TestMutationPolicyUsesDefaultsAndNullabilityAndRejectsInvalidInputFields$' ./cmd/admin` | 非法 ENUM 用例修复前红、修复后绿；最终复跑 9.391 秒通过。 |
| `make test-integration` | 通过；`admin/cmd/admin` 的真实 MySQL 套件耗时 587.672 秒，其余 integration package 同步通过。 |
| `docker compose ... exec -T mysql ... < docs/verification/fixtures/stage1_acceptance.sql` | 在隔离数据库重载成功；回查为 5 行，id 范围 1–5。 |
| `git diff --check` | 通过。 |

## 未覆盖与限制

- 浏览器人工验收覆盖当前桌面视口和 390×844 移动视口，没有穷举设备或使用真实屏幕阅读器；键盘焦点同时有自动化测试。
- 隔离 Admin 使用 auth-disabled loopback 模式，因此没有在浏览器中重跑生产反向代理的令牌转发；本轮 CORS 拒绝路径已实测，认证转发由既有自动化测试覆盖。
- Admin 容器镜像构建不是业务验收前置条件；Compose build 在本机 buildx / 网络阶段无有效进展后改用本 worktree 的本机 Admin。相同源码的 Go test 和 build 均已通过。
- 没有从外部会话并发执行 DDL；policy snapshot 的 schema 漂移及并发相关行为由本轮纳入的 integration 回归覆盖。
- 未保存编辑保护属于阶段 2，本阶段只验证提交失败后由既有 mutation workflow 保留输入。

## 下一阶段复用状态

- MySQL Compose project `rcc-stage1-20260907` 仍在运行，宿主访问地址为 `127.0.0.1:32768`；数据卷未删除。
- Colima Docker socket：`unix:///Users/asher/.colima/default/docker.sock`。
- 本地环境文件：`/private/tmp/rcc-management-work-package-20260907/deploy/.env.stage1`，已被 git ignore；不要打印其内容。
- Admin unified exec session：`74802`，监听 `127.0.0.1:18081`。
- Web unified exec session：`46016`，监听 `127.0.0.1:15173`。
- Codex In-app Browser：browser id `2`，tab id `1`，provider tab id `browser-use:29f9bdcf-c946-4d5d-a56f-1cf36636ff7b`；当前 URL 为 `http://127.0.0.1:15173/configuration/managed-data`。
- 当前页面和两项服务在交接前再次读回，均仍可用；浏览器 Managed Table 候选包含 `stage1_acceptance_items`。
