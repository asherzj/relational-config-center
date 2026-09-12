# #97: release lifecycle result notifications

Base: `12fda54fc3c2f74e09e7dbde13a6e723d6a98e33`; branch `codex/issue-97-release-notifications`. Full issue 97 / parent 92 snapshots (including empty comments and labels) are adjacent. The parent-approved formal Web, authenticated HTTP + real MySQL seams were used. Root owns push/integration/issue closure/Notion; this package records local implementation and acceptance.

## Delivered contract

Publication, completion and original-order quick rollback notify the applicant and actual saved table-decision actors by permanent Account ID. Duplicate roles/tables aggregate once, the actor is excluded, and former/disabled role members and default ADMIN decision makers retain their historical participation. A publisher who never approved is not a result recipient. The actor's older unread result is preserved.

The original transaction saves business values, actual execution results, versions, order state, downstream refresh records, personal notifications and request result consistently. Completion changes no business row, execution, command or downstream refresh but still notifies participants. Quick rollback uses the original order. Reprepare atomically cancels its source, creates the replacement draft, transfers targets and notifies only the source applicant (excluding actor); a new unsubmitted draft has no personal reminder. Notification progress remains a read DTO; original request results contain no recipient receipt.

No new schema, migration, initializer, background writer, separate recovery flow or temporary structure was introduced. Goose SQL/schema 00001–00008 are byte-for-byte unchanged (16 files in `immutable-migrations.json`); baseline remains 5. Existing legacy APPROVER storage cutover remains #98. `docs/admin-notification-center.md` now documents the completed lifecycle contract; existing glossary/ADR/design rules remain accurate and needed no semantic changes.

## Acceptance and affected checks

| Scope | Evidence | Final valid result |
| --- | --- | --- |
| AC-019 actual participants, default ADMIN, removed members, duplicate roles, own unread, terminal original results | `ac019-final.log` publication and terminal cases; `ac020-faults-http.log` reprepare case | 3 unique MySQL cases PASS; original ac019-final batch includes a repaired draft-display fixture failure |
| AC-020 notification UPDATE failure, no partial data/results/versions/two notification types; exact replay / changed-body rejection | `ac020-faults-http.log` persistence case; original persistence matrices in `affected-regression.log` | PASS |
| AC-020 actual HTTP response loss, all four operations | `regression-repaired.log` LostResponses case (final refactored test) | PASS, four subcases |
| AC-020 actual MySQL COMMIT OK loss, all four operations | `regression-repaired.log` CommitUnknown case | PASS, four consumed COMMIT OKs; no false failure history |
| AC-020 complete vs rollback, distinct-key rollback, same-key rollback, real simultaneous MySQL requests | `ac020-commit-concurrency.log` TerminalCompetition case | PASS, three subcases; one terminal state and one notification advance |
| Disabled actual reviewer receives publication/complete/rollback while disabled and reads it after formal enable/login | `affected-regression.log` DisabledParticipants case | PASS, two original orders, sequence 3 and handled history |
| Affected existing lifecycle/approval/constraint/persistence/authorization/reprepare concurrency | `regression-selection.txt`, `affected-regression.log`, `regression-repaired.log`, `cancellation-notice-verified.log` | 20 unique cases: original 18 PASS / 2 FAIL; both fixed with passing current evidence |
| Go domain/application/MySQL/HTTP/admin packages, including contract checks and external process runtime | `go-targeted-verified.log` | 5 packages PASS |
| Web release page, original request journal, release API, notification provider/center | `web-targeted-verified.log` | 5 files, 99 tests PASS |
| Web typecheck and production build | `web-typecheck-verified.log`, `web-build-verified.log` | PASS |
| Formal browser result jumps, old operations after true committed response loss, desktop/390px/keyboard | `browser-initial.log`, `browser/initial/result.json` | 1 runner PASS, 4 lifecycle checks, 14 actual detail ACKs, errors `[]` |

`mysql-results.json` resolves all **27 unique MySQL cases** to valid final PASS evidence; duplicate repetitions and subcases are not counted as extra independent cases. This is a ticket-scoped affected matrix, not the final whole-repository matrix assigned to #98.

The formal browser runner is **`TestReleaseNotificationsBrowserSystemPath`**, which executes **`web/e2e/release-notifications.cjs`** against a real Admin executable, same-origin Vite and disposable MySQL. The historical root browser script does not automatically include this new script: #98 must explicitly include the runner in its final matrix. Its four checks are publication, completion, original-order quick rollback, and administrator reprepare. Every loss uses `route.fetch()` to receive the real committed success, then aborts delivery, reloads, and resends through the original business UI with the identical key and body. Before/after comparisons include actual rows and versions, history, execution/downstream refresh summaries and every participant's personal sequence. The new draft is absent from notification-center views and has sequence 0.

The implementer inspected publication desktop detail, rollback mobile detail, completion mobile detail and reprepare mobile uncertain-result screenshots. Long names and permanent IDs wrap, actions remain available, result tables scroll locally, and source links/uncertain-result wording remain readable. All 16 emitted layout measurements have document width equal to viewport (1440 or 390). The Spec reviewer independently inspected publication and reprepare mobile screenshots. No production Web/visual rule was changed.

