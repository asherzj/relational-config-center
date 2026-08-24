# Admin 交互 Demo

基于当前 Admin V1 `table_name + query/mutation strategy` 模型的零依赖交互原型。

- 侧边一级导航：平台管理、配置管理。
- 配置策略管理：按表名、策略、配置内容、一级 Mutation 能力、状态、操作人和时间范围查询，并查看、创建、替换、启用或停用 Table Policy。
- 配置内容管理：选择 enabled Managed Table，查询并按 Mutation Policy 新增、修改或删除内容。
- 所有数据仅保存在浏览器内存中，不请求真实 Admin API。

```bash
cd web
npm run prototype
```

打开 `http://127.0.0.1:4173/prototype/config-center/`。
