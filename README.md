# 关系型配置中心

本地账号提供 `/register`、`/login` 和 `/account`，所有业务页面及 API 均要求真实 MySQL Cookie 会话，配置行和规则目录写入归属当前账号的永久 Account ID。启动需显式配置 `ADMIN_PUBLIC_ORIGIN`；本机 HTTP 还需 `ADMIN_ALLOW_LOCAL_HTTP=true`。参见[账号入口与 HTTP 契约](docs/admin-local-accounts.md)。旧共享 Token、免认证和固定 Operator 已移除；已包含草稿恢复、账号维护工具、缺结构启动检查和真实浏览器验收；参见[完整验收证据](docs/admin-local-accounts-evidence.md)。

Relational Configuration Center 是一个面向实体、字段和关系建模的配置管理系统。
它旨在为具有 Schema、约束、引用和关联查询需求的配置数据提供统一管理能力，区别于以独立键值或配置文件为主要管理单元的传统配置中心。

## 当前状态

项目已完成 Admin 第一迭代后端基线：通过运行时 Table Policy 管理一个部署配置的 MySQL 数据源，并提供受控的单表查询与变更 HTTP 接口。Server、Client 与 Web 的后续能力仍在迭代中。

- [Admin V1 技术基线](./docs/admin-v1-technical-baseline.md)
- [上下文地图](./CONTEXT-MAP.md)
- [Admin 领域术语](./admin/CONTEXT.md)

## 环境要求

- Go 1.27 或更高版本
- Node.js 24.19.0 与 pnpm 10.28.2（Web）
- MySQL 8.4 LTS，或可运行 Docker Compose 的 Docker 环境

## 本地运行

Admin 默认监听 `127.0.0.1:8080`。业务 `/api/v1/**` 请求必须携带有效会话，非 GET/HEAD 请求还需 CSRF 及同源来源；登录前准备、注册和登录入口公开；`/health/live` 和 `/health/ready` 不需要认证。

使用 Docker Compose 启动 MySQL 8.4 和 Admin：

```bash
cp deploy/.env.example deploy/.env
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up --build
```

请先修改 `deploy/.env` 中的数据库密码。该文件不应提交到仓库。Compose 会在全新 MySQL 数据卷中自动执行 `deploy/mysql/init/001-schema.sql` 初始化 Policy Catalog，并在每次启动时幂等应用仅供本地开发使用的 `deploy/mysql/local-fixture/002-notification-templates.sql`。fresh volume，以及尚未包含同名资源或已包含完全相同 fixture 的已有 volume，会获得：

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

## 持续集成

GitHub Actions 在所有面向 `main` 的 Pull Request 和所有 `main` 推送上并行执行三个稳定检查：`Web`、`Go unit and build`、`MySQL 8.4 integration`。工作流使用只读仓库权限，并取消同一 Pull Request 或分支上的过期运行。

当前私有仓库套餐不支持 branch protection 或 rulesets，因此这些检查会可靠地报告红绿状态，但尚不能阻止维护者绕过检查直接写入 `main`。升级套餐或调整仓库可见性并配置 required checks 后，三个稳定检查名可直接作为不可绕过的合并门禁。

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
