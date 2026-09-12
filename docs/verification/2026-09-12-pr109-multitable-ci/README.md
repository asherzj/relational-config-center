# PR #109: multitable WebKit click diagnostics (unresolved intermittent timeout)

Base: `d181beaa5956691368a81947a62990529eb30bbf`. This candidate adds diagnostic evidence to `web/e2e/release-multitable.cjs` and its test-only `click-diagnostics.cjs` helper. **It does not fix or establish the cause of the intermittent CI timeout.** A subsequent green CI run would still not establish that cause or eliminate the recurrence risk.

## Observed failure and limits

Formal run `34664939825` passed 24 browser invocations, failed `release-multitable.cjs@webkit`, and did not execute the final nine invocations. The earlier accessibility repairs passed all seven checks in all three engines in this run. The multitable failure occurred on the original line 68, the real “查看 Change Set” click after modifying a record for an existing draft. Its 20-second actionability timeout logged only four unstable-element checks. That does not prove the button was moving continuously for 20 seconds. The original script closed the browser without preserving its own failure result or screenshots, so the artifact contains only the runner's fallback result and call log. No failed-page screenshot, focus/visibility state, frame timing, or geometry was available from that CI failure.

The initial local Linux WebKit run retained all original interaction timing and added only post-failure capture. All ten multitable checks passed, including two-table editing, 25-item publication/rollback, independent approval, 9 MiB original-request recovery through refresh/role/account changes, and storage-failure handling. This single pass only establishes intermittency/environment sensitivity.

The bounded real two-table loop then executed 20 iterations (40 target clicks), keeping the original selection, form fill, click, and save actions. Each iteration created its own empty draft and cancelled it afterward to release the same real database targets. It used the original 20-second click timeout and a 600-second outer limit. All 40 clicks passed in 556–669 ms; `loop-baseline-webkit/timing-summary.json` retains every timing. The loop was stopped after the fixed 20 iterations, without increasing the count to manufacture confidence. No local command reproduced the original timeout, so there is no justified production, animation, focus, or interaction repair.

## Diagnostic candidate

The target click still invokes the same Playwright `locator.click()` with its existing timeout, visibility, enabledness, stability and hit-target checks. It is neither forced nor retried. Before that click, an observer records at most 250 geometry/visibility/focus/animation snapshots at 100 ms intervals, alongside requestAnimationFrame counts and maximum gaps. It stops and disposes the observer in `finally`, on both success and failure. Setup, sampling retrieval and disposal each have a separate two-second diagnostic deadline. A late setup result is also stopped/disposed; a renderer that cannot answer is recorded as unavailable and ultimately released with the existing browser cleanup. Setup/teardown failures do not replace the real click result or exception. It records no form values, cookies, request headers, or credentials. This adds one setup roundtrip; the 100 ms rectangle/animation reads may force layout, and the continuous animation-frame callback adds renderer work. Both can perturb rendering and action timing; the samples are diagnostic evidence rather than a guarantee of an undisturbed schedule.

The three still-unresolved alternatives are actual geometry/animation movement, rendering-frame/background-page throttling, and a renderer or environment stall. The samples distinguish movement from a stable rectangle with sparse frames, and distinguish a hidden/unfocused page from an active one. They do not infer a cause merely from `not stable` wording. No `bringToFront`, CSS animation change, increased timeout or page-error whitelist is introduced.

On failure, the script first saves its partial checks, click observations, page errors and original exception. It then captures each page's visibility/focus and a best-effort screenshot with bounded waits, updating the result after each page. Page-state reads have a two-second deadline and screenshots have a five-second timeout. Artifact write failures are reported separately and cannot replace the original exception. It rethrows the original exception and retains the existing browser cleanup and final zero-page-error assertion. The failure result is available even if later screenshot capture fails.

`diagnostic-helper-check/` exercises the actual helper with a deliberately continuously moving button in real Linux WebKit. The unchanged click rejects with `TimeoutError`; the exact error propagates while geometry changes and frames are collected. Removing the synthetic animation permits a real successful click. Both branches leave zero outstanding observer intervals and animation frames. `diagnostic-final-helper-check/` additionally checks setup rejection, stop rejection, a stalled stop (bounded at about two seconds), and setup that responds after its deadline; all preserve the exact original click exception, including cleaning the late observer. **This is a test of the new diagnostic capability, not a reproduction or red/green fix of the original application failure.**

