# Admin 发布结果契约（#82 原单执行）

已批准单通过 `POST /api/v1/release-orders/:id/execute` 执行，正文为 `{"expected_version":"3"}`，带原 `Idempotency-Key`。当前永久账号必须具有 PUBLISHER（ADMIN 含该能力）；合法审批不因审批人后来撤权失效。业务行、记录版本、Command、表进度、普通发布单 SUCCEEDED/EXECUTE 历史、全部真实目标持续占用、通知、成功请求结果在同一事务中提交。原单回滚成功为 ROLLED_BACK 并释放目标；原单由人工 complete 后同样释放目标并关闭回滚窗口，详见[人工完结](../admin-release-upgrade.md#发布后的人工完结与快速回滚)。执行前重新核实冻结语义和基线；明确失败保持 APPROVED、版本和占用。COMMIT 错误一律报告提交结果未知；原键重试在当前鉴权后、状态版本检查前重放持久结果。

省略主键仅支持 AUTO_INCREMENT。非自增主键即使有 DEFAULT，也需在申请中显式提供 id；否则在准备前返回 publication_unsupported，不能用上次连接的 LAST_INSERT_ID 猜测实际记录。

三个旧 `POST /tables/:table/rows`、`PATCH /tables/:table/rows/:id`、`DELETE /tables/:table/rows/:id` 路由已删除。数据页确认保存草稿。同一路径接受同一数据源内多表合计 1～1,000 项混合明细；全部校验与提交保持原子性。

## 最终行

`PublicationResult` 包含 execution_id、kind、按物理表名索引的 table_versions / notifications、publisher_id、数据库 executed_at 和全局有序 commands。table_version / notification 暂为首项表的显示别名，由 #88 移除。execution_id 为原单 ID 与 PUBLICATION / ROLLBACK 种类组成的唯一执行身份。每个 Command 包含 order_id、execution_id、execution_kind、table_name、sequence、table_version、operation、实际 id、发布后的 record_version、before 和 final。所有版本、游标、ID 均为 JSON 字符串。ADD 的 before 是 absent tombstone；DELETE 的 final 是 tombstone，before 保存实际已删行。记录版本墓碑持续保留；重建不回到零。

`before` 与 `final` 的字段顺序为实际 Schema ordinal 顺序。类型使用 information_schema 的原始 COLUMN_TYPE（包括 unsigned、精度、enum 等），Schema digest 为已冻结完整执行定义的 SHA-256。当前数据库读取用显式 UTF-8 CAST 保留数值、零日期、带符号 TIME 时长和微秒；FLOAT 先提升 DOUBLE，避免 MySQL 默认六位有效数文本输出丢失真实位值，普通查询和草稿 before 同样保留实际浮点精度；TIMESTAMP 会话固定 UTC。真实主键从最终行（删除用 before）取得，TIMESTAMP 身份采用 UTC RFC3339 格式，空字符串主键有效。

编码对象按以下键序列生成紧凑 JSON：`format,schema_digest,deleted,fields,checksum`；每字段键序为 `name,type,encoding,value`。format 固定 `rcc-admin-mysql-row-v1`。UTF-8 JSON 按 Go encoding/json 的字符串转义规则生成（HTML 字符与 U+2028/U+2029 转义）；checksum 先设为空字符串后 SHA-256，输出小写十六进制。数据库 JSON 存储或 HTTP 展示不要求保留物理键序，解码后按本契约重编码再校验。此校验和验证一致性，不替代授权或真实性签名。

- `sql_null` 的 value 必须为 JSON null。
- `json` 的 value 是有效 JSON 文本，例如 JSON null 为字符串 `"null"`。
- `text` 的 value 是 UTF-8 字符串；普通文本 null 为 `"null"`，空串为 `""`，整数与 DECIMAL 不经应用层浮点中转；已存 FLOAT 在数据库内精确提升 DOUBLE 后输出，保留实际二进制值。
- `base64` 的 value 为标准补齐且规范的 Base64，固定样例可表示任意字节；当前业务 Schema 的 binary/blob/bit/geometry 等 unsupported 字段在草稿准备前拒绝，尚无这些字段的实时发布能力。
- deleted=true 的 fields 必须为空数组；存在的行必须至少有一个字段。缺失 value、重复字段名、非法 UTF-8/JSON/Base64 和不匹配的摘要/校验和均拒绝。

`admin/internal/domain/canonical_row_test.go` 有独立写定的字节样例（含字节 00 ff 80 41、SQL NULL、JSON null、字符串 null、空串）及固定 SHA-256 `de851d5c9c0e005044dd9906b21096e04b29ed8c65aa8161c6c87f630c181cff`。持久详情和原请求重放统一解码并调用 `ReleaseOrder.VerifyPublication`，从已保存的冻结 Schema 计算预期摘要，校验before/final的字段序列、原类型、Schema摘要与checksum；损坏数据明确不可用，不能重写配置来补偿。

`CanonicalRow.Verify(expectedSchemaDigest)` 需要调用方提供可信预期摘要，不应把未验证输入中的摘要当成信任来源。

[原单回滚](../admin-release-rollbacks.md) 根据原发布的 Command.before 与实际 ID/record_version 生成预览，在当前发布权限下确认一次，倒序执行并原子提交原单 ROLLED_BACK；无新单、新审批或必填原因。不能使用旧草稿预览 before，也不回填旧操作人/时间或生成列；最终实际业务值不匹配时整笔拒绝。before/final 均保存 Schema digest，历史解析核对格式和 Schema。

## 写入能力边界

部署账号需要已存在的 TRIGGER 元数据授权，并新增显式全局 PROCESS 权限，用于读取不受目标 schema SELECT 权限裁剪的 INNODB_FOREIGN/INNODB_TABLES。仅实际权限 errno 1044/1142/1227 返回 publication_metadata_permission；连接错误返回不可用，请求期限到达返回超时。数据库/schema/table 字典名当前仅接受 ASCII 字母、数字、下划线；执行连接必须开启 foreign_key_checks 与 unique_checks。

目标表上任何可能修改其他行的行为均拒绝：父表 incoming ON DELETE/UPDATE CASCADE 或 SET NULL，包括隐藏跨 schema 子表；任意 AFTER 触发器；多句触发器、存储过程/函数调用、跨表 SQL、用户变量。允许的触发器仅为单条 BEFORE `SET NEW.<业务字段> = <表达式>`：字段不能是 id、生成列或规则指定的自动操作人/时间。表达式只包含字面量、NEW/OLD 列、算术、括号及非限定的 CONCAT/LOWER/UPPER/COALESCE/IFNULL/ABS/ROUND；受长度及递归深度限制。未知语法拒绝。隐式业务 CHECK/唯一/FK RESTRICT 约束失败也整单回滚。

业务表 `SELECT id ... LIMIT 0 FOR UPDATE` 的元数据锁保持至事务结束，阻止并发 DDL 在核验后引入未追踪效果。普通 SELECT 同样持有元数据锁；这里不声称仅 FOR UPDATE 才有 MDL。已验证 MySQL 8.4 在添加外键时扩展父表锁，包括 foreign_key_checks=0 的在线 DDL。

依据：[元数据锁](https://dev.mysql.com/doc/refman/8.4/en/metadata-locking.html)、[外键元数据锁](https://dev.mysql.com/doc/refman/8.4/en/create-table-foreign-keys.html)、[全局 InnoDB 外键字典](https://dev.mysql.com/doc/refman/8.4/en/information-schema-innodb-foreign-table.html)、[非限定 builtin 与存储函数解析](https://dev.mysql.com/doc/refman/8.4/en/function-resolution.html)。

## 运行与恢复

升级在停写维护窗口依次应用 010、011、012、013、014；014 增加逐项明细和成功执行结构，不更改业务配置，也不转换旧发布单 JSON。FLOAT 主键精度及 FLOAT/DOUBLE 正负零权重修订属于身份代际切换，须先取消旧在途单并按[记录版本维护流程](../admin-record-versions.md#t5-浮点身份修订的升级门禁)推进整表维护基线；旧 key 与历史必须保留，不能静默切换到新 key 的版本 0。服务就绪检查验证全部发布控制表的 InnoDB、列类型与精确主键；旧客户端没有兼容直写开关。每表一条进度行在提交事务中加锁并推进 table_version/command_cursor，不使用全局自增顺序。独立表可独立提交。

正式进程的发布期限为 `min(8s, MYSQL_CONNECT_TIMEOUT, MYSQL_READ_TIMEOUT, MYSQL_WRITE_TIMEOUT) × 4/5`，短于 socket 超时；默认 socket 5 秒对应实际发布期限 4 秒。直接 HTTP 组合未传值时回退 8 秒。期限覆盖发布单的全部 POST/PUT 请求，提交前明确超时返回 mutation_timeout；提交确认不确定返回 release_result_unknown。页面保留原 actor/action/key/input，允许查询详情及原键重试，不能因查不到立即换键。成功仅表示数据库生效；notification.status 固定 NOT_CONNECTED，没有投递 worker、远端调用或客户端已收敛的承诺。

## 多表整单执行

基线与发布前/最终行通过带原请求序号的同表批量读取关联回明细；记录身份仍使用相同的 live 主键类型转换和 MySQL 比较权重，FLOAT 仍提升 DOUBLE，无应用字符串匹配或 SELECT 返回顺序假设。执行先重验全部冻结内容与基线，再在同一事务读取/锁定全部发布前行、执行各项、读取完整最终行、校验实际身份/目标占用并锁定比较整个版本集合，统一推进版本与持久化 Command。任何后续失败回滚先前的业务操作。

每次未知自增 ADD 使用该次 INSERT 协议返回的真实 `LastInsertId`，按无符号十进制字符串无损传递，不从一条多行 INSERT 的首编号推算整组编号。非单位 increment/offset、唯一约束导致的号段空洞均不能改变对应项的实际 id。实际最终 id 还须满足公开 Schema 的解析约束，无法再寻址的值（例如 TINYINT(1) 自增得到 2）使整单回滚。批量 Command 仍按明细顺序拥有各自游标，每个涉及表分别推进一个 Table Version 并保存一条刷新通知，结果数组仍按全局冻结顺序组装。

发布链路不再使用旧 64 KiB 字段及 8 MiB 整单/结果预算。接受明细的实际 POST/PUT 路由按 1,000 条总数限制，其他 API 和不接受明细的动作仍保留原 envelope 限制。内部 JSON 批量语句到达组装阈值时立即刷新批次，单个大明细仍可独立发送，各批次始终在同一事务，不构成新的整单容量门禁。

取得 MySQL 标量 `lower_case_table_names` 后按数据库名称语义统一表身份，再按稳定表名顺序取得所有 SHARE（草稿）或 UPDATE（执行）保护锁；在此之前不能访问 information_schema 或建立业务 RR 一致性快照。标量名称解析使用当前连接，支持连接池只有一个连接。规则点查同时保留可索引等值条件和二进制精确校验，避免借用 mode0 中另一张大小写表的规则。所有业务 DML 只遍历冻结的全局 Items，回滚遍历其严格逆序，不能按表重组或自动排序依赖。

发布失败的 item_index 使用正向全局位置，快速回滚失败使用恢复倒序位置。表名在明细中明确展示，前端按同一坐标定位。每个实际 Command 的 schema、表版本、游标、通知与记录身份都来自自身表。

## 原单、逐项结果与成功执行

业务存储为三类表：`rcc_release_orders` 保存申请身份、状态、审批历史和摘要；
`rcc_release_details` 按 `(order_id, position)` 保存每项申请及独立的发布、回滚
实际 Command；`rcc_release_executions` 按 `(order_id, kind)` 保存成功执行摘要，
每单最多发布、回滚各一次，摘要不内嵌完整结果。原始申请及原发布结果在回滚后保持。
详情外观仍组装 `items`、`publication`、`rollback` 和 `executions`，不再将完整
数组写回主单。列表仅读取有界摘要，不逐单读取或验证结果明细。

Command 仍属于供分发使用的技术记录，以执行身份区分同一原单的发布与回滚。
通知主键为 `(execution_id, table_name)`；每次执行在每个涉及表上保存一条通知，两次执行
不会互相覆盖。成功执行、明细实际值、技术记录及幂等结果和业务写入在一个事务中。

幂等快照保留当时流程与申请，但实际 Command 从不可变明细重建，避免另一份
完整结果。原发布键在回滚后重放当时 SUCCEEDED 响应，回滚键重放 ROLLED_BACK
响应；当前角色仍须通过鉴权。这个聚合读取与请求快照外观由 #88
继续收敛，完整退出条件见 [T1 过渡清单](multitable-release-tickets/t1-transitions.md)。


#86 的确认失败仅追加操作历史，失败历史和原请求摘要独立提交，不推进业务 CAS 或成功执行记录；未知 COMMIT 不写失败历史。错误响应的提交结论与历史可用性、原操作重推和本地日志清理边界详见[执行失败契约](../admin-release-approvals.md#执行失败与失败历史86)。
