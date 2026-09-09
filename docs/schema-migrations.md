# RCC 控制表迁移维护

`schema-migrate` 使用固定版本 Goose 管理 RCC 控制表。它连接一个数据库，不启动 HTTP，不创建管理员或开发样例，不修改使用者的业务表。

当前交付包括 [T1 / #68](https://github.com/asherzj/relational-config-center/issues/68) 的新库初始化、后续升级、查询和显式恢复，以及 [T2 / #69](https://github.com/asherzj/relational-config-center/issues/69) 的严格存量接管；[T3 / #70](https://github.com/asherzj/relational-config-center/issues/70) 负责 Compose、镜像和 Admin 就绪门禁。完成 T3 前，现有 Compose 和测试仍使用旧初始化入口，不能声称部署已切换到 Goose。

## 构建与配置

在仓库根目录执行 `make build`，得到 `bin/admin/schema-migrate`；也可以在 `admin` 下执行 `go build -o ../bin/admin/schema-migrate ./cmd/schema-migrate`。

沿用 `LoadMySQL` 的 `MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_DATABASE`、`MYSQL_USER`、`MYSQL_PASSWORD`、`MYSQL_TLS_MODE` 及连接池、连接/读写超时配置。密码通过部署环境注入，不放进命令行、SQL 参数或工单。默认 TLS 开启，本地测试才显式关闭。HTTP 监听地址、会话及注册配置不是维护命令的前置条件。

维护连接需要读取目标库完整元数据、读写迁移账本及尝试记录，并具有本次迁移实际需要的控制表 DDL/DML 权限，包括外键所需权限。正常业务进程的连接权限及已有账号维护、Policy 维护、发布权限规则继续独立生效。

```sh
bin/admin/schema-migrate status
bin/admin/schema-migrate up --timeout=5m --lock-timeout=10s
bin/admin/schema-migrate baseline --timeout=5m --lock-timeout=10s
bin/admin/schema-migrate recover --timeout=5m --lock-timeout=10s
```

支持总超时 `0 < timeout <= 1h` 和锁等待 `0 < lock-timeout <= timeout`。错误退出码为 1，非法命令或参数为 2；成功输出 JSON。没有 `down`、`reset`、外部 SQL 路径、跳版本或强制成功参数。

## 查询与升级

`status` 使用只读事务，不调用会初始化账本的 Goose 查询 API。查询成功的退出码为 0，**不等于数据库就绪**；调用方必须检查状态。输出包含当前/要求版本、待执行版本，以及最后一次尝试的 ID、目标版本和内容摘要。

| 状态 | 含义与下一步 |
| --- | --- |
| `uninitialized` | 没有 RCC 控制结构；用 `up` 初始化。允许目标库已有使用者的业务表。 |
| `unmanaged` | 已有 RCC 控制表，但未接管；`up` 和 `recover` 都拒绝。按下节核对后显式运行 `baseline`，不手工登记正版本。 |
| `pending` | 受支持的已完成版本落后于此构建；用 `up` 继续。 |
| `current` | 本构建所需版本已记录，最后一次尝试已确认。T1 的状态查询仅报告账本；`up` 在锁内另行核查实际结构。Admin 的持续结构门禁由 T3 接入。 |
| `recovery_required` | 有未确认尝试或未完成的初始化元数据；普通 `up` 拒绝。按下节核查后显式恢复。 |
| `incompatible` | 超前、未知、缺失、重复、顺序错误的版本记录或非 InnoDB 版本账本；恢复不能绕过这一状态。使用对应发行版本并核查账本来源。 |

`up` 只执行未完成版本；执行前核查已完成结构及迁移文件摘要，执行后核查目标结构，确认尝试成功后才报告完成。开发 fixture 与生产迁移分开，账号授权和业务记录维护仍使用已有入口。

每次修改在 Goose 执行 SQL 的同一个 MySQL 会话上持有命名锁：`CONCAT('rcc.schema:', LEFT(SHA2(DATABASE(),256),48))`。名称在同一数据库稳定，不随构建版本变化。锁等待有界，失去这个连接也会停止使用它继续迁移。释放无法确认时弃用该连接，避免把持锁连接放回池中。即使连接池只有一个连接，迁移也可执行。

迁移锁只协调维护命令。正式升级仍需安排停写窗口、停止旧 Admin 和外部写入者，并按现有流程备份。它不能阻止任意数据库客户端直接执行 SQL。

## 校验并接管现有库

当前接管基线对应主分支 `8b5cd8592f0fbf02dcd27bb72a8a306fa371993d`，包含历史 013 的三类 Policy 审计列 `created_at` / `updated_at`。Goose `00001` 原始 SQL 和清单保持不变，新增 `00002_policy_audit_timestamps.sql` 以逐表原子改名承接这次结构变化。新安装依次执行这两个版本；已有 T1 版本库通过 `up` 应用 00002，保留审计值。

1. 按维护窗口停止 Admin 和外部写入者并备份，用 `status` 确认状态。未接管库的 `up` 不会创建迁移账本或静默登记。
2. 更旧库先按 [历史升级说明](../deploy/mysql/migrations/README.md) 执行合法的历史步骤：001～005、013、当前 `policy-migrate` 收缩，以及适用的 007～012。已执行过的脚本不应盲目重放；已有账号授权、维护基线和发布权限继续由原维护流程负责。
3. 对完整当前存量库显式运行 `baseline`。命令在 T1 的同一把锁内比较全部已知控制表的列、顺序、类型、NULL、默认值、索引、外键、CHECK 及其执行状态、引擎、字符集和排序规则，并检查认证控制锁的唯一 `id=1` 行。缺失或不兼容时非零退出、指出控制表并给历史升级指引；在通过校验前不创建账本、不改业务或控制数据。
4. 只有完整校验通过，才建立 `BASELINING` 尝试并通过 Goose Store 在一个 InnoDB 事务中登记全部基线版本；随后核查并确认 `BASELINED`。不会执行基线 SQL、历史回填或重置账号/会话、Policy、记录版本、发布历史、管理员权限与游标。已有业务表保持不变。
5. 再运行 `status`。重复 `baseline` 返回已确认的实际状态，不新增记录；已接管的旧版返回 `pending`，后续版本需显式 `up`。

定义比较只忽略 **表级 AUTO_INCREMENT 计数** 和 **描述性表注释**：历史 Policy 收缩没有设置表注释，已有自增计数反映保留的数据。这不是放宽默认值或约束；列默认值中的同名文本、列注释、CHECK 表达式与约束执行状态仍完整比较。基线不允许通过手工修改账本跳过结构校验。

## 失败与显式恢复

MySQL DDL 可以在失败前已经提交；不要据此假定整份迁移回滚。`rcc_goose_db_version` 记录实际成功版本，`rcc_schema_migration_attempts` 在开始 DDL 前保存 `RUNNING`、目标版本及迁移内容摘要，确认后变为 `SUCCEEDED`。接管使用同表中的 `BASELINING` / `BASELINED` 状态，JSON 的 `attempt_operation` 区分 `up` 与 `baseline`。失败保留尝试，不删除历史。

1. 停止后续部署，保存命令结果并运行 `status`；核对尝试 ID、目标版本、摘要和发行构建。
2. 检查数据库可用性、权限及实际控制表定义。`schema_mismatch` 会指出需要核查的控制表，诊断不输出 SQL、连接串或业务数据。
3. 修复连接/权限问题；结构不兼容时，依据已审阅的该版本迁移向前修复结构。无法证明现状符合恢复条件时，保留现场继续调查，不能通过删除账本、清空账号或填入正版本恢复。
4. 使用原迁移内容运行 `recover`。允许同一构建或仅追加新迁移、未改写原文件的后续构建。它只处理本次未确认尝试；还有新版本时，随后显式运行 `up`。
5. 再次查询，确认没有待处理/未确认状态后继续部署。

恢复区分以下情况：

- **DDL 部分完成，正版本尚未记录**：每张表必须完整匹配迁移前或目标定义；原有表不能缺失，只有本版新建表允许缺失并由可重入迁移补齐。00002 每张 Policy 表的两个改名在一条 ALTER 中原子执行，恢复只接受整张表处于旧定义或新定义。然后重试相同迁移；不同定义必须先由维护者核查修复。
- **正版本已记录，尝试尚未确认**：核查完整目标结构和必要控制元数据，只确认已有成功结果，不再执行该版本 SQL。确认仍失败时返回非零并保留 `recovery_required`。
- **接管登记或确认中断**：普通 `up` / `baseline` 都拒绝。`recover` 用原始内容摘要和完整目标结构核查后，原子登记尚未写入的完整前缀，或只确认已经保存的完整前缀；部分正版本、损坏结构或改写的原发行内容均拒绝。后续发行只能恢复原尝试，尚未执行的新版本继续等待 `up`。
- **首次初始化元数据中断**：如果尚无尝试记录且没有正版本，没有其他 RCC 控制结构时建立首次新装尝试；已有完整当前控制结构时，重新完整校验后建立接管尝试。任何不完整存量结构仍拒绝。如果版本表建好但为空，在已核查的尝试和同一把锁内，通过 Goose 的 Store 初始化零版本；零版本不代表任何控制迁移成功。

网络中断或超时后，即使客户端没有收到结果，也必须查询持久状态。连接不可用时返回安全错误，不推断数据库是否已完成。恢复不是通用修复器，也不自动改写未知账本或任意存量结构。

已确认尝试对应的整个版本账本丢失属于历史损坏，状态为 `incompatible`。应调查并按备份流程恢复原历史；`recover` 不会借用旧成功记录重建账本，也不会把已经确认的尝试重新当作未完成操作。

## 新增迁移

迁移及目标结构清单位于 `admin/internal/infrastructure/mysql/migrations/`，随维护二进制嵌入。第一版基线冻结自 `94a88058f5fbe6a54d3ad13d7e04c29a78b33b5d`，没有把历史一次性脚本改成 Goose 全量历史。

- 新增唯一、递增的编号，例如 `00003_description.sql`，使用 Goose `Up` 和 `NO TRANSACTION` 注解，显式指定 InnoDB、字符集及排序规则。不要新增 Down。
- 新版本必须能从已完成版本升级，也能从空库顺序执行。SQL 必须可在声明的恢复条件下安全重试。`IF NOT EXISTS` 不能替代实际结构检查；DML 不能在重放时覆盖已有业务状态。
- 同时提供 `00003_schema.json`，包含此版本全部 RCC 控制表的目标 `SHOW CREATE TABLE` 定义。使用真实 MySQL 8.4、UTC、utf8mb4 连接生成并审阅，与 SQL 独立检查。命令行 mysql 导出必须带 `--default-character-set=utf8mb4`，否则 CHECK 字面量的字符集会改变比较结果。
- 清单比较保留列类型、默认值、列注释、索引、约束、引擎、字符集/排序规则，仅忽略表级自增计数和描述性表注释。必要的认证控制锁必须保留唯一 `id=1` 行。后续改变必要控制元数据时，同时扩展验证及真实恢复测试。
- 已发布 SQL 和清单不可改写；SQL 与清单的前缀摘要绑定持久尝试。前向修复使用新版本，失败中的原版本则先恢复原内容并核查实际结构。
- 覆盖新安装、旧版升级结构等价、部分成功、进程中断、确认失败及数据保留。T1 的两份测试构建在临时目录追加下一版迁移，不向生产定义添加测试字段或运行时注入入口。

历史 `deploy/mysql/migrations/001`～`013` 及特殊 Policy 维护命令继续承担旧库升级职责。`deploy/mysql/init/001-schema.sql` 只是 T1 到 T3 之间的临时重复入口，删除责任归 #70；不得继续在两处独立演进新的当前结构。

参见 [ADR-0024](adr/0024-adopt-goose-at-a-verified-control-schema-baseline.md) 和 [完整规格](specs/goose-migrations/spec.md)。
