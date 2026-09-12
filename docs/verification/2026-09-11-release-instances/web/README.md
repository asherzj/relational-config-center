# #103 Web implementation evidence

Worktree: `/private/tmp/rcc-issue-103-release-instances`. The implementation and local verification use the already agreed React/API and real-browser seams. Backend/real MySQL verification, browser execution, independent Standards/Spec review, issue bookkeeping, and commit remain with the parent issue agent. This note does not mark #103 complete.

`verified-source.json` records the baseline commit, Node/pnpm versions, and SHA-256 of every changed Web file. `web-change-snapshot.patch` preserves the implementation and test diff including new files. Dependencies were installed from the unchanged lockfile in this isolated worktree. No database or container was started by the Web agent. No commit was created.

| Behavior / acceptance | Actual evidence |
| --- | --- |
| AC-009: API retains each saved table flow's identity, template/association version, node state and actor/time | `02-api-red.log` fails because existing parsing discards the fields; `03-api-green.log` passes after schema extension |
| AC-009/010: independent table names/templates/nodes and source details render; ordinary reread does not write | `04-render-red.log` fails because per-table flow UI is absent; `05-render-green.log` passes |
| AC-010: missing flow list blocks submit, displays no fabricated nodes, unchanged explicit draft save fills missing flow and sends original title/version plus empty changes | `06-repair-red.log` fails because repair UI is absent; `08-repair-green.log` passes |
| Existing approval scope, partial approval whole-order phase, actor/time presentation, original request/journal and release-page regressions | `10-affected-regression.log`: 3 files, 86 tests passed |
| Shared release response consumers, draft/unsaved protection, diff rendering | `13-consumer-regression.log`: 60 passed, 1 fixture failure; only failed fixture was changed, then `15-summary-consumer-green.log`: 1 passed. The other 60 tests' code and production dependencies were unchanged |
| TypeScript and production build | `12-build.log`: TypeScript and Vite build passed; `14-typecheck.log`: current fixture/version changes passed TypeScript |
| Browser scripts | `node --check web/e2e/release-instances.cjs` and `node --check web/e2e/release-approvals.cjs` passed; real browser execution is pending parent coordination |

The total applicable local regression evidence is 147 passing tests across six files. The new behavior tests use HTTP boundary responses and the real React application; they do not establish MySQL persistence or concurrent transaction behavior.

Retained unsuccessful attempts: `01-api-red.log` is an environment failure (copied dependencies did not include fake-indexeddb), not a behavior red; `07-repair-green.log`, despite the planned filename, is a failed attempt caused by asserting button readiness before journal cleanup, corrected by waiting for the visible readiness state; `09-typecheck.log` found two test-only unsupported `exact` options; `11-consumer-regression.log` found 36 failures, primarily pre-existing table-policy test fixtures without the required version. Two fixtures were updated to the public table-policy contract. `13-consumer-regression.log` exposed the one summary fixture without the new release fields, corrected in `15-summary-consumer-green.log`. No failed log was overwritten or relabeled as passing.

`web/e2e/release-instances.cjs` is prepared for `TestReleaseFlowBrowserSystemPath` with the existing `003-policy-fixture.sql` tables and `template.browser.admin` login. It uses public authenticated APIs for template/association/role fixture setup, then real Web to create a multi-table draft, save missing flows, submit, approve independently, publish and complete. It verifies template edit/disable/association isolation, read-only refresh, committed response loss plus refresh and exact manual save replay, frozen table roles, partial/all approval node states, and desktop/390px layouts. `RCC_WEB_URL` and a fresh explicit `RCC_E2E_OUTPUT` are mandatory; existing screenshot/result files cannot be overwritten. Planned coverage is AC-009/010/014 plus relevant AC-023/025; parent HTTP tests own the primary AC-011/013/022 proof.

Temporary structure: `ReleaseProgress.tsx` remains referenced only when `order.state === "ROLLED_BACK"`, with an inline comment and `web/DESIGN.md` explanation assigning removal to #106. Forward STANDARD paths now use saved table nodes and a separately displayed whole-order phase; missing configuration never uses fixed regular nodes. The parent must register this temporary rollback-only presentation on #106 before issue handoff.

Remaining acceptance: run the real browser wrapper against the final Admin implementation; inspect desktop/390px screenshots; resolve any browser failures; finish the independent dual-axis review; verify source hashes against any later changes and repeat only affected checks.
