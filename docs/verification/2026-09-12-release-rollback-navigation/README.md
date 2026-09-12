# Issue #112: release rollback navigation cancellation

Fixed base: `a306c94f484a5a4796f727bc4a37c192d9538bef`. Formal failed PR head: `a221562036136942ff5cbbeace95a44d13532900`. Both revisions contain the same original `web/e2e/release-rollbacks.cjs`, SHA-256 `81fdd57f2de0cda5b787400d8ae092f901b44414e9a0c15b9aa224e972e449b0`.

Final source candidate SHA-256: `2474e03eb60a292e6b5c76cc92cc428b7cacc807db250c72f33a557cfbff5d82`.

## AC-R01: formal failure and bounded red signal

`formal-ci/` preserves the complete downloaded CI artifact, raw Actions log and independent verification. Run 34683913900 / Browser job 103527409853 selected 16 of 34 invocations: 15 passed, `release-rollbacks.cjs@webkit` failed, and 18 never ran. The rollback case completed all seven business checks and then failed its unchanged final `errors` assertion with one page error: `/api/v1/approval-notifications due to access control checks.`. The event had no timestamp. There was no page-close, crash, process signal or ptrace evidence. This investigation does not merge it with Issue #110 and does not claim to reproduce the exact access-control wording locally.

`diagnostic-hard-linux/` is the issue-specific red feedback. It ran the actual rollback case against real Admin/MySQL and official Playwright 1.62.1 Linux arm64 WebKit. A temporary route fetched one real notification GET to completion, recorded HTTP 200, and held only browser delivery for five seconds. The existing hard `page.goto` from the rolled-back order to the competing order began 1,557 ms later. Six milliseconds after navigation started, WebKit emitted `Load request cancelled` for the held notification. The diagnostic assertion failed 1 != 0 after six original business checks. The database post-check and owned runner cleanup passed.

This controlled delay proves the old fixture can destroy a document while a successful notification read is active. It does not prove the browser's low-level wording mechanism or that CI had the same server latency.

## AC-R02: minimal source change

The only candidate source change replaces that hard document navigation with the real UI route:

1. click the existing exact `返回发布单列表` link;
2. find the table row containing the current competing order ID and click that row's exact business-title link;
3. wait for the exact target detail URL.

The order-ID row constraint is necessary because one shared three-engine runner retains prior engines' orders with the same title. It verifies the intended record without depending on list order.

No product source, authorization, SQL, timeouts, retries, force clicks, shared helpers, route semantics, seven business checks or final zero-page-error assertion changed. The real completion/quick rollback, independent approval, actual publication, response-loss same-body/same-key recovery, persisted preview, competing completion and final SQL checks remain intact. No shared domain or design rule changed, so `CONTEXT.md`, `CONTEXT-MAP.md`, ADRs and `web/DESIGN.md` need no update.

## Diagnostic green comparison

`diagnostic-link-linux/` ran the same five-second successful-response delay with only the UI-link navigation substituted. The list-to-detail navigation completed in 164 ms. The held notification stayed active and was delivered about 3,375 ms later; there were zero notification cancellations and zero page errors. All seven business checks, database post-check and cleanup passed.

The temporary event listeners, response delay, probe assertion and active preload were removed from the final E2E source. `probes/` archives their bounded method and the Linux connection preload as historical evidence only.

## AC-R03: final three-engine result and the retained first attempt

`final-three/` is intentionally retained as a failed first final attempt. Chromium completed seven checks. Firefox then found two prior/current records with the same title and failed Playwright strict mode before the seventh check; WebKit did not run. This was a selector defect introduced by the first navigation patch, not a rerun of the original page-error failure. Its raw runner log, case matrix, failure image/body, database post-check and cleanup remain frozen.

The only follow-up source change scopes the existing title link to the row containing `competingDraft.id`. `final-three-v2/` is the first complete run of that corrected frozen candidate. Chromium, Firefox and Linux arm64 WebKit each pass all seven original checks with `browser_errors: []`. Each engine confirms same-body/same-key request replay, actual publisher identity, real record version restoration, the real competing completion and zero rollback after completion. The runner exits 0; production Web build/typecheck, timeout-cleanup check, database post-check and owned MySQL cleanup pass.

The final Linux WebKit 390px rollback-result screenshot was inspected at original resolution. The page retains its compact header, rolled-back status, forward/restoration flows, approval record, horizontally contained change comparison, rollback reason and history without document-width overflow.

Chromium and Firefox ran natively on macOS arm64. WebKit ran in cached official image `mcr.microsoft.com/playwright:v1.62.1-noble` at digest `sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e`, Linux arm64, connected with loopback forwarding. GitHub's Linux amd64 runner remains a distinct final delivery boundary.

## Evidence index

- `ISSUE.md`: complete Issue #112 body and execution arrangement.
- `formal-ci/verification.md`: independent original result analysis.
- `formal-ci/raw.log`: original GitHub Actions raw log.
- `formal-ci/artifacts/`: complete original uploaded artifact tree.
- `diagnostic-hard-linux/release-workflow/rollback-webkit/rollback-navigation-probe.json`: red cancellation timeline.
- `diagnostic-link-linux/release-workflow/rollback-webkit/rollback-navigation-probe.json`: green delivery timeline.
- `final-three/release-workflow/rollback-firefox/runner.log`: retained first final selector failure.
- `final-three-v2/release-workflow/rollback-{chromium,firefox,webkit}/rollback-evidence.json`: final per-engine business and page-error results.
- `source.diff`: final source patch from the fixed base.
- `source-sha256.txt`: original and candidate source bindings.
- `tree.txt`: complete candidate tree inventory, including ignored raw logs.
- `SHA256SUMS`: hashes for all frozen evidence files except itself.

## Commands

The diagnostic runs temporarily installed the archived probe around the ordinary case and used the normal runner with one owned MySQL container. Final source validation used:

```sh
node --check web/e2e/release-rollbacks.cjs
git diff --check
```

The final browser command was:

```sh
DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock \
RCC_PLAYWRIGHT_PATH="$PWD/web/node_modules/playwright" \
NODE_OPTIONS=--require=/tmp/rcc-112-connect-linux-webkit.cjs \
RCC_WEBKIT_WS_ENDPOINT=ws://127.0.0.1:32792/ \
RCC_E2E_SUITE=release-workflow \
RCC_E2E_ENGINES=chromium,firefox,webkit \
RCC_E2E_CASES=release-rollbacks.cjs@chromium,release-rollbacks.cjs@firefox,release-rollbacks.cjs@webkit \
RCC_E2E_ARTIFACTS=docs/verification/2026-09-12-release-rollback-navigation/final-three-v2 \
bash scripts/browser-acceptance.sh
```

All test-created MySQL containers and volumes were removed by the runner. The separately owned Linux WebKit server is removed after evidence freezing. Unrelated Docker resources, the Colima VM, cached image and user data are unchanged.

`calibration-wrong-cli.txt` preserves the one transport calibration error: the first server command would have installed Playwright 1.63.0 through npx, so it was removed before any business run and recreated from the read-only locked 1.62.1 dependency. It is not counted as a product or acceptance result.
