# 关系型配置中心

本地账号提供 `/register`、`/login` 和 `/account`，所有业务页面及 API 均要求真实 MySQL Cookie 会话，配置行和规则目录写入归属当前账号的永久 Account ID。启动需显式配置 `ADMIN_PUBLIC_ORIGIN`；本机 HTTP 还需 `ADMIN_ALLOW_LOCAL_HTTP=true`。参见[账号入口与 HTTP 契约](docs/admin-local-accounts.md)。旧共享 Token、免认证和固定 Operator 已移除；已包含草稿恢复、账号维护工具、缺结构启动检查和真实浏览器验收；参见[完整验收证据](docs/admin-local-accounts-evidence.md)。

Relational Configuration Center 是一个面向实体、字段和关系建模的配置管理系统。
它旨在为具有 Schema、约束、引用和关联查询需求的配置数据提供统一管理能力，区别于以独立键值或配置文件为主要管理单元的传统配置中心。

## 当前状态

项目已完成 Admin 第一迭代后端基线和正式 Web 管理台。Admin 通过运行时 Table Policy 治理一个部署配置的 MySQL 数据源中的既有表，Web 提供规则目录、表规则分配、受控的单表查询与变更操作。Server 与 Client 仍是后续迭代；`web/prototype/` 只作视觉参考，正式入口是 Vite/React 应用。

- [Admin V1 技术基线](./docs/admin-v1-technical-baseline.md)
- [Web 管理台运行与验收](./web/README.md)
- [上下文地图](./CONTEXT-MAP.md)
- [Admin 领域术语](./admin/CONTEXT.md)

## 环境要求

- Go 1.27 或更高版本
- Node.js 24.19.0 与 pnpm 10.28.2（Web）
- MySQL 8.4 LTS，或可运行 Docker Compose 的 Docker 环境

## 本地运行

Admin 是提供 HTTP API 的管理端后端，默认监听 `127.0.0.1:8080`；业务 `/api/v1/**` 请求必须携带有效会话，非 GET/HEAD 请求还需 CSRF 及同源来源；登录前准备、注册和登录入口公开。Web 管理台通过 Admin 的 API 和 Table Policy 治理表数据，Web 本身不直接连接 MySQL。`/health/live` 和 `/health/ready` 不需要认证。

使用 Docker Compose 启动 MySQL 8.4 和 Admin：

```bash
test -e deploy/.env || cp deploy/.env.example deploy/.env
# 编辑 deploy/.env，至少替换 MYSQL_ROOT_PASSWORD 和 MYSQL_PASSWORD
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up --build
```

请在启动前修改 `deploy/.env` 中的数据库密码。该文件不应提交到仓库；不要把上面的复制命令当作覆盖已有 `.env` 的更新方式。Compose 会在全新 MySQL 数据卷中自动执行 `deploy/mysql/init/001-schema.sql` 初始化 Policy Catalog，并在每次启动时幂等应用仅供本地开发使用的 `deploy/mysql/local-fixture/002-notification-templates.sql`。fresh volume，以及尚未包含同名资源或已包含完全相同 fixture 的已有 volume，会获得：

- 带 3 条可辨识样例数据的 `notification_templates`；
- Active 的 `notification_page_query_v1` Query Policy；
- 允许 ADD、MODIFY、DELETE 并配置标准审计 Auto Fill 的 Active `notification_full_mutation_v1` Mutation Policy；
- enabled `notification_templates` Table Policy。

fixture 位于独立的 `mysql/local-fixture` 路径，只由本地 Compose 的一次性 `mysql-local-fixture` 服务加载，不属于生产初始化脚本，也不会改变 Admin 只治理既有业务表的生产职责。缺失资源会被补齐，完全相同的资源会原样保留，重复启动不会重复插入样例行或 Policy；若已有 `notification_templates` Schema 不兼容，或同 Code Policy、同表分配与 fixture 的生命周期、执行规则或引用冲突，一次性服务会失败并保留既有资源，不会通过 SQL 接管表、覆盖或复活 Policy、重新启用分配。需要并行启动隔离环境时，可通过 `MYSQL_PUBLISHED_PORT` 和 `ADMIN_PUBLISHED_PORT` 覆盖默认的 3306 和 8080。

直接运行 Admin 时至少需要配置以下变量：

```bash
ADMIN_PUBLIC_ORIGIN=http://127.0.0.1:5173 \
ADMIN_ALLOW_LOCAL_HTTP=true \
MYSQL_HOST=127.0.0.1 \
MYSQL_DATABASE=rcc \
MYSQL_USER=rcc_admin \
MYSQL_PASSWORD='replace-me' \
MYSQL_TLS_MODE=false \
go run ./admin/cmd/admin
```

