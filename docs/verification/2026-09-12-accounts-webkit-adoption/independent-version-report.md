# #110 WebKit 1.63 / 2359 CI 对照核验

**对象**：commit `4b45aa1b3f16a9ca8401cc7aa0c887c751d0aae4`，CI `34684793011`，job `103529792215`。结论：**版本取证 gate PASS；这不是最终交付或精确根因修复证明。**

## 身份与恢复

原始 `comparison-identity.json` 确认实际加载 Playwright/core `1.63.0`，WebKit revision `2359`、browserVersion `26.6`，入口为 `webkit-2359/pw_run.sh`；log 也记录实际下载 2359。运行时 dependency patch SHA `06791c16…3254` 与冻结候选一致。package/lock 原始、候选、恢复后校验均 OK（原 SHA 分别 `14aea09f…f50e`、`00afda34…81ae`），before/after 校验文件相同，并有“backup removed; original dependency files restored”成功标记。browser launcher before/after SHA 均 `a85baad3…c63b`、mode 均 `755`；无 restoration 错误。cache 路径未上传，launcher 原备份删除仍是 supervisor 成功路径的间接证据。

## 12 轮业务与数据

`case-results.tsv` 含预期的 12 个重复诊断调用，全部 `accounts.mjs@webkit selected 0`；7 个矩阵外调用均 not-selected。每轮 runner 均输出完全相同的 21 项业务 checks，包含真实审批/发布、同正文与幂等键丢响应恢复、SQL最终字段、权限与账号隔离。12 份 lifecycle 均无 `page-crash`/`test-failure` 或非预期 disconnect；12 次发布确认诊断均 `failure=null`。

角色首屏从 4 增至 25：iteration 12 为 `first_page_count=25`、`first_page_has_next=true`，证明数据已越过 25 项边界；目标仍在首屏且 exact 搜索后唯一可见。`fixture-before.tsv` 与 `fixture-after.tsv` 字节一致；`database-postcheck.txt` 与 `database-final.txt` 均为基线汇总 `5|1|5|0|notification_page_query_v1|stage1_mutation_v1|1|DEPRECATED`。runner exit 0、cleanup verified。

## Native 边界与结论

共 36 个实际 launcher trace（每轮 initial/reviewer/reopened 三个）。36 个 Playwright process 均 `exitCode=0, signal=null`；逐行检查无非 `SIGCHLD` 信号、无 `kill/tgkill`、无非零 native exit。相较 2336 同类 ptrace job 首轮捕获 NULL `SIGSEGV`，本次结果支持“1.63/2359 整体版本在这 12 轮中未复现”的有界效果。两个 run 位于不同 GitHub VM，且 ptrace 改变时序，因此不能归因某个 WebKit commit，也不能排除更低频复发。

可据此准备正式依赖升级与临时 job/三脚本/runner-wrapper/活动 fixture 退出候选；最终仍须固定新候选、独立 review，并由无 ptrace 的正式八项 CI 验收。

- artifact：`/tmp/rcc-110-version-ci-artifacts`；digest `2e7d0fceaac5277883846cf7474756fa103c1276a43031b261d2d6e6e2440e2b`（ID `10294964597`）
- raw log：`/tmp/rcc-110-version-ci.log`，SHA-256 `1656936db9cf92ce414bfb32f8de0503466235dc0c2de05a14376626b0114097`
- case matrix SHA-256 `5051ff20a9fb3bc8d824132cea0383c6c8813e9ccac1b6f8235839deecb6c727`
- identity SHA-256 `9cca6b4fc0ec4ae18b424965c6581793b66d3d41ac3c9303ab9a43104308b844`
