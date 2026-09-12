# Backend merge verification

Scope: integrate the existing #93/#94 backend into #101/#102, without #103 flow instances or #104 emergency execution. Source worktree is `codex/release-template-approval-integration`. The two parent commits and final five owned file SHA-256 values are recorded in `final-source-files.json`.

Original intent was read from GitHub issues #93 and #94 including all comments and labels (`issue-93.json`, `issue-94.json`), the committed ticket/spec clarification documents, and both source histories. Initial sandbox GitHub reads failed; authorized read-only retries succeeded. No GitHub writes occurred.

## Source integration

Resolved `application.go` and `security.go` by preserving both ApprovalRoles and ReleaseTemplates/TableReleaseTemplates. Approve/reject retain the external VIEWER coarse gate with authoritative per-table qualification. The control-table protection test retains both sets of new tables and explicitly covers the two table-approval control tables.

Auto-merged `router.go` retains all catalogs plus #102 table-policy version/Idempotency-Key operations. `release_history_delivery_integration_test.go` retains real role assignment, approval scope/revision requests, and the #102 observed table version for disabling. Other external approval/session/transaction changes remain intact.

The MySQL checks identified two existing test assumptions incompatible with #94. Only those test bodies were corrected; production code, schemas, fixtures shared by other tests, and migration manifests were unchanged during the checks:

- `TestRevocationRejectsNewRequestsButAllowsAuthenticatedWriteToFinish`: #94 serializes release execution with revocation using `rcc_auth_control_lock`. The original synchronous logout waited for a lock held by publication while the test itself withheld the publication's order lock. The corrected test starts logout concurrently, confirms its real `performance_schema.data_lock_waits` wait on the authorization control row with a bounded deadline, releases the order lock, verifies the authenticated publication and logout complete, and verifies a subsequent old-session request gets 401. Publisher attribution and the actual saved business row remain asserted.
- `TestReleaseHistorySurvivesExecutableRestartAndExternalChanges`: field-level diagnostics across all seven states showed differences exclusively in the live `ApprovalContext`. All frozen approvals/decisions, items, events, and executions were unchanged. The final test compares all historical fields exactly and separately verifies a fresh approval revision, no viewer approval privilege, and ADMIN fallback for draft/pending orders after the reviewer is disabled. It continues to compare the entire same-viewer response, including live context, before and after rejected deletion/forged updates.

## Validation and result reconciliation

The planned 19 top-level cases have effective passing coverage. This is a reconciled result from the original batch and the two affected rechecks, not a claim that one 19-test batch exited successfully. Commands, environment, raw Go JSON, human-readable output, exit codes, and individual tests are retained. `mysql-effective-coverage.json` maps every planned test to its passing evidence.

| Run | Result | Evidence |
| --- | --- | --- |
| Application and domain units | Both packages passed | `application-domain-unit.txt` |
| HTTP units | Package passed | `http-unit.txt` |
| Initial 19-test attempt | Exit 1: Docker provider unavailable; no business assertions ran | `mysql-selected.jsonl`, `mysql-selected.log`, `mysql-selected-result.json` |
| 19-test batch with explicit Colima environment | 17 passed, 2 failed; package 184.024s, exit 1 | `mysql-selected-recheck.jsonl`, `mysql-selected-recheck.log`, `mysql-selected-recheck-result.json` |
| Two affected cases after revocation correction, with history diagnostics | Revocation passed 9.16s; history failed only on live context; package 24.245s, exit 1 | `mysql-fixture-diagnostic.jsonl`, `mysql-fixture-diagnostic.log`, `mysql-fixture-diagnostic-result.json` |
| History after its fixture correction | Passed 13.71s; package 14.934s, exit 0 | `mysql-history-recheck.jsonl`, `mysql-history-recheck.log`, `mysql-history-recheck-result.json` |
| Owned source diff and formatting | `git diff --check HEAD` passed; Go files formatted | `final-diff-check.txt` |

The selected cases cover six external role-directory/table-approval behaviors, six #102 table-template/table-policy concurrency and request-identity behaviors, response-loss idempotence, session/CSRF enforcement, actor attribution, operator/history protection, revocation serialization, control-table isolation, and real executable history persistence. The unchanged T1 template transaction suite was not repeated. Root separately owns schema migration verification and browser acceptance.

Successful MySQL runs used `TESTCONTAINERS_RYUK_DISABLED=true`, `DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock`, and `GOCACHE=/private/tmp/rcc-go-cache` from the worktree's `admin` directory. All 22 containers created by these runs were logged terminated and absent from the final Docker snapshot. Four unrelated Created/old Exited containers were left untouched; see `mysql-owned-container-cleanup.json` and `mysql-cleanup.json`. The exclusive database slot was released after these checks.

## Source provenance

- `source-files.json` and `resolved-owned.diff` preserve the initial five-file merge resolution.
- `mysql-source-manifest.json` records all 224 backend Go/SQL/JSON/module inputs at the 19-test batch.
- `mysql-fixture-diagnostic-source.json` records the two test files during the diagnostic run.
- `mysql-final-source-manifest.json` records final inputs and the exact two changed test-file hashes. Every other input remains byte-identical. The passing revocation test body is unchanged from the diagnostic run, so its pass is reusable.
- `final-source-files.json` and `final-resolved-owned.diff` record the final five owned files.

Only the five assigned backend files and this new evidence directory are staged by this agent. No commit, push, schema/ADR/Web edit, or modification to earlier verification evidence was performed. An initial edit-script indentation error left source unchanged and was corrected before successful formatting/unit checks; it is not counted as a product test result.

Raw human-readable logs and git diffs retain their exact original bytes, including output whitespace. `raw-output-manifest.json` records their SHA-256 values. Source/document whitespace validation explicitly excludes `.log` and `.diff` output artifacts; it does not normalize or reinterpret recorded evidence. A temporary compression attempt was reversed and every restored file was verified against its original SHA-256 before final staging.
