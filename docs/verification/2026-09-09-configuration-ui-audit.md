# 配置管理交互与规则审计字段验证

基线：`main` / `94a88058f5fbe6a54d3ad13d7e04c29a78b33b5d`。环境：macOS、MySQL 8.4。

## 交付行为

- 主导航拆分为平台人员管理、表配置管理、配置管理，顶部名称对应当前分组；人员管理仅管理员可见。
- 发布单列表右侧提供固定的“查看详情”入口；移除页头“编辑配置”和详情常驻“重新读取发布单”按钮，保留读取错误重试。
- 配置内容页的规则能力默认收起，从表选择器旁展开；切换表后收起，支持键盘操作。
- 三类 Policy Catalog 的数据库列和 HTTP 字段统一为 `created_at` / `updated_at`。新库定义、Web 解析和旧库维护命令同步更新。
- 迁移 013 只重命名审计列，先检查三张表再执行 DDL；旧新列并存或缺失时拒绝执行，支持已迁移及部分迁移状态。就绪检查会拒绝尚未迁移的旧库。

## 验证结果

| 验证 | 结果 |
| --- | --- |
| `make test` | Go 各模块通过 |
| `env -u RCC_ADMIN_TOKEN pnpm test:run`（`web/`） | 28 文件、330 项通过 |
| `env -u RCC_ADMIN_TOKEN pnpm build`（`web/`） | TypeScript 检查与 Vite 构建通过 |
| 隔离 MySQL：迁移、历史升级、规则生命周期 | 通过，111.034 秒 |
| 实际浏览器 | 桌面与 390px 导航、规则能力开关及切表收起、发布单详情入口跳转通过 |
| 本地升级与重启 | 备份后执行 013；前端、后端存活及就绪检查均为 HTTP 200；三类规则目录正常读取 |

MySQL 定向回归命令，在 `admin/` 下执行，Colima 环境需配置实际 Docker 地址：

```sh
go test -count=1 -timeout=15m -tags=integration ./cmd/admin \
  -run 'Test(PolicyAudit|PolicyCatalog|PolicyMigration|QueryPolicyHTTP|MutationPolicyHTTP|TablePolicyCode|TablePolicyCreation|LegacyPolicyPreflight|AccountUpgradeFromLegacy)'
```

本轮没有重跑完整三引擎浏览器套件或完整 MySQL 回归；上述定向验证不代表远端 CI 结果。

## 升级要求

存量部署先备份，停止旧 Admin 和 Policy 写入者，执行 [迁移 013](../../deploy/mysql/migrations/013-policy-audit-timestamps.sql)，再一起启动新版 Admin/Web。旧版服务不能访问改名后的审计列，具体顺序和回退方式见 [迁移说明](../../deploy/mysql/migrations/README.md#policy-审计时间统一013)。

本地开发库已保留原有账号、发布单和测试配置；数据库内容及凭据不属于代码交付。此次操作不代表已合并 main 或部署生产。
