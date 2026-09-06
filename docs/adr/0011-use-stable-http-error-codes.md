# Use stable HTTP error codes without exposing database details

Admin 的错误响应使用统一的 `error.code`、安全的用户消息和 request ID，并稳定映射到 400、401、403、404、409、422、429、500、503 与 504。MySQL、GORM 和内部领域错误必须在接口边界转换，响应永远不包含 SQL、DSN、驱动原文或调用栈，使 Web 可以依赖错误代码而不与基础设施实现耦合。

本地账号限速使用 `429 auth_rate_limited` 与整数秒 `Retry-After`。账号/邮箱占用为 409；无效会话和统一登录失败为 401；来源或 CSRF 拒绝为 403。认证依赖不可用与超时分别为 503/504，保留 Cookie 供恢复后重新验证，不暴露数据库原文。
