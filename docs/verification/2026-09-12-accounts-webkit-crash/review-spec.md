# #110 原生诊断增量 — Spec 审查

审查对象：基点 `d4f12028732086769006cf7b55d7c19a2b0e036b`；冻结 manifest SHA-256 `c88ca3c9500c5f0a203df8792dcfd1cb5a70252424690d5ff917c15bfe3ad07a`（16 项）。我逐项校验内部 `SHA256SUMS`，并单独读取了未出现在 `source.diff` 的新增脚本；两份源码与 `source-manifest.json` 一致。基点 `accounts.mjs` 的 SHA-256 仍为 `27596293…fcd9f`。

## Findings

本轮“进入正式 Linux amd64 取证”的本地门禁无阻断项。

- Issue AC-F01/F03 所需的失败边界保留完整：PR #111 是 16 项通过后 `accounts.mjs@webkit` 失败；reviewer `page-crash`（40721 ms）先于 `test-failure`（40722 ms）和 cleanup（40882 ms）。证据只证明此次页面崩溃的顺序，没有推断自然根因或追认上一次失败同因。
- 原 Browser gate 的唯一行为差是 `DEBUG=pw:browser`；原用例序列、断言、600 秒 case 限制、失败退出及工件步骤均保留。新增 job 仅限 PR #111，单独运行原 `accounts.mjs@webkit`，没有 force、重试或替代完整套件。
- 监视器最多 900 次、约每秒一次，只采 meminfo、cgroup memory 与 PID/PPID/comm/RSS；不采 argv/env。读写失败被显式记录或忽略，但实测 case 退出 42 仍返回 42。语法、YAML、diff 与哈希校验通过；未见产品、权限或历史 97 路径越界。

## Unresolved / 必需待补证

自然 WebKit 崩溃根因仍未知，正式 Browser 仍失败；AC-F02/F03/F04 的修复、同源受影响回归及必需 CI 通过尚未满足。下一次 CI 必须分别审查正式 Browser 的 DEBUG 输出和临时 amd64 job 的资源、内核与 runner 工件；隔离 job 通过不能冲销完整套件失败。

该 job、采样脚本及正式 Browser 的 DEBUG 是有退出条件的临时兼容面：#110 获得有证据的修复并通过受影响回归后，必须在交付前一并删除，证据归档保留。
