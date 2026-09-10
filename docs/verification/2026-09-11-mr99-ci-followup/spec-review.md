# MR99 Web CI 跟进最终 Spec 增量审查

范围：固定基点 `dafdde3a4a41a2b8afd03ae57fa51b9908fae08b` 至当前两文件差异。`source-final.json` 中测试与 a11y SHA-256 分别与工作树的 `beaa0b41…e20c1b`、`2efb5cfb…95a0ef` 一致；`change-final.diff` 与工作树差异一致，`git diff --check` 通过。

## Findings

无。

- `CombinedQueryForm.test.tsx` 仅移除两个不受类型支持的 `exact` 选项。`getByRole` 的字符串 `name` 仍采用精确匹配；100 个输入仍逐项通过精确 label 触发真实 change，添加与查询仍触发真实 click。1～100 的有序全值断言、101 的明确 alert、仅一次 submit 和原 20 s 上限均保留，AC-017 未降弱。
- `browser-accessibility.cjs` 用明确 `table_name` URL 打开正式页面，并以新增按钮的 trial click 等待真实 actionability；随后同时核对 Managed Table 的实际 select 值，以及本页捕获的所有 table query 都是目标表确切路径且状态 200。它不会把错误表加载误作通过。
- 创建请求新增确切标题及唯一明细表断言。原键盘路径、raw CR 原请求、真实 MySQL 422、独立审批、冻结输入、复制转换，以及响应丢失后原 body/幂等键恢复均未删改。
- 未宣称此前错表来自 React；该增量只封闭已观察到的请求与 DOM 身份时序。未发现 scope 退化、临时兼容、flag、双写或生产行为变更。

## Residual risk

单文件 13 项、398 项与 typecheck 的最终结果已报告通过。正式三引擎 a11y 仍在执行，因此跨引擎运行结果不纳入本次只读结论。
