# Web

关系型配置中心的浏览器前端。正式应用使用 Vite、React、TypeScript、React Router、TanStack Query 和 Zod；`prototype/` 继续作为交互与视觉参考，但不参与正式应用运行。

## 当前能力

- 中文化应用壳和正式 URL 路由。
- 四个正式页面：Query Policies、Mutation Policies、Table Policies 和 Managed Data，默认入口重定向到 Query Policies。
- 类型化 Admin API Client，HTTP DTO 只停留在 `src/api` 边界。
- Zod 运行时响应校验与稳定错误码映射。
- 查询规则的列表、详情、创建草稿、替换草稿、激活、弃用、更新元数据和删除草稿。
- 变更规则的规则类型注册表、完整目录、草稿编辑、生命周期操作、元数据更新和四个固定 Auto Fill 槽位。
- Database Table Discovery 的真实表状态、稳定不兼容原因和表规则完整目录。
- 仅从兼容未分配表和 Active 规则创建未启用分配，并支持详情、原子替换、启用、停用与客户端筛选。
- 替换已启用表规则前明确提示下一次请求立即生效；实时 Schema 与引用错误保留稳定错误码和 Request ID。
- 配置内容管理只列出 enabled Managed Table，以实时动态列构造全部八种 Query Spec 操作符、单字段排序和服务端分页。
- Managed Data 值保持 JSON String 语义，并在结果中明确区分 SQL NULL 与空字符串。
- 变更规则驱动的 ADD、MODIFY、DELETE 始终显示能力状态；未授权、未知类型、无效 Auto Fill 或不可执行规则快照均失败关闭。
- 通用写入编辑器以字段开关表达省略，并区分 NULL、空字符串和普通 JSON String。ADD 可显式填写非自增主键 `id`；自增主键可保持省略。MODIFY 不修改 `id`，全部 Auto Fill 字段由后端填充。
- ADD、MODIFY、DELETE 共用完整字段 Change Set；收到成功响应后 ADD/MODIFY 以返回的 exact id 回查数据库最终值，DELETE 显示删除摘要。未知写入结果保留输入并锁定再次提交，只有只读核对与明确的人工确认才允许继续；未收到 ADD 的真实返回 ID 时，核对当前页而不猜测输入 ID。
- 规则详情把名称和描述与执行规则分开；执行规则区块解释查询排序/分页、变更授权和 Auto Fill 的实际效果，修改名称和描述不会改变执行内容。
- 未知规则类型或不完整的变更类型能力失败关闭：未知 Draft 只能安全查看，Active 或 Deprecated 只能更新名称和描述等元数据。
- GET 仅对网络错误、503、504 自动重试一次；写命令不自动重试。
- 规则和数据编辑均有未保存退出保护；提交失败保留内存草稿，提交成功才清除草稿。草稿不写入 `localStorage`、`sessionStorage` 或 URL。

## 本地开发

Web 固定使用 Node.js 24.19.0 与 pnpm 10.28.2；`package.json` 同时声明两者，确保本地开发与持续集成使用相同工具链。

Admin 默认运行在 `http://127.0.0.1:8080`。复制环境变量示例并确认 Admin 代理地址：

```bash
cd web
test -e .env.local || cp .env.example .env.local
pnpm install
pnpm dev
```

浏览器只请求同源 `/api/v1`，使用 HttpOnly 会话 Cookie 和仅存于内存的 CSRF 凭据。Vite 仅读取 `RCC_ADMIN_URL`，转发原请求；旧 `RCC_ADMIN_TOKEN` 会明确报错。Admin 的 `ADMIN_PUBLIC_ORIGIN` 必须与浏览器地址一致，本机 HTTP 显式启用 `ADMIN_ALLOW_LOCAL_HTTP=true`。生产部署使用同源 HTTPS 反向代理，不能继续注入共享 Token。

工作区先检查真实当前身份；未登录时转到登录页并保留安全的站内目标，注册或登录成功后返回。规则或 CSRF 拒绝的 403 不跳登录；会话失效的 401 转登录，服务故障保留凭据并提供重新检查。工作区与账号页复用同一浏览器 Web Lock 活动协调，业务写入不自动重放。同账号重新登录后会重新读取当前规则和目标数据，再恢复内存中的编辑内容并要求重新确认；退出或切换账号时清除草稿。

`pnpm dev` 的代理用于本地开发；`pnpm build` 生成生产静态资源，`pnpm preview` 用于本地预览并复用当前 Vite proxy 配置。preview 不能代替生产环境的同源 HTTPS 反向代理。

## 验证

```bash
pnpm install --frozen-lockfile
pnpm typecheck
pnpm test:run
pnpm build
```

## 边界

- Web 不推断 generated、auto_increment、默认值或新增必填字段，Admin 仍以实时 Schema 做最终裁决。
- Change Set 不做提交前并发刷新；当前管理语义保持 last-write-wins。
- Managed Data 的编辑值只保存在当前页面内存中；取消离开提醒可继续编辑，明确允许刷新、关闭标签页或放弃后不会恢复，也没有自动保存、自动重放写入或并发版本控制。
- `web/prototype/` 继续用于视觉参考，正式应用由 Vite/React 入口运行。

## 真实验收

从仓库根目录执行以下命令；它会从干净 checkout 启动一次性的 MySQL 8.4、Admin 和 Web preview，加载隔离 fixture，注册临时账号，并运行未保存保护、规则说明、写入恢复、操作覆盖、复杂字段及浏览器可访问性验收：

```sh
make test-browser-acceptance
```

