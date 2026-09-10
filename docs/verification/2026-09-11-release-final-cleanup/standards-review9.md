# Standards 增量复审 9

固定差异：`git diff fe42f4abc24c2e80b51c5d38f1b9cfe2d0abe6b9 2738eaf3507a484ee188cf96cc53939352243493`。来源快照 `source-candidate9.json`（727 文件）SHA-256：`f93096ea523a49768e28b3eb8368ca5f0c7162f3573ea19f36743bba17a68017`。

## Findings

- 成文规范违规：**0**
- 主观坏味道：**0**

`field-policies.cjs` 先等待精确命名的 dialog 实际关闭完成再检查入口焦点，避免在 dialog 尚未关闭时读取焦点。`combined-query.cjs` 在点击前建立真实响应等待，核对清空请求的 200 与空 conditions，并用 Playwright `Request` 对象关联请求和响应；捕获器不按预期正文筛选，因此证据没有自证循环。

`field-inputs.cjs` 直接等待两次真实 201，断言两个永久 ID 不同，再以各自新幂等键只取消本脚本创建的草稿，并核对 200/CANCELLED；没有全库清理。`field-interactions.cjs` 明确以空 policies 开始每段 UI 旅程，`finally` 仍只清理自身草稿并精确恢复进入脚本前的完整 policies。

新增 `field-display.sql` 使用专用表隔离会产生实际发布历史的脚本，避免修改共享查询/编辑 seed；Go `integration,browser` 入口和 shell 入口加载同一 fixture 文件，脚本也切到该表。两处入口枚举沿用仓库现有 fixture 装载方式，不构成本次主观 Duplicated Code。

`git diff --check` 无格式问题。生产源码与共享设计规则未变化。

## Residual risk

本次未运行测试或数据库。正在执行的 field19、后续 all34、Compose 及最终 Web 检查结果不属于本报告，不能预报为通过。
