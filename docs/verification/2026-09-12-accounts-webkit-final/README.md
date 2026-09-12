# #110 final candidate: bounded accounts diagnostics

This candidate delivers diagnostic improvement. A natural WebKit page crash has been reproduced, but its underlying cause has **not** been fixed or identified. The permanent source change is limited to `web/e2e/accounts.mjs`: bounded publication-click observations, page/context/browser lifecycle evidence, and failure artifact capture that preserves the original business error. No product behavior, click semantics, assertion, permission, SQL expectation, or case timeout changes.

## Bounded experiment and retained failure

[Native diagnostic run 34681965121, job 103522113478](https://github.com/asherzj/relational-config-center/actions/runs/34681965121/job/103522113478) ran commit `5ac0d268e51814973b977371270da2f351fccbda` on Linux amd64 with the original accounts case, persistent close/reopen, separate browser contexts and real MySQL. The fixed 12-round experiment stopped at its **first failure**: round 1 completed all 21 checks; round 2 exited 1; rounds 3–12 never ran. This result is FAIL, not 12 successful rounds or evidence of a repair.

Round 2's initial observed publication confirmation succeeded in 953 ms. The later confirmation at accounts line 439, after rebuilding a conflicting modification against its latest baseline, waited for stability and then failed with TargetClosed. The `reopened-persistent` page recorded `page-crash` and `test-failure` at 46,092 ms; cleanup began at 46,096 ms. Cleanup did not initiate this failure; the original 600-second case timeout had not elapsed. The account directory contained six rows with no next cursor and found the intended user, so pagination is not required to trigger this failure. These findings do not retroactively identify the unobserved close source in the original main run.

The 152 resource samples show minimum MemAvailable 12,575,348 KiB and zero observed cgroup OOM counters throughout. Renderer PID 7701 remained in the last pre-cleanup sample, with approximately 751 MiB RSS during the preceding 21 seconds. No contemporaneous kernel OOM or native crash stack was captured. Playwright's parent processes subsequently exited gracefully with code 0; this does not establish the renderer's exit signal. Sampling can miss transient behavior. No CPU, wait-state, core dump or renderer exit-status evidence was collected, so a specific native failure mechanism cannot be inferred from stable RSS.

`independent-ci-report.md` is the independent Spec agent's unmodified report. `native-job.log`, the two iteration directories, resources and kernel output are original bytes. `database-final.txt` and `run.txt` document cleanup, but the failed suite did not reach the normal SQL postcheck; do not count it as a complete business regression. `artifact-index.json` gives the full artifact's API URL and archive digest (artifact 10294074904, expiry 2026-09-26); `full-artifact-sha256.json` indexes every downloaded file, including omitted screenshots and install logs. This archive retains only the small case evidence plus the requested raw resource samples.

## Exact upstream classification and limits

The inspected source is the official Playwright **v1.62.1** tag, matching the tested dependency and WebKit revision 2336:

- [wkPage.ts, lines 236–248](https://github.com/microsoft/playwright/blob/v1.62.1/packages/playwright-core/src/server/webkit/wkPage.ts#L236-L248): the current target's destroyed event causes `_didCrash()` only when its protocol `crashed` flag is true.
- [bootstrap.diff, lines 10316–10327](https://github.com/microsoft/playwright/blob/v1.62.1/browser_patches/webkit/patches/bootstrap.diff#L10316-L10327): WebPageInspectorController only sends the crash event for WebKit's `ProcessTerminationReason::Crash` classification.
- [bootstrap.diff, lines 14083–14093](https://github.com/microsoft/playwright/blob/v1.62.1/browser_patches/webkit/patches/bootstrap.diff#L14083-L14093): process-termination handling calls that inspector path; [lines 334–349](https://github.com/microsoft/playwright/blob/v1.62.1/browser_patches/webkit/patches/bootstrap.diff#L334-L349) distinguish ordinary target destruction from a crash.

Inference: the observed page event reflects WebKit's internal Crash classification. It does not provide an OS signal or native stack. The separate Unresponsive classification is not accepted by the inspected crash-event condition, so the present data do not justify claiming an unresponsive watchdog caused the event. There is no evidence sufficient for a product or dependency fix. This bounded investigation stops here; further native investigation would require a new explicit hypothesis and evidence scope.

## Cleanup and acceptance mapping

The final candidate restores `.github/workflows/ci.yml` byte-for-byte to `d4f12028732086769006cf7b55d7c19a2b0e036b`. It removes the temporary native diagnostic job, formal Browser DEBUG setting, native sampler, repeat generator and any generated runner. The original eight required jobs and business runner are restored unchanged. Earlier verification directories remain immutable historical records; their descriptions of temporary structures or pending experiments refer to those historical snapshots. Their archived probes are not active CI or final runtime dependencies, and their superseded exit wording does not require claiming a native root-cause fix.

| Acceptance | Evidence and current status |
| --- | --- |
| AC-F01 | Original main failure and execution boundary retained in `../2026-09-12-accounts-webkit/`; later lifecycle failure in `../2026-09-12-accounts-webkit-crash/`; this archive adds the bounded natural failure, native evidence and classification limits. |
| AC-F02 | Unchanged accounts source completed 21 checks in each of three engines in `../2026-09-12-accounts-webkit/final-three/`, including real SQL and dropped-response recovery; formal run 34680438033 completed all 34 browser invocations. The new diagnostic FAIL remains a separate result. |
| AC-F03 | Diagnostic-only scope, timing perturbation and unknown root cause are explicit. No force click, retry-to-green, timeout increase or weakened assertion. Final source hash remains `27596293b5ba49bcfb57fe535c787d0dc6359d10895d8f40e8e134acb65fcd9f`. Final post-cleanup affected CI is pending. |
| AC-F04 | Local cleanup checks and frozen source/evidence manifest accompany this candidate. Independent Standards/Spec final review and the final HEAD's eight required CI results remain pending before delivery. |

Technical evidence from [run 34680438033](https://github.com/asherzj/relational-config-center/actions/runs/34680438033), commit `2ecfeef3e155a659358bb3a06d212f7f3fe63458`, can be reused for unchanged Go/Web/Compose/MySQL source (431 MySQL tests) and the unchanged accounts script. The earlier no-DEBUG three-engine accounts regression has exactly the final accounts source. These are supporting evidence, **not** substitutes for required statuses on the final HEAD or proof that intermittent crashes are eliminated. The final formal Browser run must still execute the original 34 invocations, with the full required CI and independent review recorded by the integrating agent before completion.

Raw TSV empty cells and original log whitespace are preserved byte-for-byte. Whitespace checks cover source and authored documentation; raw evidence is excluded rather than normalized. The four original `.log` files are explicitly included in the frozen index despite the repository log ignore rule.
