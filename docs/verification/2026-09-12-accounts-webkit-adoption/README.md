# #110 final candidate: adopt the verified Playwright version

This candidate pins Playwright/core **1.63.0**, which supplies Linux WebKit **2359 / 26.6**, and removes the temporary investigation runtime. The existing bounded accounts diagnostics remain. The dependency change is byte-identical to the reviewed candidate used in the successful native comparison; all account permissions, business assertions, original request bodies/keys, real SQL checks, animation/compositing settings and timeouts remain unchanged.

The observed 2336 failure is a native compositor-thread NULL-address SIGSEGV. The 2359 comparison completed 12 rounds without that signature. This supports adoption as an evidence-backed browser-toolchain change; it does **not** identify a particular native fix, prove all previous failures had this cause, or rule out future intermittent faults. Final delivery still depends on independent review and the integrated candidate's original eight CI jobs, including the full 34-case browser suite without ptrace.

## Original failures and successful bounded comparison

The original main failure and subsequent natural failures remain in earlier verification snapshots. `../2026-09-12-accounts-webkit-version/signal-ci/` preserves all three 2336 raw traces and the independent report/correction: reviewer compositor thread 7852 received `SIGSEGV / SEGV_MAPERR / NULL`; WPEWebProcess 7839 then terminated, followed by page-crash and only later cleanup. The correction identifies normal traces 7683/7874 versus failing 7806, and confirms real ADD publication/lost-response replay plus concurrent MODIFY occurred before the later approval failure. Those failed runs are not rewritten as passing results.

[Comparison CI 34684793011, job 103529792215](https://github.com/asherzj/relational-config-center/actions/runs/34684793011/job/103529792215), source `4b45aa1b3f16a9ca8401cc7aa0c887c751d0aae4`, used the exact temporary dependency patch now adopted:

- All **12/12 accounts rounds**, each in an independent Node process/profile against shared real services, completed the same **21 checks**. No failed round was retried. The unchanged 600-second case limit and 15-minute job budget applied.
- All 12 lifecycle logs lacked page-crash/test-failure; all 36 browser launch traces had normal native exits and no abnormal signals or kill/tgkill calls.
- Account rows accumulated from 4 to 25 returned rows; round 12 had a next cursor and still selected the intended account through the original exact search.
- Original body/key lost-response recovery, permissions, account isolation and publication assertions completed. SQL postcheck/final summaries matched the expected baseline, fixture-before/after were byte-equal, runner exit was 0 and cleanup was verified.
- Runtime identity was Playwright/core 1.63.0 and WebKit 2359/26.6. Dependency files and launcher SHA/mode were restored by the temporary supervisors. Their successful cleanup paths provide the recorded backup-removal evidence; browser-cache contents themselves were not uploaded.

`independent-version-report.md` is the unmodified independent Spec report. `business/` contains the original 12 runner/lifecycle/targeting records and SQL/fixture results; `native/` contains all 36 original signal traces and identity/restore records. `full-artifact-sha256.json` indexes the complete downloaded artifact, including omitted screenshots/install logs. Artifact ID **10294964597**, archive digest `2e7d0fceaac5277883846cf7474756fa103c1276a43031b261d2d6e6e2440e2b`, is available through that run's artifact download. Raw source bytes are preserved.

The formal Browser job in the **same source run 34684793011** still used the unchanged official 1.62.1 dependency and failed accounts at line 322 (first publication confirmation): 16 matrix calls passed, one failed and 17 did not execute. `old-version-formal/` preserves its original case matrix, lifecycle, runner error and cleanup result. The reopened page recorded page-crash at 35,262 ms, confirmation-end/failure at 35,264 ms and cleanup at 35,267 ms; the observed click lasted 22,388 ms, not the 600-second case timeout. Its three rollback engine calls passed. This formal run had no native trace, so its page-crash cannot be assigned the compositor function or signal from the separate 2336 trace. It remains a FAIL alongside the successful temporary 1.63 comparison.

The old/new comparison ran on different GitHub-hosted VMs of the same native amd64 platform class, and ptrace can perturb timing. It establishes a bounded whole-version effect, not a statistically guaranteed or precisely attributed native fix. The upstream null-protection leads documented in the previous version snapshot remain unconfirmed matches.

## Final scope and temporary-runtime removal

`web/package.json` and `web/pnpm-lock.yaml` adopt only Playwright/playwright-core 1.63.0 and remove the obsolete Darwin-only optional fsevents@2.3.2 that belonged exclusively to old Playwright. Other dependency graph entries are unchanged. The toolchain update also supplies Chromium 153.0.8010.12/revision 1243 and Firefox 155.0/revision 1543, so the final integrated three-engine browser suite is required; the WebKit-only comparison does not certify those engines.

`.github/workflows/ci.yml` is restored byte-for-byte to main `a306c94f484a5a4796f727bc4a37c192d9538bef`: only the original eight required jobs remain, with no temporary DEBUG or ptrace configuration. The three temporary scripts (`diagnose-accounts-webkit-repeat.cjs`, `diagnose-accounts-webkit-signals.sh`, `diagnose-accounts-webkit-version.sh`) and generated runners are removed. This agent started no Docker containers during adoption; the prior task-owned calibration container was removed before this stage. CI cache-wrapper and dependency-backup recovery are documented by the comparison artifacts; no local wrapper/backup is part of the final runtime.

The three formerly active version-fixture files are relocated byte-for-byte into **retired-fixture/**, which has no runtime consumers. `retired-fixture/relocation.json` records original paths, archived paths and identical SHA values. Earlier manifests describe their fixed historical commits/trees and retain that meaning; the archived fixture is not an active CI input. Other historical probes and reports remain evidence only. No #112 source or evidence is included or modified here.

## Local verification and acceptance status

After installing the exact frozen dependency graph in this isolated checkout, typecheck, build, **42 Web test files / 480 tests**, and **2 development-origin checks** passed. Logs and installed package metadata are in `local/`. The first unit invocation had 479 passing tests and one explicit sandbox `listen EPERM 127.0.0.1` failure; its original log is retained as `unit-sandbox.log`. With local listening permitted, the identical source and complete test suite passed. This was an environment correction, not a business retry or source fix. No additional browser/Docker run occurred locally.

| Acceptance | Evidence / remaining requirement |
| --- | --- |
| AC-F01 | Original main/matrix failure and later lifecycle/native SIGSEGV are preserved, distinguishing stability waits, native faults and cleanup. |
| AC-F02 | The exact dependency candidate completed 12 × 21 real accounts checks with SQL/fixture and lost-response recovery. Final integrated 34-case browser CI remains required. |
| AC-F03 | Exact reviewed toolchain update; no forced clicks, deleted assertions, timeout increases, disabled animation/compositing or retry-to-green. Native repair-function attribution and universal resolution are not claimed. |
| AC-F04 | Source/evidence manifest and local checks are fixed for independent Standards/Spec review. Final integrated HEAD's eight required CI statuses are pending before delivery. |

The earlier Go/Compose/MySQL evidence covers unchanged application and server source; final required statuses must still be evaluated on the integrated HEAD. Root will integrate the separate #112 fix, run the complete original browser matrix, and record final review/CI results before delivery. This snapshot is the completed #110 implementation candidate, not a declaration that the whole release is already accepted.
