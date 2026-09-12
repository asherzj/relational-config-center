# PR #109 WebKit browser CI navigation race

Source under diagnosis: `ea36801375a9e888708c6e9c51caea45cff66be4` (detached worktree). The production bundle is unchanged by this repair. Only `web/e2e/browser-accessibility.cjs` adds a visible `Managed Table` wait between `page.reload()` and `repeatDraftSave(page)`.

## Failure and cause

The initial Linux CI completed all seven `browser-accessibility@webkit` business checks, then failed its unchanged `assert.deepEqual(pageErrors, [])`. The recorded message was `/127.0.0.1:36297/api/v1/auth/session due to access control checks.` The original failed result, log and screenshots are in `ci-initial-webkit/`. That artifact lacks event timestamps and the underlying error stack; the timing explanation below comes from a separately reproduced equivalent navigation sequence, not an invented reconstruction of that CI run.

`page.reload()` waits for document load. It does not wait for the React effect that verifies the session and mounts the authenticated workspace. The next line calls `repeatDraftSave`, whose `reopenDraftSave` immediately performs another `page.goto('/configuration/release-orders')`. The pending initial React effect can therefore start a same-origin fetch after the second navigation's `beforeunload`. WebKit reports that request as an access-control error even though the app catches the fetch rejection. Playwright 1.62.1's WebKit console handling classifies JavaScript-source error messages as `pageerror` and splits the message at its first colon, producing the CI message beginning `/127...`.

The exact 390px probe serves this commit's real production bundle, supplies a valid synthetic account and table-policy response, loads the managed-data workspace, reloads, and navigates to the draft catalog. Its only before/after difference is whether the same `Managed Table` control is awaited after reload. It records WebKit page errors, native `window.error` / `unhandledrejection`, lifecycle events, and API paths; it does not suppress any errors.

| Exact probe | Iterations completed | Playwright page errors | Native uncaught errors | Result |
| --- | ---: | ---: | ---: | --- |
| macOS WebKit before | 20 | 8, all auth/session | 0 | assertion failed |
| macOS WebKit after | 20 | 0 | 0 | passed |
| Linux arm64 WebKit before | 20 | 15: 11 auth/session, 2 table-policies, 2 approval-notifications | 0 | assertion failed |
| Linux arm64 WebKit after | 20 | 0 | 0 | passed |

Raw results and stdout are `probes/{macos,linux}-{before,after}.{json,log}`. The Linux before timeline places the access-control reports after `beforeunload` and before `pagehide`; session and subsequent initial workspace reads can both hit this window. The fixture is intentionally limited to navigation and is not a replacement for real backend acceptance.

Earlier minimization is retained as `probes/*coarse*` plus `coarse-before.cjs` / `coarse-after.cjs`: the unguarded macOS coarse loop produced 36 page errors; the Linux coarse loop produced four matching errors and then terminated with `WebKit encountered an internal error`, **not** the final zero-errors assertion. The guarded coarse loops completed with zero errors. The later exact probe above completed all 20 iterations on both platforms. A still earlier 10-iteration plain-HTML caught-fetch cancellation probe produced only `requestfailed: cancelled` on macOS, so cancellation of an already-started request alone did not reproduce the symptom.

## Repair and actual backend verification

The repair waits for the restored, visible business control before invoking the original draft recovery action. It retains the reload, the original action, all body/idempotency-key checks, real MySQL assertions, and the final zero-page-errors assertion. It changes no timeout value, CORS policy, application code, shared helper, or error whitelist. `web/DESIGN.md` requires no update because no product design or interaction changes.

Three-engine attempt (`fixed-three-engine-attempt/`): Chromium and Firefox each passed their complete original business suite. Linux remote WebKit passed five checks but stopped earlier than the repaired line on `POST /api/v1/release-orders/6f36824360a2902dd9768670397e78ae/approve` with HTTP 403 `permission_denied`, request ID `rcc-9550e7647010524c43979fdae12e3beb`. This attempt is **failed**, not an all-engine pass.

The preserved Admin tail confirms successful role creation (201), table approval-role assignment (200), submission (200), reviewer detail read (200), and then approval (403). Its normalized paths and response metadata do not retain actor/member IDs or the review response body. They cannot prove the cause of the qualification failure. A shared-fixture interaction is possible but unconfirmed; neither a product fix nor a fixture fix for this 403 is included. The formal shared-fixture Linux CI must verify this path again.

A subsequent independent, newly created MySQL fixture ran the entire repaired WebKit suite through Linux WebKit and passed (`fixed-linux-webkit/`):

