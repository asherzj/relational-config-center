# #110 Spec Review

对象：base `a306c94f484a5a4796f727bc4a37c192d9538bef`；冻结 manifest SHA256 `c643d7e0a2930339795e6450e755e6d324a5edf8b0fdc0e37604cd00e77c8634`（93 项）；唯一源码 `accounts.mjs` SHA256 `27596293…fcd9f`。Issue #110 正文与当前 1 条评论已完整读取。

## Findings

无阻塞 Spec finding，本地提交 gate 通过。

- AC-F01：证据保留 main CI 的 16 pass/1 fail/17 未执行、Linux/amd64 环境和原始 `element is not stable` 后 `TargetClosed`；未将旁支 transport/同步错误冒充原故障。
- AC-F02/F03：diff 只加诊断。原确认仍经 unchanged helper 调用原 `locator.click()`；无 force、重试、timeout 调整或断言删除。21 项原业务检查、持久 profile 重开、独立审批、权限/账号隔离、同正文/幂等键恢复及 SQL 路径保留。
- AC-F04：同源 final-three 中 Chromium、Firefox（macOS）及 Linux arm64 WebKit 各 21 checks，确认观测 852/958/971ms、无失败事件；3 个 case 均 exit 0，runner cleanup true，fixture 前后哈希相同。runner 的 unchanged 控制流仅在 database post-check 匹配后形成 exit 0。
- 10 轮真实 WebKit 窄循环完成 10/10、page errors/test failures 均 0；阳性对照故意 `page.close()` 后 exit 1，事件顺序为 confirmation page-close → 原 click failure → test-failure → cleanup，证明诊断保留失败和阶段。SHA256SUMS 90 项逐项通过。
- 无生产代码、权限、兼容层、Feature Flag 或退出项变化；临时 loop/close/transport 探针只在证据目录，不在最终 E2E 源码。

## Unresolved / 待补证

自然运行未复现自发 `TargetClosed`，底层根因仍未知；阳性对照仅证明已知关闭可被区分，采样也可能扰动时序。Linux WebKit 为 arm64 broker + macOS 后端，不能替代 GitHub Linux/amd64。以上限制与“诊断补强、非根因修复”范围一致，不阻塞本地 gate；正式 post-push CI 仍是交付待补证，当前不得宣称 CI gate 通过。
