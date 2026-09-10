# Standards 增量审查 7

固定差异：`git diff 8b835f78aea0ad79df7d0ee5e7b4c1323e875dd4 a1e4570e71d134e887320d74f399485cbce2a825`。来源快照 `source-candidate7.json` SHA-256：`98abe7303f23f6f988ae6653f622a60d726c1744e855c6fce7e0ef7e855d4889`。

## Findings

无成文规范违规；无需要报告的主观坏味道。

`ManagedTextInput.tsx` 在已有共享多行输入组合中设置 `max-h-[42dvh] overflow-y-auto`，保留原有 `Textarea`、标签、错误关联、禁用、只读及 CR 原文保护语义。该职责位置符合 `web/DESIGN.md`“页面不得重新实现通用输入框外观”及抽屉内容独立滚动的规则。

`web/DESIGN.md` 同步记录多行编辑器随内容增长、42dvh 上限、内部纵向滚动及完整原值要求，符合 `docs/agents/design.md` 对共享设计规则同变更维护的要求。

`release-multitable.cjs` 读取真实 `getBoundingClientRect()`、`scrollHeight` 与 viewport 高度，断言控件受限且全文仍可滚动；没有镜像生产 CSS 类。原 20 秒页面超时、9 MiB 正文、捕获请求全文、持久值及同 key/同正文恢复断言均保留。新增诊断文件只记录浏览器事实，没有形成产品抽象或重复业务逻辑。

## Residual risk

`dvh` 与 `field-sizing-content` 的最终跨浏览器组合行为仍依赖正在收敛的真实浏览器结果。本次为只读 Standards 审查，未运行测试或数据库，也不宣称其余 Web、field8、all34 或 Compose 检查完成。

## Candidate 8 补核

增量：`git diff a1e4570e71d134e887320d74f399485cbce2a825 fe42f4abc24c2e80b51c5d38f1b9cfe2d0abe6b9`。来源快照 `source-candidate8.json` SHA-256：`46cccbf10b2088918a2c0dc46780b9416805e1a45d13045c1955aa9943d8185e`。

唯一变化是在大文本输入的真实高度与内部滚动断言之后，通过已有 `shot` helper 保存 viewport 截图。它不改变生产代码、页面状态、20 秒操作期限、9 MiB 全文断言或原请求恢复流程；证据采集位于被测状态建立并通过几何断言之后。**新增 0 项成文规范违规，0 项主观坏味道。**

协调者提供的 browser11 三引擎结果覆盖原 residual risk：每引擎 10 checks、exit 0、共同守卫与 cleanup 均通过，textarea 为 420px / 1000px viewport，9 MiB 全文恢复通过。本审查未亲自运行测试；最终 Web、field8、all34 与 Compose 串行检查仍在进行，不能据此宣称整体完成。
