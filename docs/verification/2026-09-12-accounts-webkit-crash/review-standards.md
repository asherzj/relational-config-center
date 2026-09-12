## Standards — #110 native diagnostic delta

审查对象：base `d4f12028732086769006cf7b55d7c19a2b0e036b`；冻结 manifest SHA-256 `c88ca3c9…ad07a`（16 项）。

- **Hard violations：0。** `.github/workflows/ci.yml:151-226` 对原八个 gate 仅给 Browser acceptance 增加 `DEBUG=pw:browser`；原三引擎、case 集、timeout、命令、artifact 和失败状态不变。新增 job 仅在 PR111 运行原 `accounts.mjs@webkit` 一次，不 force、不重试；EXIT trap kill/wait monitor，实测业务返回 42 时 step 仍返回 42。后续 `always()` 收集 dmesg 与 artifact，采集失败不覆盖原 case 结果。
- **Sampler boundary：符合。** `scripts/diagnose-accounts-webkit-native.cjs:1-40` 最多 900 次、约每秒一次；只读取限定 meminfo、cgroup memory 字段及 `ps` 的 PID/PPID/comm/RSS，单次 ps 500ms/1MiB。它不读取 argv、env、HTTP、Cookie 或进程内存；真实 argv canary 未进入样本。不可用读取显式记录，写入失败只影响独立 monitor。短峰值可能漏采，阴性样本不能排除 OOM。
- **Subjective smells：0。** 临时职责、范围和删除条件命名明确；未见 Mysterious Name、Duplicated Code、Feature Envy、Data Clumps、Primitive Obsession、Repeated Switches、Shotgun Surgery、Divergent Change、Speculative Generality、Message Chains、Middle Man 或 Refused Bequest。
- **Evidence integrity：通过。** 外层 16 项、内部 `SHA256SUMS` 12 项及 reviewed manifest 15 项逐项匹配；其中 unchanged `accounts.mjs` 从固定 base 核得 `27596293…fcd9f`。原 trace 证明 reviewer page 在 cleanup 前 crash，但未解释底层原因；本 delta 是取证设施，不能称修复 crash 或整体验收通过。
- **Diagnostic submission gate：通过；delivery blocker：1。** GitHub Linux amd64 原生取证尚待运行。#110 具备证据化解决且受影响回归通过后，必须在交付前移除专用 job、sampler 与临时 Browser `DEBUG`；保留不可变证据。

结论：本窄 delta 可提交用于 CI 取证；hard 0，subjective 0，原故障/整体验收未关闭。
