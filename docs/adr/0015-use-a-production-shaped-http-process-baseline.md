# Use a production-shaped HTTP process baseline

> 本地账号交付由 [ADR-0017](./0017-introduce-local-accounts-in-the-trusted-network.md) 取代下述部署 Token、免认证和 CORS 约定；正常业务入口现在使用同源 Cookie 会话与 CSRF。Readiness 同时检查认证控制表的必要列、约束、索引与锁行，结构不完整拒绝启动并给出迁移指引；账号目录为空仍正常就绪。以下其余进程、超时、日志和优雅停机约定继续有效。

Admin 默认监听 `127.0.0.1:8080`，所有 `/api/v1` 请求经过部署级 Bearer Token、Request ID、1 MiB Body Limit、Recovery、精确 CORS Allowlist 和结构化访问日志；只有显式绑定 loopback 并配置关闭认证时才能省略 Token。HTTP Server 设置读取、写入、Header 与 Idle 超时，不使用无超时的便捷启动函数。

进程提供无认证的 liveness 和 readiness，后者只检查 MySQL 与 Policy Catalog 可用性，不受单个业务表或 Policy 状态影响。日志输出 JSON 到 stdout，禁止 Token、DSN、字段值、SQL 和绑定参数。配置、注册表、MySQL 或 Catalog 初始化失败会阻止启动；SIGINT/SIGTERM 触发最长 10 秒的优雅停机并最终关闭连接池。
