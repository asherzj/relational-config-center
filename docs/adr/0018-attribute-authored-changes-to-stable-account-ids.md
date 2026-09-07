# Attribute authored changes to stable account identities

本地账号迭代同时将配置行和规则目录写入的 Operator 改为已认证账号的永久 Account ID，以免显示名称或账号资料变化改变历史归属的解释。相比继续使用固定部署值或写入可变名称，这需要在所有写入口传递真实身份，但为后续身份接入和审计保留稳定的归因基础；界面可按需解析显示名称。

已有固定 Operator 值保留原样，不批量归属给某个新账号，也不把旧值强行解释为 Account ID。此决策替代 [ADR-0003](./0003-single-tenant-trusted-network-first-iteration.md) 的固定部署 Operator 约定，仅覆盖现有归因字段，持久审计记录仍属于后续工作包。Account ID 使用固定 36 个 ASCII 字符的小写、含连字符 UUID v4，永久不变且不复用；一般配置字段不自动解析账号名称。

若既有业务表的操作人字段无法完整保存新 Account ID，拒绝该写入并明确指出原因，由表维护者调整表结构；不截断标识，不回退为固定操作人，也不由 Admin 自动修改业务表结构。
