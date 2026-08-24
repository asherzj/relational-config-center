# 关系型配置中心

Relational Configuration Center 是一个面向实体、字段和关系建模的配置管理系统。
它旨在为具有 Schema、约束、引用和关联查询需求的配置数据提供统一管理能力，区别于以独立键值或配置文件为主要管理单元的传统配置中心。

## 当前状态

项目已完成 Admin 第一迭代后端基线：通过运行时 Table Policy 管理一个部署配置的 MySQL 数据源，并提供受控的单表查询与变更 HTTP 接口。Server、Client 与 Web 的后续能力仍在迭代中。

- [Admin V1 技术基线](./docs/admin-v1-technical-baseline.md)
- [上下文地图](./CONTEXT-MAP.md)
- [Admin 领域术语](./admin/CONTEXT.md)

## 环境要求

- Go 1.27 或更高版本
- MySQL 8.4 LTS，或可运行 Docker Compose 的 Docker 环境

## 本地运行

Admin 默认监听 `127.0.0.1:8080`。认证默认开启，所有 `/api/v1/**` 请求都必须携带部署级 Bearer Token；`/health/live` 和 `/health/ready` 不需要认证。

使用 Docker Compose 启动 MySQL 8.4 和 Admin：

```bash
cp deploy/.env.example deploy/.env
docker compose --env-file deploy/.env -f deploy/docker-compose.yml up --build
```

请先修改 `deploy/.env` 中的密码和 `ADMIN_API_TOKEN`。该文件不应提交到仓库。Compose 会在全新 MySQL 数据卷中自动执行 `deploy/mysql/init/001-schema.sql` 初始化 Policy Catalog。

直接运行 Admin 时至少需要配置以下变量：

```bash
ADMIN_API_TOKEN='replace-me' \
MYSQL_HOST=127.0.0.1 \
MYSQL_DATABASE=rcc \
MYSQL_USER=rcc_admin \
MYSQL_PASSWORD='replace-me' \
MYSQL_TLS_MODE=false \
go run ./admin/cmd/admin
```

可选配置包括 `ADMIN_HTTP_ADDR`、`ADMIN_OPERATOR`、`ADMIN_CORS_ORIGINS` 及技术基线中列出的 MySQL 连接池和超时变量。仅当 `ADMIN_AUTH_DISABLED=true` 且监听地址是显式 loopback IP 时才能关闭认证；不能在通配或内网地址上关闭。

## 测试

```bash
make test
make build
make test-integration
```

集成测试使用 Testcontainers 和真实 MySQL 8.4；本机没有可用 Docker provider 时会明确跳过，不会以数据库 mock 替代。

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
