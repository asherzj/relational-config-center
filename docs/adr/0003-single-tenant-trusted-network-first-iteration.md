# Keep the first iteration single-tenant and inside a trusted network

后续本地账号迭代对终端用户、共享 Token、免认证及固定 Operator 的设计调整见 [ADR-0017](./0017-introduce-local-accounts-in-the-trusted-network.md) 和 [ADR-0018](./0018-attribute-authored-changes-to-stable-account-ids.md)。下面保留第一迭代的原始决策；单组织、可信网络及其余未被替代的约束继续适用。

第一迭代的一个部署只服务一个组织或信任域，并仅运行在本机或可信内网；不在领域模型或 Query Spec 中隐式加入租户条件。功能范围保持克制，但运行形态仍包含超时、查询限制、连接池配置、结构化日志、健康检查、优雅停机和写操作日志，为后续扩大暴露面保留清晰边界。

## Consequences

- 第一迭代不承诺公网暴露、多租户隔离或高可用。
- Admin 使用部署级 Bearer Token，不建立终端用户、角色或 OIDC 模型；只有显式绑定到 loopback 时才能关闭 Token。
- 自动填充通过 `OperatorProvider` 获取操作人；第一迭代实现从部署配置返回固定值，为后续接入真实身份保留替换点。
- Web 与 Admin 在部署环境通过标准反向代理保持同源；开发环境只允许配置中明确列出的 CORS Origin。
- 第一迭代不区分普通列与敏感列；被 Query Policy 允许读取的值会按普通配置数据返回，系统不提供秘密遮蔽或 Secret Manager 能力。
- 第一迭代不建立与业务变更同事务提交的持久化审计表；操作日志不记录字段值，也不构成合规审计轨迹。
- 若未来引入公网访问或多租户，必须重新设计身份、授权、审计主体与数据隔离，而不是仅修改部署配置。
