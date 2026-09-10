# Spec 增量审查 7

最终候选 8 覆盖范围：`git diff 8b835f78aea0ad79df7d0ee5e7b4c1323e875dd4 fe42f4abc24c2e80b51c5d38f1b9cfe2d0abe6b9`。来源快照 `/private/tmp/rcc-88-final-verification/source-candidate8.json` 的 SHA-256 已核对为 `46cccbf10b2088918a2c0dc46780b9416805e1a45d13045c1955aa9943d8185e`。生产变更与候选 7 相同；候选 8 仅增加一行视口截图调用。

## Findings

无。

`ManagedTextInput.tsx:24` 只给真实多行控件增加 `max-h-[42dvh] overflow-y-auto`：限制可见高度并保留内部滚动，没有增加 `maxlength`、截断、转换或请求预算，符合 AC-006“不采用固定字节上限”与 AC-012 原正文保留要求。作用域限定在配置数据录入组件，没有扩展到其他 Textarea。

`web/e2e/release-multitable.cjs:168-173` 保留原 20 秒页面超时、真实 9 MiB 填写、正文/持久值/同 key 同 body/账号隔离断言；新增断言直接检查控件高度不超过 42dvh、`scrollHeight > height`，并保存几何和点击耗时诊断。耗时只作观察数据，未断言单次点击超时是唯一因果。`web/DESIGN.md` 同步记录最大 42dvh、内部滚动、全文和 CR 保护，和实现一致。

现有 RED 证据稳定证明 Firefox 控件高度约 4,194,338px；候选实现的三引擎证据均为 420px 且全文仍可滚动。未发现需求遗漏、验收降弱、范围扩展、临时兼容、flag 或双写。

候选 8 的新增 `shot(largePage, "large-text-editor.png")` 位于几何断言通过后、Change Set 操作前，仅在已配置输出目录时保存可视证据，不改变请求、超时、正文或恢复流程；无新增发现。三引擎 browser11 已各自完成 10 项检查并退出 0，且共同守卫和清理结果通过。

## Residual risk

最终 Web 四项、field 8、all 34 和 Compose 串行检查尚未完成；这是待完成验证，不是本增量的 Spec 缺口。已有单次 20 秒点击超时与另一次 4,625ms 成功只能作为诊断背景，不能据此声称唯一因果。
