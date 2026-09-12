# #110: preserve evidence for the accounts WebKit confirmation failure

Base: `a306c94f484a5a4796f727bc4a37c192d9538bef`. Source candidate: `web/e2e/accounts.mjs`, SHA-256 `27596293b5ba49bcfb57fe535c787d0dc6359d10895d8f40e8e134acb65fcd9f`.

This change improves diagnosis of the intermittent confirmation failure. It does **not** establish or repair its original low-level cause. No application source, permissions, SQL, business assertion, browser/action timeout, shared click helper, or UI rule changes.

## Original failure and actual scope (AC-F01)

`formal-main/` preserves CI run `34677046038` attempt 1 and its original failure artifacts. The merged tree equals the earlier passing PR head; that green result did not exclude an intermittent failure. The browser matrix completed 16 invocations, then `accounts.mjs@webkit` failed; 17 invocations were not executed. In particular all three multitable invocations were still unexecuted.

The accounts invocation began at `06:12:27.4859165Z` and reported its error at `06:13:03.6043523Z`, about **36.1 seconds** later. The unchanged suite timeout is **600 seconds**. The log does not show that deadline firing, so an outer timeout kill is not established. The actual error is `locator.click: Target page, context or browser has been closed`, after repeated `element is not stable` observations on “确认发布到数据库”. The failure-state artifact contains only a URL and there is no failure screenshot. This shows that subsequent diagnostic reads did not yield their intended artifacts, without proving whether a renderer crash, browser disconnect, deliberate closure, or another failure caused that state.

## Feedback loop and evidence limits

The unmodified complete accounts scenario passed on native macOS WebKit: `native-baseline/`. Its 21 original checks, real database post-check, and runner cleanup passed. A passive observed version then passed the same complete journey on Linux WebKit: `linux-observed/`. At the original confirmation, the observed click completed in 633 ms. Its dialog's entering animation ended around 500 ms and its bounds then stabilized. This is a successful observation, not evidence that a persistent animation caused the CI failure.

The narrowed loop retains the real initial account registration and UI role edits, persistent profile close/reopen, separate reviewer browser, approval role, and 390px viewport. After the original publication completes, it creates exactly ten additional independent real release orders. Each iteration submits and independently approves a unique template record, navigates/reloads the approved order, opens the original confirmation, executes against real Admin/MySQL while losing the response, reloads, repeats the original request, asserts two equal bodies and idempotency keys, and explicitly completes the order. It stops on the first failure; it does not retry a failed iteration.

`loop-ten/` records **10/10** completed iterations and the remainder of the original accounts scenario passing. The ten confirmation clicks took 926–988 ms; no page errors or test failures were recorded. All original SQL/permission/isolation/recovery assertions remain in the archived probe. This bounded loop did **not** reproduce the spontaneous CI failure. Consequently the investigation did not proceed to a causal fix or claim a red/green repair of that failure.

`loop-incomplete-wait/` preserves an earlier failed diagnostic attempt. The added loop initially omitted the original script's response-and-dialog-close synchronization before comparing the two packets. It reported `1 !== 2` after a successful confirmation. The archived `accounts-loop-incomplete-wait.mjs` exposes the omission; the next probe copies the original synchronization. This fixture failure is separate from the CI failure and is not relabeled as a passing run.

The bounds/animation/rAF sampler can itself affect browser scheduling or layout timing. Neither the successful observed runs nor the bounded loop prove that instrumentation leaves the probability of the original intermittent failure unchanged.

## Diagnostic positive control

An explicit positive control closes the real publication page 100 ms after confirmation observation begins, during the existing opening animation. It produces `element is not stable` followed by `Target page, context or browser has been closed`, and the runner exits **1** as expected. `positive-control/` preserves that failed run; `probes/accounts-positive-control.mjs` contains the deliberate close.

The lifecycle log identifies a `page-close` during `publication-confirmation`, then the original click failure, then `test-failure`, then `cleanup-start` and the other contexts' deliberate closes. The existing sampler's stop/dispose failures remain diagnostic fields while the original click exception is rethrown. The checker verifies that ordering:

```sh
node docs/verification/2026-09-12-accounts-webkit/probes/check-positive-control.cjs docs/verification/2026-09-12-accounts-webkit/positive-control/accounts-lifecycle.jsonl
```

The checker passes. This proves the evidence path detects a **known injected closure**; it does not show why the original CI page closed. The later candidate additionally guards lifecycle/file-write failures so evidence collection cannot replace the business exception. The explicit close and loop are absent from final E2E source.

## Final change (AC-F02, AC-F03)

