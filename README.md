# 关系型配置中心

Relational Configuration Center 是一个面向实体、字段和关系建模的配置管理系统。
它旨在为具有 Schema、约束、引用和关联查询需求的配置数据提供统一管理能力，区别于以独立键值或配置文件为主要管理单元的传统配置中心。

## 当前状态

项目处于初始化阶段，已建立支持多个独立 Go module 和前端项目的 monorepo 骨架。

## 环境要求

- Go 1.27 或更高版本

## 本地运行

```bash
go run ./server/cmd/server
```

## 测试

```bash
make test
```

## 项目结构

```text
admin/   管理端 Go module
server/  核心服务 Go module
client/  Go 客户端 module
shared/  跨模块共享契约与基础类型 Go module
web/     前端 module
docs/    跨模块设计与项目文档
```

## 开发约定

- 通过小而清晰的提交记录项目演进。
- 不向仓库提交密钥、令牌或本地环境配置。
- `admin`、`server`、`client` 和 `shared` 独立声明依赖，根目录通过 `go.work` 提供本地协作体验。
- Go package 使用简短、清晰的小写名称。
- 引入新能力时同步补充测试和文档。
