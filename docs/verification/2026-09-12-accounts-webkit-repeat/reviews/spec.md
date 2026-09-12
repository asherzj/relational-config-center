# #110 十二轮原生诊断 — Spec 审查

对象：base `2ecfeef3e155a659358bb3a06d212f7f3fe63458`，不可变 tree `3e7a2652ea7c13b2f3bfeaae3a61a2f1786f0bee`。9 个变更路径与 diff 一致；`reviewed-manifest.json` 10 项逐一匹配 tree，内部 `SHA256SUMS` 通过。关键源码 SHA-256：workflow `c29567ad…7d48`，repeat generator `0248317e…94b6`。

## Findings / 当前提交门禁

未发现阻断“推送并执行有界诊断”的 Spec 问题。

- 生成器要求原 accounts 调用串恰好出现两次，并只替换这两处；漂移或错误目录会在输出前失败。当前 `release-workflow`/WebKit 选择仅进入其中一支固定 `{1..12}` 循环。
- 原 runner、`accounts.mjs`、900-sample monitor 均与 base 字节一致。MySQL/Admin/Web 与真实 fixture 在循环外只启动一次；每轮仍经 `run_browser_suite` 启动独立 Node，使用独立 `iteration-N` 输出，`accounts.mjs` 又在该目录创建独立 persistent profile。原 SQL、凭据、cleanup、权限及 600 秒 case 限制保留。
- 原 runner 的 `set -e` 使首个失败立即停止；合成检查证明第 3 轮 exit 42 时只执行 3 轮并返回 42，外层 monitor/临时文件清理失败也不掩盖 42。12 轮全通过才可能成功，15 分钟 job 截止导致的未完成不能算通过。
- 每个成功轮次新增两个账号；从首轮可见 4 个到末轮 26 个，设计足以跨越 25 行边界。它检验账号累积/分页假设，不把该状态差异当成崩溃原因。

## 限制与最终交付门禁

真实 Linux amd64 十二轮仍待 CI：须核对 12 个 selected/pass、独立 PID/profile/目录、末轮 `account-role-targeting.json` 的 25 行、`next_cursor` 与搜索命中，以及原生/lifecycle、SQL 和 cleanup 工件。若无自然失败，应按既定边界停止扩轮；根因未知不阻止已授权的诊断补强交付，也不得声称根因修复。

交付前仍须删除临时 diagnostic job、native sampler、repeat generator、生成副本及正式 Browser DEBUG，并对清理后的最终同源候选完成受影响回归与独立审查。当前报告仅放行诊断提交，不放行最终交付。
