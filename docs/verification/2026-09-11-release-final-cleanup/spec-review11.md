# Spec 增量审查 11

范围：`git diff dd0e60fa1862538cc772a81a8b8b6667ed85bc9f e95958a841a827dddb27f0726eabb5f6c71f0bdb -- web/e2e/release-multitable.cjs`。`source-candidate11.json` SHA-256 已核对为 `7634288ed8875e6bb7ca3b625df1b500094e05c3179c8c9c4b28df9bb0dc3e9a`；`git diff --check` 通过。

## Findings

无。

- 故障注入从 `indexedDB` 实例赋值改为覆盖 `IDBFactory.prototype.open`，符合 browser23 诊断所揭示的“实例赋值未进入真实应用调用”问题。
- 真实发布单页面现在先要求 `__rccIDBOpenCalls > 0`，随后才等待“浏览器无法读取原发布请求”提示；最终还断言调用次数为正、实例解析出的 `open` 正是带标记的原型方法。因而 AC-012 的存储不可用路径不能再以未命中注入的正常 IndexedDB 行为误通过。
- 注入、调用或提示任一步失败仍进入诊断分支并重新抛错；诊断写入不会屏蔽失败。原零 PUT、失败前输入保留、页面可用以及最终 `errors=[]` 强断言均未改动。
- 未发现验收降弱、正常/失败路径删除、临时兼容、flag、双写或范围扩展。

## Residual risk

browser24 三引擎定向 runner 的 10 checks 与共同 guards 已由 root 核对通过。browser23 已证明旧注入未命中，但其表现本身不再代表候选 11。最终 `all25` 与 Compose11 尚未完成，不纳入本次只读增量结论；本次变化仅增强浏览器验收注入与证据，生产代码未变。
