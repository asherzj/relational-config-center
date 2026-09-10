# 多表实现前接入 main

在 #83 本地提交 `220b54479d13ae09d4c6a8a0f96c10aa6045fea3` 后，将 main `8b5cd8592f0fbf02dcd27bb72a8a306fa371993d` 接入隔离功能分支。20 个文件来自已合入的配置编辑交互改动；7 个重叠文件自动合并，没有文本冲突。保留查询结果的生成列标记、修改表单的完整可编辑字段、NULL 区别、导航顺序及草稿去向布局，同时保留 #83 的增量保存与目标占用。

合并后的验证：

- `make test`：全部 4 个 Go 模块通过，包含既有架构和路由契约检查；没有测试的包按实际输出记录。
- `pnpm --dir web test:run`：28 个文件、330 项全部通过。
- `pnpm --dir web build`：类型检查与生产构建通过。
- 真实浏览器 → Vite → Admin → 单个隔离 MySQL：账号主路径及 `release-drafts.cjs`、`release-batches.cjs`、`draft-targets.cjs` 全部通过，91.231 秒。覆盖 1000 条真实发布、实际 ID 和分页、增量草稿、占用冲突、两窗口输入保留及 390px 输入边界。

日志位于 `/private/tmp/rcc-main-integration-{go,web,build,browser}.log`。首次浏览器启动因验收输出目录未创建而退出，原失败保留在 `rcc-main-integration-browser-setup-failed.log`；补齐目录后按同一命令重新运行通过。测试数据库按串行策略创建和清理，没有操作既有开发数据库。

该合并仅作为 #84 的固定实现基点。#82、#83 和本次集成均只在本地提交；完整功能尚未完成，统一 MR 由全部工单验收后创建。
