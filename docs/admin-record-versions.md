# 配置记录并发版本

[#33](https://github.com/asherzj/relational-config-center/issues/33) 为所有现有 Managed Data 单行写入维护同一份 Record Version。它是正式发布的基线能力，T5 已把记录版本纳入完整发布事务并删除旧直写调用。

## HTTP 与使用流程

查询 `POST /api/v1/tables/:table_name/query` 保持 `columns`、`rows`、`page`，增加 `record_versions`。该字符串数组与 `rows` 逐项对应、长度相同；空页返回两个空数组。每条记录内容和版本在同一个只读 REPEATABLE READ 事务读取，版本不是业务列。

```json
{"columns":[{"name":"id","type":"uint64","nullable":false},{"name":"label","type":"string","nullable":false}],"rows":[{"id":"7","label":"原值"}],"record_versions":["9007199254740993"],"page":{"page_number":1,"page_size":20,"total_count":1,"total_pages":1}}
```

修改与删除通过发布草稿提交 `items[].expected_record_version`，接口与完整样例见 [发布草稿](admin-release-drafts.md)。已批准单再由 PUBLISHER 执行，返回实际 ID、发布后 record_version 和最终行，见 [发布结果契约](design-notes/publication-contract.md)。旧三条 rows 写路由已删除。

版本是规范十进制无符号 64 位字符串（含 `"0"`），不得转为 JavaScript Number。缺失、null 或空串返回 `422 record_version_required`；非法表示返回 `422 record_version_invalid`；JSON 数字为 `400 invalid_request`。旧版本返回 `409 record_version_conflict`，行不存在为 `404 mutation_row_not_found`。这些拒绝不写业务内容。

Web 固定初次读取的版本，冲突或登录恢复后保留输入和 Change Set。先“查看最新值”，再“基于最新值重建差异”，最后“确认并保存草稿”；随后独立审批与正式发布才生效。最新行删除或读取失败时不能重建。新增重复唯一键与记录版本冲突保持不同语义。

## 存储、身份与事务

`rcc_record_versions` 是唯一版本控制表，受 `rcc_*` 通用访问保护。主键为二进制表名和 32 字节记录身份摘要；业务表不增加版本列。缺失条目的存量行以 `0` 为初始版本，只读查询不创建控制行；首次写入按目标行取得数据库锁、原子建立控制条目，再按期望值条件推进。版本推进、业务数据、自动操作人/时间与规则快照共享同一写事务。不同记录不获取公共全局版本锁；正式发布另按表取得进度行锁，以保证该表版本及 Command 提交顺序。不同表仍独立。缺失/不兼容控制表阻止启动与就绪；业务写入只允许 InnoDB 表。

新增版本从 `1` 开始；修改或删除都推进一次，包括写回相同值。删除保留控制条目，同一 id 重建继续推进（例如 `1 → 删除 2 → 重建 3`）。事务约束或控制存储失败全部回滚；溢出不能绕回零或成功写入。控制条目永久保留，不提供自动清理或删除 API。

当前实际行身份先由 MySQL `WHERE id = ?` 定位，再使用存储主键的数据库比较权重摘要。数值及时间主键取数据库规范字符输出；FLOAT 先按真实 32 位精度解析并提升 DOUBLE 后无损输出，FLOAT/DOUBLE 的正负零按 MySQL 相等语义统一为同一权重；字符主键遵守 live collation，PAD SPACE 排序规则去除等价尾空格，NO PAD 保留。以 SHA-256 固定长度摘要容纳长主键；不使用小写化、客户端字符串或业务字段内容作为主键等价判断。

`record_versions.go` 的查询、修改、删除及插入后初始化仍以真实行定位。T3 #50 在同一身份能力中补齐缺行已知 ADD 的读取：依据 live 主键类型转换及同一排序规则权重产生身份，读取墓碑与整表维护基线，草稿/预览不初始化控制行。真实用例将缺行身份与插入、删除后的身份对照验证。`ReleaseOrderSession.ReadRecordBaseline` 向后续提交占用提供同一内部 RecordKey；HTTP 仅通过[草稿预览](admin-release-drafts.md)返回可明确重建的基线，不暴露内部 key。T4 复用此身份取得占用，T5 已把统一版本推进纳入发布事务，均不能另建发布专用行版本表。

## 升级与维护代际

首次升级必须停掉全部旧 Admin 及外部业务写入，备份业务表和控制表，执行 `deploy/mysql/migrations/009-record-versions.sql`，部署同时要求期望版本的 Admin/Web，再恢复服务。迁移可重跑，既有业务数据与控制版本不变；旧客户端缺版本会被拒绝。旧二进制不能与新版本同时写入，也不能回退旧二进制后继续写。

直接 SQL、其他应用写库、表重建和任意绕过版本推进的写入不在自动保护范围内。维护窗口内可用以下流程使所有旧令牌失效，同时保留历史位置：

1. 停止并排空所有读写请求和所有其他写库进程；备份业务与 `rcc_record_versions`，记录准确的数据库版本、主键定义和排序规则。
2. 为每张受影响表，在旧身份仍有效时取得该表全部控制条目的最大 `lock_version`（包含原维护基线；没有条目为 0）。确认未耗尽 unsigned BIGINT。
3. 将下面的 `@rcc_maintenance_table` 设置为准确物理表名并执行。空 `record_key` 是保留的整表维护基线，普通记录 key 始终为 32 字节，二者不能混用。

```sql
SET @rcc_maintenance_table = 'notification_templates';
START TRANSACTION;
SELECT COALESCE(MAX(lock_version), 0) + 1 INTO @rcc_next_floor
  FROM rcc_record_versions WHERE table_name = BINARY @rcc_maintenance_table;
INSERT INTO rcc_record_versions(table_name, record_key, lock_version)
  VALUES(BINARY @rcc_maintenance_table, X'', @rcc_next_floor)
  ON DUPLICATE KEY UPDATE lock_version = @rcc_next_floor;
COMMIT;
```

4. 执行计划中的表重建、外部数据修复、主键/排序规则调整或数据库升级；保留控制表和维护基线。更改表名必须把原表最大版本也带入新表基线，不能以改名获得 `0`。
5. 恢复前验证真实主键等价测试、查询/写入和依赖检查；当前记录有效版本为其控制版本与整表维护基线中的较大值，缺失新身份条目也从维护基线开始。原令牌都小于新基线，必须重新读取并确认。

MySQL 将 [WEIGHT_STRING](https://dev.mysql.com/doc/refman/8.4/en/string-functions.html#function_weight-string) 定义为内部调试函数，其行为可能随版本变化。因此 MySQL 版本、排序规则、字符集或身份算法变化均视为上述维护代际切换；禁止直接升级后静默产生新 key 并回到 `0`。本期经过验证的是 MySQL 8.4，不能仅据旧技术基线假定其他版本等价。中断恢复仍保持停写，读取已经提交的基线继续维护；重复提高基线安全，但绝不能降低或清空它。该流程没有在线或滚动升级承诺。


### T5 浮点身份修订的升级门禁

T5 修正旧 `CAST(FLOAT AS CHAR)` 的六位有效数截断：例如实际不同的 `1.2345670461654663` 与 `1.2345678806304932` 以前同为 `1.23457`，会碰撞到同一个旧记录 key。FLOAT/DOUBLE 的 `-0` 也必须与 `0` 共享主键身份和删除墓碑。本修订改变了受影响的持久 key 规则，**不得只换二进制后直接恢复请求**。

升级前，在旧 Admin 仍可用时取消每张 FLOAT 或 DOUBLE 主键表的全部 PENDING_APPROVAL / APPROVED 单据，确认目标占用已释放；旧 DRAFT 必须在升级后重新读取并明确重建。随后停止并排空所有读写进程，按上述维护流程为每张 FLOAT/DOUBLE 主键表写入严格大于其全部旧版本的维护基线，保留全部旧记录 key 和历史，再部署新 Admin/Web。不得删除旧 key、清空占用冒充取消或将版本降回 0。只含普通浮点业务列的表不改变记录身份，不因此要求更换 key。但旧草稿或批准单的 before 可能保存了旧短文本，升级后完整精度复核会拒绝这份旧快照；应显式重新采集当前值，取消并重建需要更新的发布申请，再独立审批，不能在后台改写旧冻结内容或旧历史。

真实验收覆盖 FLOAT 旧短文本碰撞 key 的版本 5 → 维护基线 6 → 新身份首次修改 7（另一真实主键仍 6），以及 DOUBLE 旧负零 key 的版本 5 → 基线 6 → 删除 7 → 正零重建 8；旧 key 版本 5 始终保留。未应用此维护流程的数据库不在本次升级支持范围；没有在线或滚动升级承诺。
