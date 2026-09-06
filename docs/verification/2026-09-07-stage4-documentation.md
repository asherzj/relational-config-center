# 阶段 4：同步项目状态与运行验收文档

- 日期：2026-09-07
- 工作目录：`/private/tmp/rcc-management-work-package-20260907`
- 分支：`codex/management-work-package-20260907`
- 基线：`2e256858db548b19c8e6badecc075e2cded12c71`
- 对应事项：PM-006，项目状态与运行文档同步

## 交付结论

运行和验收文档已同步到阶段 1–3 已验证的实现状态。正式 Web 管理台当前包含 Query Policies、Mutation Policies、Table Policies 和 Managed Data 四个页面；`web/prototype/` 继续作为视觉参考，正式入口是 Vite/React 应用。Server 和 Client 仍只有后续迭代的骨架，尚未提供运行时配置服务或账户功能。

本报告所在分支的文档变更已完成并可供父代理复核；分支交付不表示已经合入 `main`。本阶段没有执行 commit、push、Issue 或 Notion 写入。

## 文档变更

- 根 [`README.md`](../../README.md) 更新当前状态、Compose 启动顺序、环境文件保护、Web 已交付状态和 CI 触发边界。
- [`docs/README.md`](../README.md) 区分已冻结的 Admin、已交付的 Web，以及仍待设计的 Server / Client；明确 prototype 不是正式入口。
- [`web/README.md`](../../web/README.md) 补充四个页面、规则效果与数据编辑能力、未保存保护、内存草稿限制、Vite 代理认证策略、生产 build/preview 边界和阶段 1–3 验收脚本。
- [`web/DESIGN.md`](../../web/DESIGN.md) 将实际效果说明、变化识别、未保存退出保护和对话框焦点约束标为已实现。
- [`docs/admin-v1-technical-baseline.md`](../admin-v1-technical-baseline.md) 保留「第一迭代仅后端」历史范围，同时增加当前 Web 入口说明；补充当前显式 SQL migration 文档的入口，没有把迁移框架写成已引入能力。

## 运行指引核对

- Node.js 与 pnpm 版本以 `web/package.json` 和 CI 为准：Node.js `24.19.0`、pnpm `10.28.2`。
- Go 使用根 `go.work` 约束的 Go `1.27` 工具链；根 `Makefile` 提供 `make test`、`make build` 和 `make test-integration`。
- Compose 指引要求复制 `deploy/.env.example` 后先替换 root password、应用 password 和 `ADMIN_API_TOKEN`，再执行 `up --build`；没有把复制命令当作覆盖已有环境文件的更新方式。
- Web 本地开发使用 `.env.local`，只由 Vite 读取 `RCC_ADMIN_URL` 和 `RCC_ADMIN_TOKEN`。Token 没有 `VITE_` 前缀，不进入浏览器包；生产环境由同源反向代理持有 Token。
- Admin 默认启用 Bearer Token；`/health/live` 和 `/health/ready` 不需要认证。CORS 使用配置的精确 origin，认证与 CORS 的实际行为以 Admin 源码为准。
- `pnpm dev` 的代理用于本地开发；`pnpm build` 生成生产静态资源，`pnpm preview` 是复用当前 Vite proxy 配置的本地预览，不等同完整生产部署；生产同源反向代理仍需独立配置。
- 迁移入口为 [`deploy/mysql/migrations/README.md`](../../deploy/mysql/migrations/README.md)；文档没有声称不存在 migration。

## 验证与复用证据

本阶段实际执行的低成本检查：

| 命令 | 结果 |
| --- | --- |
| `node --version` | `v24.19.0` |
| `pnpm --version` | `10.28.2` |
| `go version` | Go `1.27` 工具链 |
| `DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock docker compose --env-file deploy/.env.example -f deploy/docker-compose.yml config --quiet` | 通过；未输出环境值 |
| `git diff --check` | 通过 |
| 文档引用目标检查 | 通过；本报告及更新文档中核对的仓库内相对链接目标均存在 |

父代理另行完成了独立 preview smoke：在临时端口运行 `pnpm preview`，请求 `/api/stage4-probe` 返回 200，临时上游确认 `Authorization` 已注入，随后停止 preview 和上游进程。[机器结果](2026-09-07-stage4-preview-probe.json)证明当前 Vite preview 可复用 proxy 配置；它没有验证生产部署。

以下结果复用前三阶段已完成的实际验证，没有为文档重复执行昂贵套件：

- 阶段 1：106 个 Web 测试、Go 全模块 test/build、完整 MySQL 8.4 integration 通过；真实 Admin + Web + MySQL 流程和 fixture 证据见 [`2026-09-07-stage1-acceptance.md`](2026-09-07-stage1-acceptance.md)。
- 阶段 2：129 个 Web 测试、14 项真实 Chromium 验收通过；未保存退出、提交失败保留输入和隔离 fixture 复跑方式见 [`2026-09-07-stage2-unsaved-changes.md`](2026-09-07-stage2-unsaved-changes.md)。
- 阶段 3：130 个 Web 测试、typecheck/build、6 项真实 Chromium 只读验收通过；规则效果、实时 Schema 和移动视口证据见 [`2026-09-07-stage3-rule-clarity.md`](2026-09-07-stage3-rule-clarity.md)。

## 剩余限制

- 文档状态反映当前分支中已交付的实现；在父代理完成 commit/push 前，远端和 `main` 仍不包含本阶段文档变更。
- Web 草稿仅存在当前页面内存中，不提供自动保存、刷新恢复、自动重放写入或并发版本控制；当前写入语义仍是 last-write-wins。
- 阶段 1–3 的真实浏览器验收使用隔离 Admin 的 loopback auth-disabled 模式；生产 Bearer Token 认证和精确 CORS 的代码测试仍是部署边界依据。
- `pnpm preview` 是本地构建预览，当前配置可复用 Vite proxy；它不等同生产部署，生产仍需独立配置同源反向代理并由其保存秘密。
- Server、Client、账号、会话、发布、审批和多租户能力不在本阶段范围内。
