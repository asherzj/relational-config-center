# Atomically replace the current policy

第一迭代中，每个 Table Policy 只有一份当前定义，并由使用流程保证同一时间只有一个人修改。Admin 先根据真实数据库元数据完整构造和校验候选 Query Policy 与 Mutation Policy，成功后再原子替换当前定义；失败时旧 Policy 保持生效。系统不保留 revision 历史、不提供乐观锁，并接受约束被违反时最后写入者获胜。

Policy 生命周期只有 `enabled` 和 `disabled` 两种状态，不引入 draft、发布、回滚或历史 revision。创建和原子替换仍由专用 Catalog API 完成。

新规则创建为 disabled；原子替换保持当前状态。启用前重新校验目标表、实时 Schema 和两类规则定义，禁用立即停止数据 API 授权。运行时校验失败只拒绝当前请求，不自动改变规则状态。

Policy 不绑定 Schema Fingerprint。每次执行都读取当前 MySQL 元数据；表结构变化自动进入后续请求，只有目标不再满足 Managed Table 的基本条件时才拒绝执行。

2026-09-11，#102 按[发布模板关联决策](0026-configure-release-workflows-with-stable-templates.md)为表规则管理加入并发版本及持久原请求重推；本文的最后写入者获胜约定由此替代，原子替换与启用时重新校验保留。
