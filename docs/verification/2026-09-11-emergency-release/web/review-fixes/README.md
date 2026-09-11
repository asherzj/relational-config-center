# Issue 104 Web evidence

Source tree: `/private/tmp/rcc-issue-104-emergency-release`

## TDD slices

- Release type switch and persisted flow replacement: `01-release-switch-red.log`, `02-release-switch-green.log`, `03-release-switch-green.log` (final PASS).
- Emergency reason, pending-publication UI, absence of approval UI: `04-emergency-submit-red.log` through `08-emergency-submit-green.log` (final PASS).
- Empty emergency draft creation: `09-new-emergency-draft-red.log`, `10-new-emergency-draft-green.log` (final PASS).
- Lost-response journal and exact manual submit replay: `11-emergency-submit-retry-red-or-green.log` (PASS; the existing journal seam already satisfied the new assertion).
- Type contract: `12-typecheck-first.log` (RED), `13-typecheck-green.log` (PASS).

## Regression and build

- `16-release-orders-suite-green.log`: release page plus release API, 90/90 PASS.
- `17-managed-data-regression.log`: RED because an existing local summary fixture lacked the new required `emergency_reason` DTO field.
- `18-managed-data-regression-green.log`: affected managed-data mutation page, 30/30 PASS.
- `21-typecheck.log`: PASS.
- `22-build.log`: PASS, including TypeScript and Vite production build.
- `19-web-full-suite.log`: first full run, 414/426 PASS. It exposed 11 `ManagedDataPage` failures plus the sandboxed TCP test failure.
- `20-unrelated-managed-data-isolated.log`: reproduced the 11 `ManagedDataPage` failures alone. Diagnosis showed the shared `enabledPolicy` mock omitted the required `version` field, so every table-policy response failed public DTO parsing.
- `23-managed-data-page-fixture-green.log`: after adding only `version: "1"` to that fixture, all 11/11 tests PASS. `managed-data-page-test-sha256.txt` records its source hash.
- `client-stream.test.ts` remains for the primary agent to rerun outside the sandbox; the failure is `listen EPERM 127.0.0.1`, followed by its 10-second timeout.

## Browser-system blocker

- `approval-rejection.txt` preserves both automatic approval review outcomes. The second review rejected the proposed script even after requiring `RCC_E2E_ISOLATED=1`, local HTTP, and a loopback hostname.
- This blocker was later resolved by the user's explicit authorization (“同意啊，你继续”). `web/e2e/release-emergency.cjs` was then created with the documented isolation guards; the primary agent owns its real browser execution.

## Independent-review P2 fixes

- `25-submit-type-conflict-red.log` and `26-submit-type-conflict-green.log`: rejected submit rebuild now follows the latest persisted release type in both directions, drops a stale emergency reason when the latest type is STANDARD, and requires a fresh Unicode-counted reason when it is EMERGENCY.
- `27-emergency-copy-reprepare-copy-red.log`, `28-emergency-copy-reprepare-copy-green.log`, and `28b-emergency-copy-reprepare-copy-green.log`: emergency copy/reprepare text no longer implies approval and explains that a new reason and manual publication are required. `28` preserves a test-selector correction failure; `28b` is the final PASS.
- `29-emergency-success-feedback-red.log`, `30-emergency-success-feedback-green.log`, and `30b-emergency-success-feedback-green.log`: successful emergency draft creation and submit show confirmation; lost-response submit does not. `30b` confirms the final neutral submit wording after primary review.
- `31-emergency-hide-approver-red.log` and `32-emergency-hide-approver-green.log`: emergency detail hides the inapplicable recent-approver row while STANDARD behavior remains covered by the full suite.
- `33-release-orders-p2-regression.log`: release page plus API, 93/93 PASS.
- `34-web-p2-typecheck.log`: PASS.
- `35-web-p2-build.log`: PASS, including TypeScript and Vite production build.
- `36-web-src-diff-check.log`: PASS.
- `37-web-src-sha256.txt`: SHA-256 for every currently changed `web/src` file.
- `38-web-src-diff-sha256.txt`: complete current `web/src` binary diff SHA-256, `2bc7feb345c56c5297935d6ac7d46832d8a6076ce66aebee772b018190ef15f6`.

## Hashes

- `source-sha256.txt` / `web-src-diff-sha256.txt`: pre-fixture-fix hashes retained as historical evidence.
- `source-sha256-final.txt`: final SHA-256 for every changed Web source file.
- `web-src-diff-sha256-final.txt`: final complete unstaged `web/src` binary diff: `3522524da9adb6783cf78c1ea2b20fbbd44fa65f62904ca3ddb7838dba146363`.
