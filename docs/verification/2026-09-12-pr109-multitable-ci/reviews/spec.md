# PR109 multitable click diagnostics — Spec review

**结论：诊断能力 candidate 的 local commit gate 可通过；Findings 0。原间歇性故障仍未解决。**

## Findings（a–e）

未发现需求遗漏、证据缺口、越界、兼容/退出条件遗漏或可疑实现。

- **实现与失败语义：** `clickWithDiagnostics` 包裹的仍是原 `locator.click()`（`click-diagnostics.cjs:17-68`），未 force、重试、改 timeout 或放宽 actionability。setup、stop、dispose 各有 2 秒 deadline；采样/清理错误被记录，不能替换原 click 异常。多表脚本先持久化原 exception/checks/page errors，再以有界读取和每页 5 秒截图补证，最后重抛原异常（`release-multitable.cjs:214-232`）；原零 page-error 断言保留。
- **诊断范围：** 记录 rect、visibility、focus、surface animation 与 rAF gap，不含表单值、cookie、header 或凭据。100ms 布局/动画读取和持续 rAF 会扰动渲染时序，因此结果只能辅助诊断，不能代表未受观测的调度。
- **范围/兼容：** 仅新增测试 helper 并改一条既有 E2E click 及失败产物；产品、权限、共享业务 helper、timeout 与交互均未变，无临时生产兼容结构。

## 证据与映射

`source-manifest.json` 与完整 `manifest.json` SHA-256 均匹配给定值；2 个源码和 102 个证据文件逐项哈希通过，内部 101 项 `SHA256SUMS` 通过。`source.diff` 可从固定 base 精确重建两个冻结源码。

最终同源共享序列中 Chromium、Firefox、Linux WebKit 均 10/10，通过真实多表、独立审批、发布/回滚、390px、9MiB 同 key/order 恢复；各有 2 次被观测 click、`page_errors=[]`，runner exit 0 且 cleanup 通过。合成持续移动按钮使真实 click 原样抛 `TimeoutError`，并验证成功/失败及 setup/stop 异常后的 observer 清理与原异常保真；它不是原故障复现。基线 20 轮、40 次真实 WebKit click 均在 556–669ms，仅证明未复现。

## Unresolved / 必需待补证

原 CI 仅报告少数 `not stable` 检查后 20 秒超时；几何移动、后台/rAF 节流或 renderer/environment stall 仍未定因，candidate 不得称为修复。完整 34-case Linux amd64 post-push CI 仍必须运行；通过也只验证回归门禁，不能消除间歇性复发风险。
