# PR111 最终 Browser 原始证据核验

## 结论

**FAIL（正式 Browser 门禁未通过）。** CI run `34682606445`、job `103523926158` 在 PR merge ref `519b635`（合入 cleanup commit `490a9e18017dff934c27a74dee0db3193a03fb8a`）执行。`case-results.tsv` 有 17 个互异且均为 `selected` 的 case：16 个 exit 0，`accounts.mjs@webkit` exit 1；期望 34 项中的其余 17 项未执行，无多跑或重复。不得据此宣布最终交付或 Browser 全绿。

## 首个失败与 lifecycle

原始日志首错（lines 1897–1900）为 `locator.evaluate: Target page, context or browser has been closed`，定位 `web/e2e/accounts.mjs:205:72`：点击成员角色按钮后，对“管理 <member> 的角色”dialog 等待动画的 `evaluate`。WebKit lifecycle 精确记录：31,320ms、`phase=setup`、`event=page-crash`、`context=initial-persistent`、URL `/platform/account-roles`；31,322ms 才记录 `test-failure`，31,325ms 才 `cleanup-start`。因此这是测试过程中自然 page crash，不是 cleanup 诱发；原生根因仍未知。检索证据为首屏 25 项且有 next cursor，目标不在首屏，但搜索后唯一目标可见，故不能归因于遗漏分页搜索。

## 覆盖边界

- `browser-accessibility` 三引擎均 `ok=true`、7 checks、`pageErrors=[]`、2 次原 body/key POST、SQL row=1、cleanup remainingRows=0。
- Chromium/Firefox `accounts` 各完成 21 checks，lifecycle 无 `page-crash`/`test-failure` 或非预期断连。WebKit 未完成 21 checks，也未到发布/SQL验收。
- `release-multitable` 三引擎均未执行且无 result；critical 6 只有 accessibility 3 份成功，不能满足 critical6 门禁。
- `run.txt` exit 1，结尾 `cleanup verified: true`；`database-final.txt` 为基线汇总 `5|1|5|0|notification_page_query_v1|stage1_mutation_v1|1|DEPRECATED`。失败导致 `database-postcheck.txt`/`fixture-after.tsv` 未生成，因此只有全局清理恢复证据，没有完整成功态 SQL 后检。

## 原始证据

- artifact：`/tmp/rcc-111-final-browser-artifacts`，上传 digest `738b1f1561b934cc483dfef053ecb2bca74feb6d1a35bcf9c86a9f60b3142ad8`（ID `10294672015`）
- raw log：`/tmp/rcc-111-final-browser.log`，SHA-256 `d769cffb278f20649adc4998cb6ccd9d4773ca32b1d5bc4bc2538b6dab5ac9e6`
- `case-results.tsv` SHA-256 `b2a172662bef5b5afcc982eed75d75a64aa863ecca43776d9f57b9544faca0ea`
- WebKit lifecycle SHA-256 `551bcceeb9d948db5c5231db0b9213b23e81b41ea22d92aa8b53955e29ff6acb`

未重试、未改源码。
