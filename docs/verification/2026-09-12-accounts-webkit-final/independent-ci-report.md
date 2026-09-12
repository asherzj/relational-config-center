# #110 十二轮 native CI 核验

对象：run `34681965121`、HEAD `5ac0d268e51814973b977371270da2f351fccbda`、job `103522113478`。诊断步骤失败；内核收集和 artifact 上传成功。未重试。

## 首次失败与执行边界

`case-results.tsv` 仅有两次 selected accounts WebKit：iteration 1 exit 0，iteration 2 exit 1；后 10 轮未执行，符合首错停止。

- 第 1 轮输出完整 21 checks；生命周期只有预期 persistent-reopen 与 cleanup，第一次发布确认 508 ms 成功。首次账号页 4 行、无 next cursor，精确搜索命中。
- 第 2 轮没有完成 JSON/21 checks。在原 `accounts.mjs:439`，冲突重建后的第二次“确认发布到数据库”点击报告不稳定后 TargetClosed。第一次发布确认已于 13.448 s 完成（953 ms）；46.092 s 先写入 `page-crash`（`reopened-persistent`），随后同一毫秒写入 `test-failure`，46.096 s 才开始 cleanup。故这是自然页面崩溃，不是 cleanup、装置断言或分页失败。首次账号页仅 6 行、无 next cursor，搜索仍命中；本次未到 25 行边界。

全局 runner exit 1，cleanup verified true；`database-final.txt` 为预期基线，但失败中止使正式 `database-postcheck.txt`/fixture-after 未执行，不能声称 12 轮 SQL/cleanup 全通过。

## Native / 资源证据

两轮共 6 个 Playwright wrapper 最终均在测试清理时 graceful exit 0/signal null；stderr 没有 crash stack、assert 或异常信号，只有启动期 automation-context、libEGL/MESA/ZINK 告警。第 1 轮 deliberate reopen 附近另有一次 Fetch access-control 行，随后该轮完整通过，不能当作首错原因。

152 个 1 秒资源样本覆盖至失败后：MemAvailable 最低 12,575,348 KiB，swap 最低仍 3,145,700 KiB，cgroup `oom/oom_kill/oom_group_kill/max/high` 始终 0；崩溃前样本 cgroup current 5.57 GB。dmesg 无同期 OOM、kill 或 renderer crash，仅启动期平台/DRM 警告。采样不能排除瞬时或非 OOM 原生故障。

结论：有界循环成功复现自然故障并诚实失败，但没有得到底层退出信号或分页因果；12 轮与 25 行验证未完成，不得作为通过或根因修复证据。

原始日志 SHA-256 `361f9321…6a22`；case-results `ed37384f…f8dd`；iteration-2 lifecycle `29c1ac22…2d8c`；resources `d656ed52…458`；dmesg `fde64da8…e5bb`。
