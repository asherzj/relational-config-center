# PR #109: deterministic accessibility fixture and restored mobile navigation

Base: `071ad8c0671f2827fd6edd7502d51eb0727e4025`. This task changes only `web/e2e/browser-accessibility.cjs` and adds this evidence directory. It does not change application behavior, authorization, the shared recovery helper, timeout values, or earlier verification evidence.

## Confirmed failure

Formal run `34662979824`, Browser job `103469164442`, passed the preceding browser cases and the first five WebKit accessibility checks, then rejected approval of order `6702646716bffadf9a521e6bba694f32`: HTTP 403 `permission_denied`, request ID `rcc-afb21f23bba9eab45e9b9786ae2b34b0`. The preserved `ci-second-webkit/result.json` shows that the browser's create request actually targeted `field_display_browser_items`, including the generated title, even though this suite's assigned reviewer role was for `stage1_acceptance_items`.

A local reproduction retained the original Chromium → Firefox → Linux WebKit order against one shared, real MySQL fixture. Instrumentation saved existing read results in memory and issued extra diagnostic reads only **after** the unchanged approval assertion failed. `shared-red-webkit/approval-diagnostic.json` proves:

| Fact | Recorded value |
| --- | --- |
| Expected applicant and actual order owner | `7b5ab985-783c-47af-a43a-60d8844c6f70` |
| Expected reviewer and actual session account | `9b3c94e4-a0d6-4f22-a0aa-aa59c2698cac` |
| Created and submitted order | `2c19d2bc4b987d7032b581ceeb1d586a` |
| Newly created, enabled role | `0c47cbc7-cd1f-43d0-b152-224acb1fb2aa`, version `1` |
| Its enabled member | The expected reviewer above |
| Current `stage1_acceptance_items` assignment | Version `3`, exactly the newly created role |
| Actual create body and submitted table | `field_display_browser_items` |
| Frozen approval roles for that actual table | Empty |
| Reviewer eligibility before and after rejection | `mode=ADMIN`, `can_approve=false`, `approvable_tables=[]`; identical revision |

The reviewer is a VIEWER and is not the applicant. Account identity, role membership and the assignment to the intended table are correct. The order concerns the other table, so the service correctly denies approval. Business-request evidence also shows no query of `stage1_acceptance_items` in the failing WebKit path, only the default `field_display_browser_items` query. This is not a reason to grant broader authority or accept a 403 as success.

The old `managed()` fixture opened the generic page, called `selectOption(table)`, then waited only for an already-existing “新增记录” button. It did not check the selector's retained value, the opened drawer's table, or the new order's table. In the failing full sequence, the intended selection did not become the actual edit target. The precise reason that this initial automated selection did not take effect is **not established**. This repair does not claim to fix a product table-switching defect.

## Repair scope

The accessibility suite now opens the application's existing `/configuration/managed-data?table_name=...` deep link, so the first rendered workspace targets its fixture table. It asserts the selector value, exact table-specific new-record drawer title, submitted `items[0].table_name`, resulting `table_names`, and applicant ownership. These checks prevent another wrong-table path from silently reaching an unrelated permission or SQL assertion.

This suite owns focus, narrow-screen overflow, raw-CR preservation and uncertain-write recovery. Dedicated table-switching coverage remains unchanged in `web/src/features/managed-data/ManagedDataPage.test.tsx`, the test “切换 enabled Managed Table 时重置 Query Spec 并查询所选表的动态列” at line 172. It selects another table and verifies that table's columns, request path and reset query specification. The second formal CI's Web tests already passed; this task did not replace or weaken that coverage.

An intermediate full shared run with the deep-link fix passed every business check and the independent approvals, but failed the final zero-page-errors assertion on `/api/v1/table-field-policies/stage1_acceptance_items due to access control checks.` The previous patch waited for the restored selector after `reload()`, then `repeatDraftSave()` immediately performed another full document navigation. The selector can be visible while subsequent field-configuration reads are still starting. That intermediate run remains explicitly failed in `deeplink-only-shared/`.

The final 390px recovery path retains the real reload and waits for the restored control, then uses “打开导航” → “发布单” → confirm “放弃修改并离开” in the leave-protection dialog → “新建草稿” → “确认并保存草稿”. This follows the original business action through the real mobile UI without a second document teardown during startup reads. The leave confirmation only proceeds with navigation (`LeaveProtection.tsx`); journal unmount only removes its listener, and unresolved original request storage remains available to `useReleaseWrite.retry()`. The passing body/key equality below verifies that this route did not discard the original intent. The shared helper is unchanged; the now-unused helper import is removed from this script. The full page-error assertion, original body and idempotency-key equality, separate reviewer approval, real publication and SQL checks remain intact. No automatic write retry or error whitelist is introduced.

