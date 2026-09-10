# Standards review — candidate 3 fixture delta

Reviewed tree `759ae638fe41d0c082ad9757c391d342e5402130` → `44b4241b2605df6b7641f28d15a3f3cf7ca66d6c` in `/private/tmp/rcc-issue-88-final-release-cleanup`, following the accepted candidate 2 review. The complete delta contains exactly two test files, three additions and three deletions; no production code changes.

Read `AGENTS.md`, the domain/design agent conventions, the root and Admin domain terminology, `web/DESIGN.md`, ADR-0025, the T7 ticket and `standards-review2.md`. Applicable written rules include `web/DESIGN.md`'s “每条明细携带真实表名”, preserving exact original/edited field values, and T7's removal of single-table default fields and adoption of the final interface by callers.

**Hard written-rule violations: 0 actionable findings.**

- `release_copy_reprepare_multitable_integration_test.go:60–70` now omits only `detail_id` in the successful copy case. Both original table names remain explicit. The two restored detail IDs and their ordered table identities are still asserted. Replaced-ID rejection, cross-table rejection and its global item index remain unchanged. The existing `TestReleaseWritesRequireExplicitTableOnEveryDetail` separately retains omitted-table rejection for incremental edit, copy and reprepare, including unchanged storage snapshots.
- `complex-fields.cjs:177–180` checks that the retired root `table_name` is absent and the item's actual table is correct. Item count, operation, exact content, JSON record identity and Change Set value assertions remain. Independent approval, stored-value preservation and publication-failure atomicity checks are unchanged. This updates the expected envelope without weakening those business assertions.

**Subjective baseline smells: 0 actionable findings.** The small fixture corrections introduce no actionable Fowler-baseline smell and need no repository-pattern refactor.

Conclusion: **Standards PASS for this candidate 2 → 3 delta; written-rule findings 0, subjective findings 0.** Only this report was written. No candidate source/index changes, tests, builds, database operations or remote writes were performed. Browser and broader verification outcomes remain separate evidence owned by the coordinating agents.
