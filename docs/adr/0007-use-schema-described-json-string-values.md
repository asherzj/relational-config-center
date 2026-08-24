# Use schema-described JSON string values at the HTTP boundary

动态表的 HTTP 请求和响应以 JSON String 表示实际字段值，并由响应中的列元数据以及实时 MySQL Schema 提供类型信息；SQL NULL 使用 JSON `null`，缺少字段表示不参与写入。这样可以统一处理 `BIGINT`、`DECIMAL` 和日期时间而不引入 JavaScript 精度损失，值仍必须在进入 Query Compiler 或 Repository 前按数据库字段类型严格解析。

第一迭代支持整数、`DECIMAL`、浮点、字符与文本、`ENUM`、`TINYINT(1)`、日期时间及 `JSON` 字段；不支持二进制、BLOB、BIT、SET 和空间类型。Query 因返回实时全量字段，会在表中存在任何不支持类型时失败；Mutation 只要求本次提交与 Auto Fill 涉及的字段类型受支持。

ADD 中缺少字段表示使用数据库默认行为，MODIFY 中缺少字段表示保持原值，JSON `null` 表示 SQL NULL，空字符串始终是实际空字符串。数据库会话使用 UTC；`DATE`、`TIME`、`DATETIME` 分别使用 MySQL 的规范字符串格式，`TIMESTAMP` 使用 RFC 3339 UTC。
