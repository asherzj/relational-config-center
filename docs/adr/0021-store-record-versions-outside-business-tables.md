---
status: accepted
---

# Store continuous Record Versions outside business tables

为避免多人基于旧配置互相覆盖，同时保持 Admin 不修改业务表结构的边界，配置记录并发身份统一保存在受保护的 `rcc_record_versions`，查询内容和版本在同一快照读取，条件推进与业务写入在同一事务提交。缺失条目有明确初始基线；删除保留版本位置，重建不恢复旧令牌；普通写入仅竞争自己的目标记录，不使用全局写锁。

真实主键等价由 MySQL 存储值及其排序规则决定，不能由调用方字符串拼接代替。持久化身份使用比较权重摘要，这要求主键语义、数据库版本和身份算法改变时停写并推进整表维护基线，避免新身份重新使用旧令牌；具体维护与 HTTP 契约见 [记录版本](../admin-record-versions.md)。这是对 ADR-0016 受管记录 last-write-wins 的实际替代，规则目录的生命周期及并发语义保持原约定。
