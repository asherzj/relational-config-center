# Spec 增量审查 10

范围：`git diff 2738eaf3507a484ee188cf96cc53939352243493 dd0e60fa1862538cc772a81a8b8b6667ed85bc9f -- web/e2e/release-multitable.cjs`。`source-candidate10.json` SHA-256 已核对为 `faa18c3c6f2d65d27bd82d205f35b582386dc70e1face2919f2a46ebd86e8250`；`git diff --check` 通过。

## Findings

无。

- 新增 `reloadIdentity` 用于四个身份切换点：每次都先挂接真实 `GET /api/v1/auth/session` 响应等待，要求状态 200，再核对精确账号 ID、完整角色集合、可见的 ready workspace、用户名及账号入口中的角色文案。它没有用缓存证据替代真实 session/UI 屏障。
- AC-006/AC-012 证据未削弱：仍断言控件保留全文、持久化全文、请求体超过 9 MiB；丢响应后的刷新、撤权、换号期间业务 POST 保持 1 次，手动恢复后恰为 2 次，并保留原幂等键、原请求体摘要、同一发布单与版本断言。
- storage-unavailable 分支只在提示等待失败时写诊断，随后立即重新抛错；不会把失败转成通过。原提示、页面可用、零 PUT 与最终 `errors=[]` 断言均保留。
- 未发现正常或失败路径删除、临时兼容、flag、双写、旧接口残留或范围扩展。

## Residual risk

已知 browser21 的 storage 提示超时根因仍未确定；browser22 WebKit 的 10 checks、guards 已通过。`all34` 与 Compose6 尚在串行运行，不纳入本次只读增量结论。候选 10 仅增强验收证据，生产代码与候选 9 相同。