- Observe page crashes/closes, context closes and available browser disconnects, labeled by initial persistent context, reopened persistent context, or reviewer. Mark explicit persistent reopen and final cleanup separately from publication confirmation and failure-artifact collection.
- Append lifecycle evidence as it occurs, before final cleanup can obscure its order. Output write failures report an unavailable diagnostic and preserve the observed business error.
- Wrap only the original publication confirmation in the existing `clickWithDiagnostics`, with its original actionability checks and click timeout. Reuse the helper unchanged.
- Bound failure screenshot/evaluation by the existing 2-second **diagnostic** deadline, retain the original fallback failure-state format, and preserve the original business exception if writing that state fails.

The normal 21-check JSON output, request-body/key comparisons, role and account separation, SQL result checks, failure exit status, and runner cleanup remain intact. `web/DESIGN.md`, root/Admin context glossaries and the relevant account/publication ADRs were read. No shared visual/domain rule changed, so those documents need no update.

## Linux environment and transport calibration

All real runs use the normal disposable MySQL 8.4/Admin/Web runner. Chromium and Firefox run on macOS; Linux WebKit uses the cached official Playwright 1.62.1 image `mcr.microsoft.com/playwright@sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e` on arm64. GitHub's Linux amd64 runner remains a distinct environment boundary.

The diagnostic broker mounts this worktree read-only and exposes only random host-loopback ports. It calls the locked Playwright server implementation to launch a **real persistent profile**, expose its actual default context, and attach the same SOCKS loopback forwarding used by Playwright remote browsers. Closing a persistent context closes the real browser process normally; reopening uses the same profile directory. It does not simulate reopen using storage-state export/import. A local preflight observed a real default page, fetched a host-loopback HTTP page, closed/reopened the browser, and verified its persisted test cookie.

`transport-no-context/` and `transport-no-loopback/` preserve initial calibration failures before the accounts business path. The first launchServer mode did not expose the default context; the second lacked loopback forwarding for that already-created context. These are diagnostic-transport failures, not reproductions of the CI failure. Intermediate preflights also exposed close-interface and pnpm-resolution setup issues before the final successful preflight. No production dependency file or Playwright source was edited.

There are at most two owned containers at a time: one Linux browser broker and one runner-created MySQL container. The runner removes its named database container/volume after each attempt. The Linux broker and its temporary profiles are removed after final validation. Unrelated Docker resources and cached images are untouched.

## Commands and final verification (AC-F04)

The native baseline uses the unchanged case-selection interface:

```sh
DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock RCC_E2E_SUITE=release-workflow RCC_E2E_ENGINES=webkit RCC_E2E_CASES=accounts.mjs@webkit RCC_E2E_ARTIFACTS=/tmp/rcc-110-accounts-native-baseline bash scripts/browser-acceptance.sh
```

The observed snapshots in `probes/` were temporarily installed at the original `web/e2e/accounts.mjs` location, so all imports, credentials, maintenance executable, real fixtures and runner checks followed the original path. They are archived diagnostic inputs, not added CI cases. The final frozen source runs through the same runner, with a transport preload used only for Linux WebKit accounts:

```sh
DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock RCC_PLAYWRIGHT_PATH="$PWD/web/node_modules/playwright" NODE_OPTIONS="--require=$PWD/docs/verification/2026-09-12-accounts-webkit/probes/connect-linux-persistent.cjs" RCC_WEBKIT_BROKER=http://127.0.0.1:32788 RCC_WEBKIT_PERSISTENT_PORT=32789 RCC_WEBKIT_REVIEWER_PORT=32790 RCC_E2E_SUITE=release-workflow RCC_E2E_ENGINES=chromium,firefox,webkit RCC_E2E_CASES=accounts.mjs@chromium,accounts.mjs@firefox,accounts.mjs@webkit RCC_E2E_ARTIFACTS=/tmp/rcc-110-accounts-final-three bash scripts/browser-acceptance.sh
```

`final-three/` records the frozen-source run from `2026-09-12T06:36:59Z` to `06:38:31Z`, exit **0**, real database post-check passed, and cleanup verified. Chromium, Firefox and Linux WebKit each passed all **21** original checks. Their confirmation observations succeeded in **852 / 958 / 971 ms** respectively, with no crash/test-failure events. The source hash remained fixed throughout all three engines. The final WebKit 390px publication-result screenshot was inspected: the narrow page and locally scrollable data comparison remain usable. The owned Linux broker was then removed; its browser lifecycle stdout is retained in `linux-browser-lifecycle.log`. The preserved initial resolution error was a calibration issue described above, not part of these successful runs. `node --check web/e2e/accounts.mjs` and `git diff --check` pass. Independent Standards/Spec review and required CI remain delivery gates owned by the coordinating agent; no commit, push or external write has been performed by the diagnostic agent.
