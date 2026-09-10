# Spec re-review — candidate 3

**Residual Spec findings: 0.**

Reviewed candidate 2 tree `759ae638fe41d0c082ad9757c391d342e5402130` → candidate 3 tree `44b4241b2605df6b7641f28d15a3f3cf7ca66d6c`, both complete changed files, and the prior `spec-review2.md` conclusion. The delta contains only the two fixture corrections below. Both current files match candidate 3 and their SHA-256 entries in `source-candidate3.json`; unstaged/untracked inspection is empty. No source edits, Go commands, builds, tests, or database operations were performed. Only this requested review report was written.

- **AC-008 and final explicit-table contract:** `admin/cmd/admin/release_copy_reprepare_multitable_integration_test.go:60–70` now omits only `detail_id`, retains each explicit `table_name`, and still checks both stable identities at their original table/position. Wrong detail identity and cross-table substitution remain rejected. This aligns with `release_orders.go:846–868`: a source position supplies its stable detail ID; a missing or different table cannot inherit a default. The complete file retains target-conflict, atomic transfer, rollback-on-failure, exact replay, and fresh independent approval assertions.
- **AC-001 / AC-015 and T7 cleanup:** `web/e2e/complex-fields.cjs:177–183` requires root `table_name` to be absent and the sole item's `table_name` to equal the selected table. It replaces the retired single-table expectation without weakening item count, operation, identity, exact content, or downstream SQL/result/input-protection checks. No production behavior, compatibility layer, automatic retry, or new scope is introduced.

`integration-fixture3.jsonl` records the complete `TestMultitableDerivedDraftRequiresExactDetailIdentity` PASS (9.41 seconds) and package PASS, with no test skip/failure. Its SHA-256 is `b4407dde6670983caace3769d7a7c39ef29a14cc3158071777c444d0963c76b8`. The existing final integration reconciliation reports 335/335 PASS and retains earlier failed attempts.

No new missing requirement, suspicious implementation, or exit-condition gap was found in this delta. Browser verification is still pending; this review does not claim browser success or final feature acceptance.