可选配置包括 `ADMIN_HTTP_ADDR`、`ADMIN_TRUSTED_PROXIES`、账号限速参数及技术基线中的 MySQL 连接池和超时变量。`ADMIN_API_TOKEN`、`ADMIN_AUTH_DISABLED`、`ADMIN_OPERATOR` 和旧跨源 `ADMIN_CORS_ORIGINS` 配置均已移除；提供非空旧配置会明确拒绝启动。自动化脚本也使用公开 Cookie/CSRF 登录流程，见账号契约。

## 测试

```bash
cd web
pnpm install --frozen-lockfile
pnpm test:run
pnpm typecheck
pnpm build

cd ..
make test
make build
make test-integration
```

集成测试使用 Testcontainers 和真实 MySQL 8.4；`make test-integration` 禁用 Go 测试缓存。本机没有可用 Docker provider 时测试会明确跳过，不会以数据库 mock 替代；持续集成会先执行 Docker 健康检查，因此 Docker 不可用时整个检查失败，不会跳过后假绿。

整组集成测试的进程上限为 25 分钟，以容纳隔离 MySQL 容器启动时间的波动；CI 任务另有 30 分钟总上限。各请求、数据库等待和进程停止的独立超时仍由对应测试验证。

## 持续集成

GitHub Actions 在所有面向 `main` 的 Pull Request 和所有 `main` 推送上并行执行四个检查：`Web`、`Go unit and build`、`MySQL 8.4 integration` 和 `Browser acceptance`。浏览器检查在 Linux runner 上使用 Playwright 的 Chromium、Firefox 和 WebKit；每个引擎单独写入 artifact 子目录。工作流使用只读仓库权限，并取消同一 Pull Request 或分支上的过期运行。

浏览器验收通过公开 Cookie 会话及 CSRF 流程进入管理台；未登录的 Admin 和 Web 代理都拒绝业务请求。临时账号、规则和业务数据只存在于本次创建的独立 MySQL 中，结束后连同数据库一起清理。

工作流当前只在推送到 `main` 和目标为 `main` 的 Pull Request 上运行这四个检查；推送到其他分支不会自动触发这套 CI。是否配置 branch protection、rulesets 或 required checks 由仓库设置决定，不能从本地文档推断为合并保证。

浏览器检查也可以在本地按套件或引擎运行。`all` 包含 `unsaved-changes`、`rule-clarity`、`write-recovery`、`operation-coverage`、`complex-fields` 和 `browser-accessibility`；后一个套件按 `RCC_E2E_ENGINES` 逐引擎运行。每次运行都应使用独立的空 artifact 目录：

```bash
RCC_E2E_ARTIFACTS=/tmp/rcc-browser-acceptance-$(date +%s) \
RCC_E2E_ENGINES=chromium,firefox,webkit \
  make test-browser-acceptance

RCC_E2E_SUITE=browser-accessibility \
RCC_E2E_ENGINE=firefox \
RCC_E2E_ARTIFACTS=/tmp/rcc-browser-accessibility-firefox \
  make test-browser-acceptance
```

本地阶段 5 的三引擎证据是在 macOS 上由 Playwright Chromium 151.0.7922.34、Firefox 153.0 和 WebKit 26.5 运行得到的；WebKit 结果代表 Playwright WebKit 构建，不代表系统 Safari 的所有发行版。Linux CI 是另一条实际环境边界，不能由 macOS 结果替代。使用 Colima 时，Admin integration 需要让 Docker client 和 Ryuk 都连接到 VM 内的 socket，例如：

```bash
DOCKER_HOST=unix://$HOME/.colima/default/docker.sock \
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock \
  go -C admin test -count=1 -timeout=25m -tags=integration ./...
```

只设置 `DOCKER_HOST` 会让 Ryuk 尝试把 macOS socket 路径挂载进 VM 并失败；本次环境在 provider 健康检查未通过时会跳过 integration，其他 Docker provider 可能自动发现 daemon，不能据此泛化。其他 provider 的 daemon 与 VM socket 仍需按本机环境核实。

## 项目结构

```text
admin/   第一迭代管理端后端 Go module
server/  后续运行时读取服务 Go module
client/  后续配置消费端 Go module
shared/  跨进程传输契约 Go module，不包含共享领域模型
web/     前端 module
docs/    跨模块设计与项目文档
```

## 开发约定

- 通过小而清晰的提交记录项目演进。
- 不向仓库提交密钥、令牌或本地环境配置。
- `admin`、`server`、`client` 和 `shared` 独立声明依赖，根目录通过 `go.work` 提供本地协作体验。
- 各上下文拥有自己的领域模型，传输契约必须通过 Adapter 转换。
- Go package 使用简短、清晰的小写名称。
- 引入新能力时同步补充测试和文档。