The existing `web/DESIGN.md`, root `CONTEXT.md` and multitable ADR 0025 were consulted. No application/design rule changes, permission changes, shared business helper changes or historical verification edits are made.

## Environment and commands

The official cached `mcr.microsoft.com/playwright:v1.62.1-noble` image (`sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e`) provides Linux arm64 WebKit 26.5 through a loopback-only server. Playwright 1.62.1 forwards client loopback using `exposeNetwork: '<loopback>'`. Chromium and Firefox, production Web preview and real Admin run on macOS; MySQL 8.4 is disposable in Docker. This is not the all-Linux amd64 GitHub environment.

Baseline and the bounded loop used the same command below with `RCC_E2E_ENGINES=webkit` and their respective artifact directories. The archived loop script was temporarily placed at `web/e2e/release-multitable.cjs`; the final source restores the complete business suite. `probes/` preserves temporary diagnostic snapshots and is not a set of additional CI tests.

```sh
NODE_OPTIONS='--require=/tmp/rcc-pr109-multitable-connect.cjs' RCC_WEBKIT_WS_ENDPOINT=ws://127.0.0.1:32770/ RCC_E2E_SUITE=release-multitable RCC_E2E_ENGINES=chromium,firefox,webkit RCC_E2E_ARTIFACTS=/tmp/rcc-pr109-multitable-final-bounded-shared bash scripts/browser-acceptance.sh
node /tmp/rcc-pr109-multitable-diagnostics-final-check.cjs
```

`initial-diagnostic-shared/` passed all ten checks in all three engines before the auxiliary deadlines were tightened. It is retained as an intermediate result, not substituted for final-source verification.

The final candidate verification is the complete multitable suite in shared Chromium → Firefox → Linux WebKit order, not all 34 formal browser invocations. Formal all-suite CI remains necessary, with the original intermittent timeout unresolved even if that run passes.

## Final same-source verification

`final-shared/run.txt` records `2026-09-12T01:58:35Z` → `2026-09-12T02:01:21Z`, exit **0**, and verified database cleanup. All three engines pass all ten multitable checks in one shared fixture sequence. Each `result.json` records `page_errors: []`, two successful observed clicks, the independent approval/publication/rollback journey, and a 9 MiB request preserved under the same key/order through account isolation and explicit recovery. The derived `final-shared/verified-summary.json` checks these fields directly.

The final WebKit target clicks recorded 607/565 ms, 17/16 observed frames, and visible/focused state throughout their sampled intervals. Those passing intervals provide a functioning diagnostic baseline; they neither prove nor disprove background throttling during the earlier CI failure. `elapsedMs` includes diagnostic retrieval/disposal after the click, while sample timestamps are renderer-relative.

The final WebKit `multitable-restoration-mobile-final.png` was inspected: it shows the real restored `before-1` value at 390px with the horizontally scrollable comparison surface. The screenshots preserve the existing business acceptance evidence, not an image of the unreproduced CI failure. `node --check` for both changed scripts and `git diff --check` pass; the runner also passes production build/typecheck and its timeout-cleanup regression test.

The final source hashes were fixed before the first browser case in this run started. The earlier diagnostic helper check exercised the identical observer/deadline logic; the final export additionally lets the caller reuse that same deadline for failure-page state reads. No browser business interaction, timeout, error acceptance or assertion was relaxed.

## Cleanup and manifest

Every real runner recorded successful removal of its own MySQL container and volume. The final named Linux WebKit container `rcc-pr109-multitable-linux-webkit` was removed after verification. Read-only Docker inventories then returned no matching task containers/volumes. Existing unrelated containers, volumes, Colima VM and cached images were left untouched. Temporary loop variants are archived only under `probes/`; the active E2E source contains no `[DEBUG-...]` instrumentation. The intended diagnostic observer remains explicitly part of this candidate.

`source-sha256.txt` identifies the frozen two-source candidate, and `SHA256SUMS` verifies all evidence files. Git status contains only those two source paths and this new directory; no historical evidence changed. Nothing was committed or pushed by the diagnostic agent.
