# #110 OS 信号窄增量 Spec 审查

**对象**：base `490a9e18017dff934c27a74dee0db3193a03fb8a` → tree `2c906bed92aaa90c58095ff1b4ebd857af8a6838`。

## Findings

未发现阻断本次**取证提交**的 Spec 缺口、越界或兼容性问题。

- 增量仅新增 PR #111 条件化临时 job、两个临时诊断脚本及不可变证据；正式八个 job 的既有内容未改。`scripts/browser-acceptance.sh` 和 `web/e2e/accounts.mjs` 与 base 均字节一致（SHA-256 分别 `cfafd6d6…19681`、`27596293…fcd9f`），原业务断言、权限、SQL、每 case 600 秒和正式 Browser gate 保留。
- 生成器 SHA `0248317e…794b6` 与已审 `5ac0d268` 版本相同；在共享真实服务内，每轮仍由原 runner 启动独立 Node/accounts profile，最多 12 轮，`set -e` 使首个失败停止。job 总上限 15 分钟，没有 force、重试失败项或延长原超时。
- wrapper SHA `ee0ce2ab…dc588` 从 Playwright 解析真实 WebKit 入口，以 `exec strace -f` 跟随原 launcher 进程树；仅记录 exit/wait/kill/tgkill 与 signals，不采 argv/env/read/write。EXIT trap 保留原状态，并核对入口 SHA、mode 和字节恢复；恢复失败会使原成功变失败，原失败码仍保留。GNU `stat` 仅用于 Ubuntu job，兼容边界明确。
- 33 项 `SHA256SUMS` 与 37 项 reviewed manifest 均逐项匹配 tree。真实 persistent WebKit trace 含 `WPEWebProcess`；受控 SIGABRT 记录 sender/wait，canary 未泄露；四种 0/42×恢复成功/失败路径符合预期。该阳性对照只证明诊断能力，不构成自然 crash 复现。

## Unresolved / gates

本地取证装置门禁通过；arm64 探针及受控信号不能解释 Linux amd64 的间歇性 page crash。提交后必须核验该临时 job 的真实入口 SHA/mode、每轮 21 checks/SQL/cleanup、首次失败 signal/exit 与生命周期；无失败或无信号只能记为有界/不确定结果，不能声称根因已修复。最终交付仍需删除临时 job、两脚本、生成 runner/cache wrapper，再由正式八项 CI 验收。
