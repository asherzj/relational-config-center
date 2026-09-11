# #98 Web integration verification

Target: merge published `eb55e841e80345cfef7733b8c560d6037256cd2c` into checkpoint `aec58942f01143c1a579aaf2a885141bf5a263c9` without implementing #105 or pre-merging #104.

## Semantic resolutions

- `web/DESIGN.md`: kept #98's unified-change navigation and compact release-detail rules; restored the release-template entry, table-template association rules, template catalog rules, persisted per-table instance facts, and explicit missing-flow save behavior from the template branch.
- `web/src/features/release-orders/ReleaseOrdersPage.tsx`: kept #98's compact business-title header, primary-action selection, destructive More menu, collapsed IDs/basic information, and review toolbar; kept notification acknowledgement from #97; restored the template branch's missing-flow repair entry and persisted `ReleaseFlows`. Forward STANDARD orders use the factual whole-order phase plus persisted table instances; only rolled-back orders retain the temporary legacy progress component.
- `web/src/features/release-orders/ReleaseOrdersPage.test.tsx`: retained both template-instance/missing-flow tests and #98 compact-layout/action tests. The approval-person assertion opens collapsed basic information before checking the current name, matching the merged public UI.
- `web/e2e/release-approvals.cjs`: kept #98 selectors for status badges, collapsed basic information, renamed difference filtering, UUID disclosure, and More-menu cancellation; retained template assertions for the factual whole-order phase and persisted per-table flow region.
- `scripts/browser-acceptance.sh`: the prior three-way semantic merge required no additional content edit; its shell syntax and all top-level Web E2E JavaScript syntax were checked.

## Results

- TypeScript: PASS.
- Affected tests: 8 files, 154 tests PASS in `05-affected-tests-green.log`.
- Production build: PASS.
- Browser acceptance shell and E2E JavaScript syntax: PASS. No database or browser run was performed.
- Conflict-marker scan and `git diff --check`: PASS.
- `02-affected-tests.log` preserves the first failure caused by a stale #98 expectation of a fixed four-step list. `03-affected-tests-green.log` preserves the second failure after switching to the template phase: the approver existed inside collapsed basic information but the assertion expected it visible before expansion. `04` proves that focused fix and `05` is the final affected regression PASS.

Commands and exit codes are recorded in `commands.tsv`; final source hashes are in `08-source-sha256.txt`.
