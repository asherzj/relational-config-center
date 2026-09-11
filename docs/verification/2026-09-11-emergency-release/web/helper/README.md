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
- No `web/e2e/release-emergency.cjs` file was created and no browser-system mutation was run by this agent.

## Hashes

- `source-sha256.txt` / `web-src-diff-sha256.txt`: pre-fixture-fix hashes retained as historical evidence.
- `source-sha256-final.txt`: final SHA-256 for every changed Web source file.
- `web-src-diff-sha256-final.txt`: final complete unstaged `web/src` binary diff: `3522524da9adb6783cf78c1ea2b20fbbd44fa65f62904ca3ddb7838dba146363`.
