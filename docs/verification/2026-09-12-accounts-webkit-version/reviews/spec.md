# #110 WebKit 1.63 临时对照 Spec 审查

**对象**：base `a221562036136942ff5cbbeace95a44d13532900` → tree `70579231454628eb5dacdd3c1ffb3a752d297e01`。

## Findings

未发现阻断本次**版本取证提交**的 Spec 问题。

- 正式 `web/package.json`、`pnpm-lock.yaml`、八个 CI job、accounts、browser runner、repeat generator 与 signal supervisor 均与 base 字节一致；增量只改 PR111 临时 job，并新增 version supervisor/历史证据。正式产品、业务断言、600 秒 case timeout 和 15 分钟诊断 job 上限未变。
- 固定 `candidate.patch`（SHA `06791c16…3254`）只把 Playwright/core `1.62.1→1.63.0`，并删除旧 Playwright 唯一引用的一个 Darwin 可选节点 `fsevents@2.3.2`；lock 中 package/snapshot 两段是同一节点，不能说成两个依赖。`fsevents@2.3.3` 及其他图保持不变。
- supervisor（SHA `e89afc74…b9a6`）先校验原 package/lock SHA、备份、应用精确 patch，再 frozen install；随后断言实际 Playwright/core 1.63.0、WebKit revision 2359/browserVersion 26.6 和 executablePath，才交给既审 signal wrapper 与原 12 轮首失败停止 runner。退出时恢复两文件、复核原 SHA并删除专属备份；既有失败码保留，成功后恢复失败会转为非零。
- 本地真实 candidate metadata 与五种 0/42、缺 backup、坏 candidate hash 路径通过；29 项 `SHA256SUMS`、36 项 reviewed manifest 均逐项匹配 tree。证据准确说明 `fsevents@2.3.2` 的孤立性，未把历史 SIGSEGV 或版本名当成修复证明。

## Unresolved / gates

本地门禁只证明补丁、依赖身份读取和恢复装置；本地未安装/运行 WebKit 2359。提交后必须由 Linux amd64 工件证明实际 identity、launcher SHA/mode、依赖 before/candidate/after、backup cleanup，以及每轮 21 checks/SQL/cleanup 或首个真实失败 signal。12 轮通过仅支持有界版本效果，不能归因具体 WebKit fix；失败/不确定不得重跑求绿。

最终交付仍须移除临时 job、三份诊断脚本、生成 runner/cache wrapper及活动 comparison fixture，恢复仅正式八 job，并完成正式 CI。tree workflow SHA `20ab9346…16bd`；README SHA `92a8fabc…8e1`。
