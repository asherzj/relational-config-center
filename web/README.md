# Web

关系型配置中心的浏览器前端。正式应用使用 Vite、React、TypeScript、React Router、TanStack Query 和 Zod；`prototype/` 继续作为交互与视觉参考，但不参与正式应用运行。

## 当前能力

- 中文化应用壳和正式 URL 路由。
- 类型化 Admin API Client，HTTP DTO 只停留在 `src/api` 边界。
- Zod 运行时响应校验与稳定错误码映射。
- 查询策略的列表、详情、创建草稿、替换草稿、激活、弃用、更新元数据和删除草稿。
- 变更策略的 Type Registry、完整目录、草稿编辑、生命周期操作、元数据更新和四个固定 Auto Fill 槽位。
- Database Table Discovery 的真实表状态、稳定不兼容原因和 Table Policy 完整目录。
- 仅从兼容未分配表和 Active Policy 创建未启用分配，并支持详情、原子替换、启用、停用与客户端筛选。
- 替换 enabled Table Policy 前明确提示下一次请求立即生效；实时 Schema 与引用错误保留稳定错误码和 Request ID。
- 配置内容管理只列出 enabled Managed Table，以实时动态列构造全部八种 Query Spec 操作符、单字段排序和服务端分页。
- Managed Data 值保持 JSON String 语义，并在结果中明确区分 SQL NULL 与空字符串。
- Mutation Policy 驱动的 ADD、MODIFY、DELETE 始终显示能力状态；未授权、未知 Type、无效 Auto Fill 或不可执行 Policy Snapshot 均失败关闭。
- 通用写入编辑器以字段开关表达省略，并区分 NULL、空字符串和普通 JSON String；`id` 与全部 Auto Fill 字段不会进入写请求。
- ADD、MODIFY、DELETE 共用完整字段 Change Set；执行后 ADD/MODIFY 以 exact id 回查数据库最终值，DELETE 显示删除摘要，失败保留输入并展示稳定错误与 Request ID。
- 未知 Policy Type 或不完整的 Mutation Type 能力失败关闭，只允许安全查看或元数据更新。
- GET 仅对网络错误、503、504 自动重试一次；写命令不自动重试。

## 本地开发

Admin 默认运行在 `http://127.0.0.1:8080`。复制环境变量示例并填入部署级 Token：

```bash
cd web
cp .env.example .env.local
pnpm install
pnpm dev
```

浏览器只请求同源 `/api/v1`。Vite 开发代理读取 `RCC_ADMIN_URL` 和 `RCC_ADMIN_TOKEN`，并在代理层注入 `Authorization`；变量没有 `VITE_` 前缀，因此不会进入浏览器包。生产部署也应由同源反向代理持有 Token。

## 验证

```bash
pnpm typecheck
pnpm test:run
pnpm build
```

## 边界

- Web 不推断 generated、auto_increment、默认值或新增必填字段，Admin 仍以实时 Schema 做最终裁决。
- Change Set 不做提交前并发刷新；当前管理语义保持 last-write-wins。
