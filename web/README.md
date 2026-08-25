# Web

关系型配置中心的浏览器前端。正式应用使用 Vite、React、TypeScript、React Router、TanStack Query 和 Zod；`prototype/` 继续作为交互与视觉参考，但不参与正式应用运行。

## 当前能力

- 中文化应用壳和正式 URL 路由。
- 类型化 Admin API Client，HTTP DTO 只停留在 `src/api` 边界。
- Zod 运行时响应校验与稳定错误码映射。
- 查询策略的列表、详情、创建草稿、替换草稿、激活、弃用、更新元数据和删除草稿。
- 变更策略的 Type Registry、完整目录、草稿编辑、生命周期操作、元数据更新和四个固定 Auto Fill 槽位。
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

## 后续边界

- 表策略列表在首版拉取完整 Catalog 后进行客户端筛选；Admin 当前不提供筛选或分页参数。
- 配置内容管理等待 Admin 提供完整 Schema 契约后再实现，不会硬编码动态表字段。