## Original failures and valid recovery

All original raw logs remain intact, including filenames from intended green/final attempts that actually failed. No failing batch is called green.

- `ac019-publication-compile-failure.log`: new test used nonexistent `ReleaseExecution.Tables`; corrected to public `TableVersions`.
- `ac019-publication-fixture-failure.log`: public default decision source is `ADMIN`, not the invented `DEFAULT_ADMIN` value; corrected fixture check.
- `ac019-publication-red.log`: valid feature red, applicant sequence did not advance after actual publication. Publication hook then added.
- `ac019-publication-persistence-check-failure.log`: publication behavior passed, but new read-only SQL check incorrectly assumed a status column; existing refresh status is in its JSON document. Corrected check; final publication PASS in `ac019-final.log`.
- `ac019-terminal-red.log`: completion correctly failed for missing personal result; rollback fixture first lacked reverse DELETE policy. `ac019-results-green.log` passed completion but retained that rollback setup failure. Added the real reverse policy; both terminal subcases PASS in `ac019-final.log`.
- `ac019-reprepare-red.log`: valid feature red, original reprepare returned 201 despite a failing notification trigger because no hook existed. Reprepare hook then added.
- `ac019-final.log`: reprepare fault already rolled back properly, but the draft response's dynamic approval arrangement was mistaken for a persisted submitted approval. Removed that false assumption while retaining draft state, no reminder rows, target transfer, source history and exact replay checks. Reprepare PASS in `ac020-faults-http.log`.
- `affected-regression.log`: 18 PASS / 2 FAIL. The cancellation fixture waited for an obsolete release-order lock; now it gates incoming HTTP body after real authentication, then observes the cancellation connection waiting on the publication connection's shared authorization lock. All original exact final-state/history assertions remain. The original-publication replay fixture compared the changing current `ApprovalContext` to an old context; it now explicitly compares that field with current GET while comparing every business field to the original result and verifying durable request JSON is byte-for-byte unchanged. Both PASS in `regression-repaired.log`.
- `cancellation-notice-actor-fixture-failure.log`: after strengthening the race to another ADMIN's cancellation, the final comparison accidentally mixed account-specific contexts. Reading both objects as the same cancelling ADMIN fixes the fixture without removing fields. `cancellation-notice-verified.log` PASS retains complete object equality, cancellation-before-failure ordering, one applicant cancellation notification, zero reviewer success notification and all business/target checks.
- `go-targeted.log`: four packages passed; admin runtime tests could not bind local TCP under sandbox. All five packages passed with permitted local listening in `go-targeted-verified.log`.
- `web-targeted.log`, `web-typecheck.log`, `web-build.log`: initial reused node_modules lacked `fake-indexeddb`. Repointed only the untracked symlink to #96's already-installed locked dependencies; no lockfile changed. Verified logs pass. `dependencies.json` records identical versions/manifests for runtime/browser packages, so this test-only dependency repair does not invalidate the earlier browser run.
- Standards review's one P3 subjective duplication observation was fixed by sharing preparation only. The independent HTTP-loss and COMMIT-loss transports and assertions remain; both refactored tests PASS in `regression-repaired.log`. The original observation and resolution remain in `standards-review.md`.

Result lifecycle recipients always have their aggregate created by preceding submit/approval, so these fault tests target the actual UPDATE path. #96's unchanged submit and qualification INSERT handling is not reintroduced as a second result mechanism; the previous adapter INSERT evidence remains applicable.

## Review, provenance and resources

Independent Standards and Spec reports are adjacent, with 0 unresolved findings after fixes. They reviewed the fixed base plus working-tree additions because local commit is permitted only after checks/review. `source-manifest.json` lists SHA-256 by repository-relative path for the final source (600 files), excluding verification artifacts, symlinks and dependencies. `source-before-final-race-manifest.json` preserves the earlier review snapshot. `evidence-manifest.json` lists every other artifact relative to this evidence directory and excludes itself to avoid recursive hashing.

All four production notification changes were present before `ac019-final.log` and the formal browser run; subsequent changes were tests, test fixtures or documentation. Final repaired tests and source manifests bind those changes. No accepted AC result was removed or replaced with an unverified weaker assertion.

MySQL integration, COMMIT proxy and browser runners ran sequentially with at most one task-owned MySQL; each genuine business race retained multiple simultaneous connections within its own fixture. `resource-verification.json` matches created container IDs to termination logs and a final read-only Docker inventory. User containers and other tasks' old resources were untouched. No Compose deployment or reset was run. `web/node_modules` is a local symlink excluded from commit; its target is recorded in `dependencies.json`.

Raw vendor stdout is preserved byte-for-byte, including trailing spaces and terminal blank lines. The staged source/artifact whitespace check excludes only raw `.log` files; `static-check-result.json` records that exact successful check.