- Seven business checks, zero page errors, zero leaked fixture rows.
- The injected 503 followed a real 201 draft commit; recovery issued exactly two matching original body/key requests.
- The recovered draft was submitted, approved by the separate permanent reviewer, published and completed through the actual Admin; SQL verified one published row.
- Database post-check and fixture comparison passed, and the runner recorded `cleanup verified: true`.
- `result.json`, `http-evidence.json`, runner log and 320px / 390px screenshots are retained. The 390px screenshot shows the recovered browser draft view; later API-driven publication is proved by the result/SQL checks, not by that unrefreshed screenshot.

The original unmodified macOS WebKit suite also passed once (`macos-original-baseline/`), demonstrating why a single clean local run was insufficient to diagnose the CI race. The repaired build/typecheck completed inside both actual-backend runners. No unrelated unit suite was rerun for this test-only synchronization change.

## Reproduction and transport boundary

From the repository root with the pinned dependencies and production build present:

```sh
RCC_PROBE_READY=0 RCC_PROBE_OUTPUT=/tmp/rcc-navigation-before.json node docs/verification/2026-09-12-pr109-webkit-ci/probes/navigation-race.cjs
RCC_PROBE_READY=1 RCC_PROBE_OUTPUT=/tmp/rcc-navigation-after.json node docs/verification/2026-09-12-pr109-webkit-ci/probes/navigation-race.cjs
```

The first command is expected to exit 1 when the race occurs; the second expects zero page errors. Both ran to completion for the retained exact results. The browser engine remains Playwright WebKit, not installed Safari.

Linux used the official `mcr.microsoft.com/playwright:v1.62.1-noble` image, digest `sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e`, on Colima Linux arm64. The exact probes used a read-only repository mount and `--network none`; their local HTTP server and browser ran inside the same container. This differs from GitHub's Linux amd64 runner architecture.

For actual backend verification, a disposable container exposed Playwright `run-server` on a random host-loopback port. `probes/connect-linux-webkit.cjs` documents the temporary client preload used in that run: only the WebKit launch was redirected to `webkit.connect(..., { exposeNetwork: '<loopback>' })`. The original suite, account maintenance tool, production Web proxy, Admin, SQL commands and assertions stayed unchanged. The backend/client ran on macOS while WebKit ran on Linux; this is not a claim of a fully Linux-hosted CI run. The [official Docker guidance](https://playwright.dev/docs/docker) requires matching Playwright package and image versions; both were 1.62.1.

Recorded runner invocations:

```sh
# Original macOS baseline
RCC_E2E_SUITE=browser-accessibility RCC_E2E_ENGINES=webkit RCC_E2E_ARTIFACTS=/tmp/rcc-pr109-webkit-baseline bash scripts/browser-acceptance.sh
# Failed combined attempt; Chromium/Firefox local, WebKit remote Linux
NODE_OPTIONS='--require=/tmp/rcc-pr109-connect-linux-webkit.cjs' RCC_WEBKIT_WS_ENDPOINT=ws://127.0.0.1:32768/ RCC_E2E_SUITE=browser-accessibility RCC_E2E_ENGINES=chromium,firefox,webkit RCC_E2E_ARTIFACTS=/tmp/rcc-pr109-webkit-fixed-three-engines bash scripts/browser-acceptance.sh
# Successful isolated Linux WebKit attempt
NODE_OPTIONS='--require=/tmp/rcc-pr109-connect-linux-webkit.cjs' RCC_WEBKIT_WS_ENDPOINT=ws://127.0.0.1:32768/ RCC_E2E_SUITE=browser-accessibility RCC_E2E_ENGINES=webkit RCC_E2E_ARTIFACTS=/tmp/rcc-pr109-webkit-fixed-linux-only bash scripts/browser-acceptance.sh
```

## Cleanup and remaining coverage

At most one self-created MySQL ran at a time; every runner removed its random container and named volume. The remote Playwright container and all probe containers were removed. Final `docker ps` and `docker volume ls --filter label=rcc.browser-acceptance` were empty. No pre-existing container or volume was changed. The temporary `.webkit-probe/` folder was removed after preserving its evidence. The Docker VM was initially stopped and was started for these checks; it remains running. The public Playwright image remains cached. A temporary empty Docker client config was used for the public pull because the existing Desktop credential helper was stalled; the user's Docker config was not edited.

Only this new dated evidence directory is added; earlier verification bytes are unchanged. No commit, push or external comment was made by this diagnosis task. The original CI stopped at WebKit: its later 26 cases were not executed. Those cases, Linux amd64 execution, and the combined-fixture approval path still require the formal CI run; none is represented here as green.
