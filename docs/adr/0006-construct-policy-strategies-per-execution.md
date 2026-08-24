# Construct policy strategies from explicit registries per execution

Table Policy 通过稳定的 `query_policy` 和 `mutation_policy` 标识选择执行策略，并分别携带策略配置。Admin 使用两个显式 `map` 将策略标识映射到构造函数；每次数据操作都调用构造函数创建新的策略实例后执行，实例不跨请求复用，从而保留策略扩展点，同时隔离请求级可变状态。

## Consequences

- 持久化标识不能直接使用 Go 类型名，代码重命名不得改变已有 Policy 语义。
- 第一迭代不使用反射、动态插件或运行时注册；未知标识一律拒绝执行。
- 构造函数接收依赖和策略配置，返回新实例；请求与 `context.Context` 只传给 `Execute`，不通过可变 `Build` 方法保存在对象中。
- 策略依赖 application 定义的 Repository port，不能直接依赖 `infrastructure/mysql`。
- 每个策略使用强类型配置并拒绝未知 JSON 字段；创建、替换、启用和执行都复用同一条构造校验路径。
- 第一迭代不缓存 Table Policy，每次请求直接读取 Catalog，使原子替换和启停在提交后的下一个请求生效。
- `ChangeMatchFields` 和 `field_infos` 不进入新模型；表结构与字段类型来自实时 MySQL 元数据。

第一迭代只注册 `mysql_page_query_v1` 和 `mysql_single_table_mutation_v1`。前者配置默认排序及默认、最大页大小；后者配置 ADD、MODIFY、DELETE 开关和结构化 Auto Fill 规则。
