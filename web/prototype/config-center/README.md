# Admin 交互 Demo

基于规则目录的零依赖交互原型，展示可复用查询/变更规则与表规则编码分配。

领域与交互以 [ADR-0016](../../../docs/adr/0016-separate-policy-definitions-from-table-assignments.md) 为准；不保留 ADR-0010 的内联 JSON 或每表权限模型。

- 侧边一级导航：平台管理、配置管理。
- 查询规则定义：使用强类型字段创建、完整替换和删除 Draft，验证并激活，弃用 Active，以及更新 Active/Deprecated 显示元数据；类型固定来自显式 `page_query` registry，不使用 JSON 编辑器。
- 变更规则定义：使用关系字段管理 ADD/MODIFY/DELETE 授权与四个可空 Auto Fill 目标槽位，并提供相同的受保护生命周期；类型固定来自实现三种操作的 `single_table_mutation` registry，不使用 JSON 编辑器。
- 表规则分配：按表名、两个规则编码、状态、操作人和时间范围查询；新建或替换只能从 Active 定义下拉选择，创建默认停用，替换保留启用状态。界面不再提供 JSON、每表 Mutation 权限或复制覆盖入口。
- 配置内容管理：选择 enabled Managed Table，查询并按变更规则新增、修改或删除内容。
- 所有数据仅保存在浏览器内存中，不请求真实 Admin API。

```bash
cd web
npm run prototype
```

打开 `http://127.0.0.1:4173/prototype/config-center/`。