该命令要求本机已有 Docker、Go、Node.js 和 pnpm；它会安装锁定的 Web 依赖、默认使用 Chromium，并构建 Web。MySQL、Admin 和 Web 均使用动态宿主端口；正常或失败退出时只删除本次创建的进程、容器和数据卷。日志、截图与结构化结果写入命令最后显示的临时目录，可通过 `RCC_E2E_ARTIFACTS` 指定一个新的空目录。CI 执行同一命令并设置 `RCC_E2E_ENGINES=chromium,firefox,webkit`，每个引擎使用独立的 artifact 子目录，成功或失败时均上传证据。

runner 支持按套件和引擎缩小范围。`RCC_E2E_SUITE` 可选 `all`、`unsaved-changes`、`rule-clarity`、`write-recovery`、`operation-coverage`、`complex-fields` 或 `browser-accessibility`；`RCC_E2E_ENGINES` 是逗号分隔的 `chromium`、`firefox`、`webkit`，也可用 `RCC_E2E_ENGINE` 选择单个引擎。默认 `all` 包含前五个 Chromium 套件以及 `browser-accessibility` 的所选引擎。每次本地运行请指定新的空输出目录，例如：

```sh
RCC_E2E_SUITE=browser-accessibility \
RCC_E2E_ENGINES=firefox,webkit \
RCC_E2E_ARTIFACTS=/tmp/rcc-browser-accessibility-firefox-webkit \
  make test-browser-acceptance
```

每个套件通过公开注册接口取得独立 Cookie 会话，并在内存中复用该账号；页面自己发送 CSRF，脚本不向页面请求注入认证 Header。只读主键回查等直接 APIRequest 操作显式取得当前 CSRF。独立认证检查证明无会话为 401、缺 CSRF 为 403、注册后查询为 200、退出后恢复 401。账号及会话随专属数据库销毁，不落盘保存 Cookie、CSRF 或注册密码。

本地 macOS 阶段 5 已分别验证 Chromium 151.0.7922.34、Firefox 153.0 和 Playwright WebKit 26.5；WebKit 结果代表 Playwright 构建，不代表系统 Safari 的所有发行版。Linux CI 的三引擎运行是独立的环境证据，不能由 macOS 结果替代。使用 Colima 时，Admin integration 需要同时指定 Docker daemon 和 VM 内的 Ryuk socket：

```sh
DOCKER_HOST=unix://$HOME/.colima/default/docker.sock \
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  go -C admin test -count=1 -timeout=20m -tags=integration ./...
```

只设置 `DOCKER_HOST` 会让 Ryuk 尝试把 macOS socket 路径挂载进 VM 并失败；本次环境在 provider 健康检查未通过时会跳过 integration，其他 Docker provider 可能自动发现 daemon，不能据此泛化。其他 provider 的路径需要按本机环境核实。

账号专用系统验收入口也保留：从仓库根目录运行 `make test-browser`：它创建独立 MySQL、Admin、同源 Vite 和浏览器环境，依次验证账号注册与会话恢复、未保存编辑保护、规则效果说明，再销毁测试资源。需要已安装 Web 依赖、可用的 Docker 和 Chrome；也可通过 `RCC_BROWSER_EXECUTABLE` 指定 Chromium。该入口使用公开账号会话与 CSRF 流程，不依赖已移除的免认证模式。

完整流程、真实 MySQL 8.4 和浏览器验收见 [`docs/verification/2026-09-07-stage1-acceptance.md`](../docs/verification/2026-09-07-stage1-acceptance.md)。未保存保护见 [`docs/verification/2026-09-07-stage2-unsaved-changes.md`](../docs/verification/2026-09-07-stage2-unsaved-changes.md)，规则效果说明见 [`docs/verification/2026-09-07-stage3-rule-clarity.md`](../docs/verification/2026-09-07-stage3-rule-clarity.md)。浏览器脚本使用隔离 fixture，运行前先启动隔离 Admin、Web 和 MySQL；不要对生产环境运行脚本：

从仓库根目录运行脚本，并为输出指定新的临时目录，避免覆盖历史报告：

```sh
RCC_PLAYWRIGHT_MODULE=/absolute/path/to/node_modules/playwright \
RCC_E2E_OUTPUT=/tmp/rcc-stage4-unsaved \
  node web/e2e/unsaved-changes.cjs
RCC_PLAYWRIGHT_MODULE=/absolute/path/to/node_modules/playwright \
RCC_E2E_OUTPUT=/tmp/rcc-stage4-rule-clarity \
  node web/e2e/rule-clarity.cjs
```

两个脚本都支持 `RCC_WEB_URL` 和 `RCC_E2E_OUTPUT`；只有 `unsaved-changes.cjs` 支持 `RCC_E2E_TABLE`。阶段 1 的完整重建和 fixture 加载方式见其报告。正常停止自建 Compose 环境时，从仓库根目录执行 `docker compose --env-file <env-file> -f deploy/docker-compose.yml -p <isolated-project> down`，将环境文件路径和项目名替换为启动时使用的值。原生 Admin 与 Web 在各自启动终端用 Ctrl+C 停止。

手动运行这些脚本前，须按[本地账号运行手册](../docs/admin-local-accounts.md)配置当前认证与数据库结构；历史阶段报告中的 Token 或免认证启动方式不适用于当前版本。每个脚本通过公开 API 注册随机测试账号，邮箱使用 `@example.invalid` 且不执行邮件操作，清理规则草稿时携带当前会话的 CSRF。测试账号随隔离数据库销毁，不应在共享数据库运行这些脚本。
