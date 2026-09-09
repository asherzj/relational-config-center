# 隔离开发／测试库的旧发布单重置

`release-reset` 仅用于维护者明确指定的隔离 development/test 数据库。它删除发布历史与关联占用，让下一轮测试从当前业务数据开始；**不会撤销已发布的业务值**。这不是生产维护流程，也不是字段配置升级的前置步骤；Admin 启动、Ready、SQL 迁移和 HTTP 接口均不会调用它。

## 目标与停写前提

1. 确认这是可丢弃历史的隔离开发／测试实例，备份需要保留的数据。停止并排空全部 Admin、直接 SQL 写入、其他写库程序和后台重试；保持停写直到校验完成。停止测试客户端并丢弃其未完成请求／旧幂等键；重置会删除历史幂等结果，恢复旧 `create` 请求可能生成新发布单，不能将此操作当作网络错误恢复方式。
2. 独立核对实例地址、数据库名与 MySQL 实例 UUID。通过已核实目标的 MySQL 客户端执行只读查询 `SELECT DATABASE(), @@server_uuid;`，记录结果。UUID 用来避免连接配置指向另一实例时仍因同名数据库而误操作；它不自动证明实例属于开发环境。
3. 设置本次目标的标准 `MYSQL_*` 环境变量（见[部署说明](../README.md)）；密码沿用环境配置，不放入命令参数或校验记录。维护账号需目标五张清理表的 SELECT/DELETE、两张保留控制表的 SELECT、可见结构的权限、清理表上的直接 TRIGGER 权限及全局 PROCESS 权限。TRIGGER 权限只用于证明触发器完整可见，不会创建触发器；不依赖仅由角色继承的授权。维护命令不读取 HTTP／登录配置，也不创建或迁移控制表。
4. 构建并使用实际核对值执行下面命令。占位值必须替换为本次隔离目标；`--writers-stopped` 是维护者已完成停写的明确声明，命令无法代替运维排空所有外部写入。

```sh
make build
./bin/admin/release-reset \
  --environment=test \
  --target-address='127.0.0.1:替换为隔离实例端口' \
  --target-database='替换为已核实库名' \
  --target-server-uuid='替换为已核实实例UUID' \
  --writers-stopped
```

`--environment` 只接受 `development` 或 `test`。地址必须与加载的 `MYSQL_*` 配置生成的地址完全一致（TCP 为 HOST:PORT，socket 为路径）；数据库名既核对配置也核对实际 `DATABASE()`，UUID 核对当前连接的 `@@server_uuid`。缺失确认或任意目标不符均拒绝。

## 原子范围与校验结果

| 同一事务删除 | 完整保留 |
| --- | --- |
| `rcc_release_orders` 主单及 document 内的历史、关联 | 所有业务表的行与已发布值 |
| `rcc_release_requests` 全部历史幂等结果及未完成请求 | `rcc_record_versions` 全部记录 key、墓碑、空 key 维护代际 |
| `rcc_release_targets` 在途占用 | `rcc_table_publications` 的 table_name、table_version、command_cursor |
| `rcc_publication_commands` 发布命令 | 账号、会话、规则目录及字段配置 |
| `rcc_refresh_notifications` 通知记录 | 业务表自增位置 |

当前 Schema 无独立的发布历史表；关联内容在主单／幂等结果的 JSON 中。命令先持有上述七张控制表的元数据锁，在同一事务重新校验其受支持列、键和 InnoDB 引擎，拒绝清理表的触发器、任何入向或出向外键（包括其他 schema 的隐藏引用）或不足以核实它们的权限。结构不符应先调查目标，不能绕过校验或关闭外键检查。

命令使用事务性 `DELETE`，不使用 `TRUNCATE`、删表重建或重置自增。提交前核对五张表剩余行数为零，并对两张保留版本表按稳定顺序读取全部条目，比较前后行数与 SHA-256 摘要。标准输出的 JSON 包含 `environment`、`address`、`database`、`server_uuid`、`committed`、`before`、`after`、`preserved_before` 和 `preserved_after`。只有提交确认后输出 `committed: true`；检查五个 `after` 均为 0，保留表的两组摘要完全相同。该摘要核对的是控制版本，不是业务表内容备份；业务值保留由严格删除范围和无副作用结构保障，并在隔离集成中通过真实业务查询验证。

清理后的下一次正式发布仍使用已保留进度，继续增加 Table Version、Command Cursor 和 Record Version；历史命令缺口是有意清理的结果。该维护入口不能用来让运行时消费者从空日志恢复，Server/Client 分发恢复不在本步骤范围内。旧单不可再用于快速回滚或审批；新的业务修改必须重新建立草稿、审批并发布。

## 中断和重跑

命令处理 SIGINT/SIGTERM，并有五分钟总时限（连接读写超时仍按 MYSQL 配置）。提交前错误或中断会回滚整个事务；连接断开后 MySQL 完成回滚，不会留下只删了部分关联表的提交。若进程在提交附近中断、输出写入失败或提示“reset not confirmed”，结果可能已经提交：**继续保持停写，以同一经过核实的目标重新执行命令**。数据库恢复可用后，重跑删除剩余历史并重新输出校验；已清空时 `before`/`after` 均为零，版本／游标不会因此再变动。不要换目标猜测结果，不要恢复旧请求重试队列。

自动验收见 [AC-021 证据](verification/2026-09-09-release-reset.md)：只在本工单新建的 MySQL Testcontainers 上执行，覆盖目标／Schema 拒绝、真实发布后保留值与版本、事务中断回滚、重跑，以及下一次真实发布连续增长。
