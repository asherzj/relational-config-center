# 关系型配置中心

Relational Configuration Center 是一个面向实体、字段和关系建模的配置管理系统。
它旨在为具有 Schema、约束、引用和关联查询需求的配置数据提供统一管理能力，区别于以独立键值或配置文件为主要管理单元的传统配置中心。

## 当前状态

Admin 第一迭代提供了策略驱动的 MySQL 通用表访问能力。注册一张表的查询和变更策略后，Web 可以通过统一 HTTP API 获取策略元数据、分页查询并执行受限的创建、更新和删除操作。

## 环境要求

- Go 1.27 或更高版本
- 推荐使用 Docker 与 Docker Compose 直接启动 Admin 和 MySQL

## 本地运行

```bash
docker compose up --build
```

Admin 默认监听 `http://localhost:8080`，MySQL 使用 8.4 系列镜像。API 示例见 [Admin managed-table API](./docs/admin-api.md)。

如果已有 MySQL，也可以直接运行：

```bash
RCC_MYSQL_DSN='user:password@tcp(127.0.0.1:3306)/rcc?charset=utf8mb4' \
  go run ./admin/cmd/admin
```

## 测试

```bash
make test
```

安装了 Docker 后可运行真实 MySQL 8.4 集成测试：

```bash
make test-integration
```

## 项目结构

```text
admin/   管理端 Go module
server/  核心服务 Go module
client/  Go 客户端 module
shared/  跨模块共享契约与基础类型 Go module
web/     前端 module
docs/    跨模块设计与项目文档
deploy/  开箱即用的数据库初始化文件
```

## 开发约定

- 通过小而清晰的提交记录项目演进。
- 不向仓库提交密钥、令牌或本地环境配置。
- `admin`、`server`、`client` 和 `shared` 独立声明依赖，根目录通过 `go.work` 提供本地协作体验。
- Go package 使用简短、清晰的小写名称。
- 引入新能力时同步补充测试和文档。
- Admin 只允许访问代码中注册了 Table Policy 的受管表；不得接受客户端提供的物理表名、列名或 SQL。