`web/DESIGN.md` needs no amendment: this is a test fixture/navigation correction using existing product behavior and accessible controls, not a design change.

## Diagnostic boundaries and retained attempts

- `pre-read-diagnostic-green/`: a full shared run passed and captured correct actors, roles and snapshots, but the extra **before-approval** reads could alter timing. It was not treated as proof of resolution.
- `shared-red/` and `shared-red-webkit/`: the next run preserved the original approval request timing and reproduced the actual wrong-table 403. This is the decisive account/role/snapshot evidence.
- `selection-probe/`: a narrower selection→drawer loop observed the correct table on 17 attempts, then failed during drawer-close actionability/runner termination. It did not reproduce or exclude the original initialization failure and is not reported as a 20-iteration pass.
- `selection-events-webkit/`: the complete business sequence passed, but a default-table field-read access-control error failed the final assertion. Its post-failure probe additionally attempted to read an earlier page response after that page had closed, producing a diagnostic exception. The result still preserves the original page-error failure; this attempt did not yield reliable selection-event evidence and is not reported as passed.
- `mobile-before-confirm/`: the first mobile-navigation variant stopped at the expected leave-protection dialog in Chromium. It had not confirmed leaving and did not reach the recovery action. The final script explicitly confirms that real dialog; this intermediate attempt is not passed.
- A first diagnostic setup mistakenly passed the auth/session path to a helper that intentionally permits only business paths. It stopped at that helper assertion, was corrected to the proper session-read API, and its isolated database was cleaned up. It was not the reported 403.

`probes/` contains archived temporary script variants, including the after-failure instrumentation and Linux transport preload. They are evidence snapshots of code temporarily placed in `web/e2e`, not additional CI tests or generally runnable standalone commands. All diagnostic edits to the tracked helpers were restored before final verification.

## Execution environment

The actual-browser runner uses the production Web proxy, real Admin, account-maintenance command and disposable MySQL 8.4. Chromium and Firefox run locally on macOS; WebKit 26.5 runs in the cached official `mcr.microsoft.com/playwright:v1.62.1-noble` container on Linux arm64, connected with Playwright 1.62.1 and `exposeNetwork: '<loopback>'`. Its service is bound to a random host-loopback port and has no Docker-socket or credential mount. The client/backend host and architecture differ from GitHub's all-Linux amd64 environment; the formal CI still provides that final boundary.

The failure and final verification use the same shared sequence, not separate fresh databases per engine:

```sh
NODE_OPTIONS='--require=/tmp/rcc-pr109-approval-connect-linux-webkit.cjs' RCC_WEBKIT_WS_ENDPOINT=ws://127.0.0.1:32769/ RCC_E2E_SUITE=browser-accessibility RCC_E2E_ENGINES=chromium,firefox,webkit RCC_E2E_ARTIFACTS=/tmp/rcc-pr109-approval-final-shared bash scripts/browser-acceptance.sh
```

The failing preserved sequence used the same runner/environment with the original script plus post-failure-only diagnostics and artifact path `/tmp/rcc-pr109-approval-diagnostic-shared-3`.

## Final verification

`final-shared/run.txt` records exit status **0**, start `2026-09-12T01:19:05Z`, finish `2026-09-12T01:20:13Z`, and verified cleanup. Chromium, Firefox and Linux WebKit each pass all seven accessibility checks in that order against the same database and persistent role assignments. Each final result has `ok: true`, `pageErrors: []`, and zero remaining test rows. `final-shared/verified-summary.json` derives and checks the two identical unknown-result draft POST records (including body and idempotency key) for each engine. Each recovery check records actual 201 committed then injected 503, real independent approval/publication, and exactly one resulting SQL row. Build/typecheck and the runner timeout-cleanup test also pass.

The three `write-recovery-390.png` screenshots show the recovered, correctly targeted draft in the 390px workspace. The WebKit image was inspected. The page is not refreshed after subsequent API publication, so its visible draft state is not evidence of final publication; the actual publication response and SQL assertions in `result.json` provide that evidence.

This is the complete three-engine **accessibility suite**, not a rerun of all 34 formal browser cases. No repeated green run was used to imply certainty about the unknown initial selection mechanism. The next formal CI must still verify the complete browser suite on Linux amd64.

## Cleanup and frozen files

The runner verified removal of its named MySQL container and volume. A final read-only Docker inventory found no `rcc-browser-*` containers or volumes; this task's exact Playwright container `rcc-pr109-approval-linux-webkit-1789172100` was removed after the pass. Existing unrelated containers and volumes were left untouched. The existing Colima VM and cached images remain available.

`node --check web/e2e/browser-accessibility.cjs` and `git diff --check` passed. Git status contains only that source modification and this new evidence directory; historical verification files and shared helpers have no diff. `source-sha256.txt` binds the verified candidate, while `SHA256SUMS` covers the evidence files.
