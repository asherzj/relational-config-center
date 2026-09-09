# RCC 控制表迁移维护

`schema-migrate` 使用固定版本 Goose 管理 RCC 控制表。它连接一个数据库，不启动 HTTP，不创建管理员或开发样例，不修改使用者的业务表。

当前交付范围是 [T1 / #68](https://github.com/asherzj/relational-config-center/issues/68)：新库初始化、后续升级、查询和显式恢复。[T2 / #69](https://github.com/asherzj/relational-config-center/issues/69) 负责校验接管现有库；[T3 / #70](https://github.com/asherzj/relational-config-center/issues/70) 负责 Compose、镜像和 Admin 就绪门禁。完成 T3 前，现有 Compose 和测试仍使用旧初始化入口，不能声称部署已切换到 Goose。

## 构建与配置

在仓库根目录执行 `make build`，得到 `bin/admin/schema-migrate`；也可以在 `admin` 下执行 `go build -o ../bin/admin/schema-migrate ./cmd/schema-migrate`。

沿用 `LoadMySQL` 的 `MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_DATABASE`、`MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_TLS_MODE` 及连接池、连接/读写超时配置。密码通过部署环境注入，不放进命令行、SQL 参数或工单。默认 TLS 开启，本地测试才显式关闭。HTTP 监听地址、会话及注册配置不是维护命令的前置条件。

维护连接需要读取目标库完整元数据、读写迁移账本及尝试记录，并具有本次迁移实际需要的控制表 DDL/DML 权限，包括外键所需权限。正常业务进程的连接权限及已有账号维护、Policy 维护、发布权限规则继续独立生效。

```sh
bin/admin/schema-migrate status
bin/admin/schema-migrate up --timeout=5m --lock-timeout=10s
bin/admin/schema-migrate recover --timeout=5m --lock-timeout=10s
```

支持总超时 `0 < timeout <= 1h` 和锁等待 `0 < lock-timeout <= timeout`。错误退出码为 1，非法命令或参数为 2；成功输出 JSON。没有 `down`、`reset`、外部 SQL 路径、跳版本或强制成功参数。

## 查询与升级

`status` 使用只读事务，不调用会初始化账本的 Goose 查询 API。查询成功的退出码为 0，**不等于数据库就绪**；调用方必须检查状态。输出包含当前/要求版本、待执行版本，以及最后一次尝试的 ID、目标版本和内容摘要。

| 状态 | 含义与下一步 |
| --- | --- |
| `uninitialized` | 没有 RCC 控制结构；用 `up` 初始化。允许目标库已有使用者的业务表。 |
| `unmanaged` | 已有 RCC 控制表，但未接管；`up` 和 `recover` 都拒绝。等待 T2 的严格接管入口，不手工登记正版本。 |
| `pending` | 受支持的已完成版本落后于此构建；用 `up` 继续。 |
| `current` | 本构建所需版本已记录，最后一次尝试已确认。T1 的状态查询仅报告账本；`up` 在锁内另行核查实际结构。Admin 的持续结构门禁由 T3 接入。 |
| `recovery_required` | 有未确认尝试或未完成的初始化元数据；普通 `up` 拒绝。按下节核查后显式恢复。 |
| `incompatible` | 超前、未知、缺失、重复或顺序错误的版本记录；恢复不能绕过这一状态。使用对应发行版本并核查账本来源。 |

`up` 只执行未完成版本；执行前核查已完成结构及迁移文件摘要，执行后核查目标结构，确认尝试成功后才报告完成。开发 fixture 与生产迁移分开，账号授权和业务记录维护仍使用已有入口。

每次修改在 Goose 执行 SQL 的同一个 MySQL 会话上持有命名锁：`CONCAT('rcc.schema:', LEFT(SHA2(DATABASE(),256),48))`。名称在同一数据库稳定，不随构建版本变化。锁等待有界，失去这个连接也会停止使用它继续迁移。释放无法确认时弃用该连接，避免把持锁连接放回池中。即使连接池只有一个连接，迁移也可执行。

迁移锁只协调维护命令。正式升级仍需安排停写窗口、停止旧 Admin 和外部写入者，并按现有流程备份。它不能阻止任意数据库客户端直接执行 SQL。

## 失败与显式恢复

MySQL DDL 可以在失败前已经提交；不要据此假定整份迁移回滚。`rcc_goose_db_version` 记录实际成功版本，`rcc_schema_migration_attempts` 在开始 DDL 前保存 `RUNNING`、目标版本及迁移内容摘要，确认后变为 `SUCCEEDED`。失败保留尝试，不删除历史。

1. 停止后续部署，保存命令结果并运行 `status`；核对尝试 ID、目标版本、摘要和发行构建。
2. 检查数据库可用性、权限及实际控制表定义。`schema_mismatch` 会指出需要核查的控制表，诊断不输出 SQL、连接串或业务数据。
3. 修复连接/权限问题；结构不兼容时，依据已审阅的该版本迁移向前修复结构。无法证明现状符合恢复条件时，保留现场继续调查，不能通过删除账本、清空账号或填入正版本恢复。
4. 使用原迁移内容运行 `recover`。允许同一构建或仅追加新迁移、未改写原文件的后续构建。它只处理本次未确认尝试；还有新版本时，随后显式运行 `up`。
5. 再次查询，确认没有待处理/未确认状态后继续部署。

恢复分三种情况：

- **DDL 部分完成，正版本尚未记录**：检查仍存在的每张控制表定义必须等于该版本的目标定义，缺失表允许由可重入迁移补齐。然后重试相同迁移；不同定义必须先由维护者核查修复。
- **正版本已记录，尝试尚未确认**：核查完整目标结构和必要控制元数据，只确认已有成功结果，不再执行该版本 SQL。确认仍失败时返回非零并保留 `recovery_required`。
- **首次初始化元数据中断**：如果尚无尝试记录，只在没有其他 RCC 控制结构且没有正版本的情况下允许建立首次尝试。如果版本表建好但为空，在已核查的尝试和同一把锁内，通过 Goose 的 Store 初始化零版本；零版本不代表任何控制迁移成功。

网络中断或超时后，即使客户端没有收到结果，也必须查询持久状态。连接不可用时返回安全错误，不推断数据库是否已完成。恢复不是通用修复器，也不自动改写未知账本或任意存量结构。

## 新增迁移

迁移及目标结构清单位于 `admin/internal/infrastructure/mysql/migrations/`，随维护二进制嵌入。第一版基线冻结自 `94a88058f5fbe6a54d3ad13d7e04c29a78b33b5d`，没有把历史一次性脚本改成 Goose 全量历史。

- 新增唯一、递增的编号，例如 `00002_description.sql`，使用 Goose `Up` 和 `NO TRANSACTION` 注解，显式指定 InnoDB、字符集及排序规则。不要新增 Down。
- 新版本必须能从已完成版本升级，也能从空库顺序执行。SQL 必须可在声明的恢复条件下安全重试。`IF NOT EXISTS` 不能替代实际结构检查；DML 不能在重放时覆盖已有业务状态。
- 同时提供 `00002_schema.json`，包含此版本全部 RCC 控制表的目标 `SHOW CREATE TABLE` 定义。使用真实 MySQL 8.4、UTC、utf8mb4 连接生成并审阅，与 SQL 独立检查。命令行 mysql 导出必须带 `--default-character-set=utf8mb4`，否则 CHECK 字面量的字符集会改变比较结果。
- 清单比较保留列类型、默认值、索引、约束、引擎、字符集/排序规则，仅忽略表级自增计数。必要的认证控制锁必须保留唯一 `id=1` 行。后续改变必要控制元数据时，同时扩展验证及真实恢复测试。
- 已发布 SQL 和清单不可改写；SQL 与清单的前缀摘要绑定持久尝试。前向修复使用新版本，失败中的原版本则先恢复原内容并核查实际结构。
- 覆盖新安装、旧版升级结构等价、部分成功、进程中断、确认失败及数据保留。T1 的两份测试构建在临时目录追加下一版迁移，不向生产定义添加测试字段或运行时注入入口。

历史 `deploy/mysql/migrations/001`～`012` 及特殊 Policy 维护命令继续承担旧库升级职责。`deploy/mysql/init/001-schema.sql` 只是 T1 到 T3 之间的临时重复入口，删除责任归 #70；不得继续在两处独立演进新的当前结构。

参见 [ADR-0024](adr/0024-adopt-goose-at-a-verified-control-schema-baseline.md) 和 [完整规格](specs/goose-migrations/spec.md)。
