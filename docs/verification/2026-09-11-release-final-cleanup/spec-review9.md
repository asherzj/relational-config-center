# Spec 增量审查 9

范围：`git diff fe42f4abc24c2e80b51c5d38f1b9cfe2d0abe6b9 2738eaf3507a484ee188cf96cc53939352243493`。`source-candidate9.json` SHA-256 已核对为 `f93096ea523a49768e28b3eb8368ca5f0c7162f3573ea19f36743bba17a68017`。

## Findings

无。

- `field-policies.cjs` 仅等待真实字段配置 dialog 隐藏后再检查入口焦点；原保存、重开、390px、错误输入与未知结果恢复路径均保留。
- `combined-query.cjs` 将请求与响应按 Playwright Request 对象关联，并在清空后明确等待 200、断言空 conditions；原两字段 AND、静态值加自定义 IN 及真实结果断言未删弱。
- `field-inputs.cjs` 明确取得两次独立 201 结果，以不同幂等键逐单取消，并断言两个 ID 不同；清理仅触及本脚本草稿，符合 AC-003 占用隔离。
- `field-display.cjs` 改用新建的 `field_display_browser_items` 历史夹具。Go 浏览器补充入口与 `scripts/browser-acceptance.sh` shell 浏览器验收入口加载同一 fixture；`stage1_acceptance_items` 整行守卫仍保留，避免发布历史用例改写共享 query/editor seed。
- `field-interactions.cjs` 每个 viewport 从空 policies 经真实 UI 配置；`finally` 仍只取消记录在 `drafts` 的自身草稿，并恢复进入脚本前的完整 policies 后 GET 等值核对。正常与失败路径均保留清理。

未发现 AC-006 大值容量/整图、AC-012 原正文与原请求恢复、AC-003 目标占用或旧 main 字段验收被删弱；无不当共享环境清理、临时兼容、flag、双写或范围扩展。

## Residual risk

审查时 `field19` 尚在运行，随后已由 root 核对 8/8、完整 seed/守卫/cleanup 通过；`all34` 与 Compose 仍不在本增量结论内。`account_browser_integration_test.go` 具有 `integration && browser` 标签；仅计划编译只能证明入口可编译，不能计入 335 项集成重跑。
