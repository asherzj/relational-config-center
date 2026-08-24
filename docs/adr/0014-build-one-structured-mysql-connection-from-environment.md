# Build one structured MySQL connection from environment

Admin 只从环境变量读取一个 MySQL database 的主机、端口、库名、账号、密码、TLS 与连接池参数，并通过 go-sql-driver Config 构造 DSN；Policy、HTTP 和 Catalog 不能提供连接信息。程序固定启用 ParseTime、UTC、ClientFoundRows 和 utf8mb4，固定关闭 MultiStatements、InterpolateParams 与 AllowAllFiles，并为连接、读取和写入设置有限超时。

`database/sql` 使用默认 MaxOpenConns 10、MaxIdleConns 10、ConnMaxLifetime 3 分钟和 ConnMaxIdleTime 1 分钟，允许环境变量覆盖资源数值。全进程只创建一个连接池，Repository 不能自行打开连接。
