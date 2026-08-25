---
status: superseded by ADR-0016
---

# Use one schema-driven single-table mutation strategy

> ADR-0016 retains the schema-driven ADD/MODIFY/DELETE behavior but supersedes
> the technology-specific identifier, per-table authorization, arbitrary JSON
> Auto Fill, literal sources, and separate per-mutation transactions described
> below. The final source is a relational Mutation Policy and the row change is
> part of the same Policy Snapshot transaction.

第一迭代使用稳定标识 `mysql_single_table_mutation_v1`。Table Policy 通过一级 `allow_add`、`allow_modify`、`allow_delete` 能力决定可执行的操作；三个能力默认禁止，不在 Mutation Strategy JSON 中重复保存。ADD 可写实时 Schema 中的非生成列并允许省略自增 `id`；MODIFY 使用 `id` 定位一行、采用 PATCH 语义且不能修改 `id`；DELETE 获得授权后只执行按 `id` 的硬删除。系统不保存 `ChangeMatchFields`、字段配置或应用层唯一键定义，数据库唯一索引是唯一性的最终裁决。

每次变更使用独立事务，MODIFY 和 DELETE 必须恰好影响一行。自动填充使用结构化规则，只支持 `operator`、`now` 和 `literal` 来源；第一迭代的 Operator 由部署配置提供固定值，服务端填充值覆盖客户端输入。

ADD 返回字符串形式的 `id`，MODIFY 和 DELETE 返回影响行数。重复唯一键由 Repository 映射为冲突错误；客户端值在 Repository 执行前按照实时 MySQL 字段类型严格解析。

MODIFY 不使用 ETag、版本列或原值比较，采用 last-write-wins；后到请求可以覆盖先前请求写入的相同字段。

ADD 和 MODIFY 只校验本次提交及 Auto Fill 产生的字段是否存在、可写并符合实时 Schema；新增但未提交的列由数据库默认值或 NULL 规则处理。DELETE 只校验目标表仍以 `id` 作为唯一主键。

MySQL Driver 启用 `ClientFoundRows`，使 MODIFY 把匹配行而非实际变化行作为影响行数；因此把字段写成原值仍成功，只有找不到 `id` 时返回零行。
