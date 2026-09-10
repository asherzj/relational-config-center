# Spec re-review — candidate 2

Reviewed staged tree `759ae638fe41d0c082ad9757c391d342e5402130`, the repair delta from candidate 1, and the established feature/main/AUTO_MERGE boundaries (`e626885` / `9aa4bdd` / `b2ef8c6`). All 726 files in `source-candidate2.json` match current contents; unstaged/untracked inspection is empty. No source edits, tests, builds, or database operations were performed.

**Previous findings closed:**

- **P2 documentation:** active specification and plan status, all seven ticket headers, ticket README, ADR-0025, and approval/upgrade contracts now describe the implemented local scope and explicitly retain ongoing final verification and unmerged/undeployed status. Approved AC definitions remain unchanged. Final evidence indexing and project-record updates remain delivery prerequisites, not claims already completed.
- **P3 standalone rollback residue:** `admin/internal/interfaces/http/security.go:205`, `web/src/features/release-orders/release-journal.ts:6`, and `useReleaseWrite.ts:58` no longer recognize the retired route/error. Legitimate quick rollback, optional reasons, and known-conflict review remain. Route regression assertions now require 404.
- **P2 omitted-table compatibility:** `admin/internal/application/draft_details.go:40` no longer fills a missing upsert table; `release_orders.go:851` rejects missing copy/reprepare tables. The new public HTTP test exercises all three paths and snapshots all eight release-owned tables to prove rejection leaves workflow, details, requests, targets, and technical records unchanged.

**Identity repair accepted:** `admin/internal/infrastructure/mysql/publication.go:221` copies the original stable detail identity into each Command; reverse preparation preserves that identity. `admin/internal/domain/publication.go:122` and `web/src/api/release-orders.ts:46` enforce matching nonempty identities. Canonical row ID, frozen schema/checksum, execution/table attribution, and rollback actual-ID checks remain. This satisfies AC-001/AC-004/AC-015 without approximating MySQL equality, adding storage structures, or introducing compatibility.

`detail-binding-red1.jsonl` records the expected two failing parents/six negative subcases; `detail-binding-green1.jsonl` records those repaired cases plus both unchanged FLOAT identity families and old-version readiness: five top-level PASS, no failures/skips. Same-table/same-operation/same-execution result exchange and absent publication/rollback detail identities are rejected. Fixture changes preserve failure-history and historical/offline boundaries. All 22 protected published files still match main.

**New Spec findings: 0.** Final acceptance remains open pending the 131-family/broad package reconciliation, real three-engine browser scope, six Compose scenarios, and final evidence/project records. This review does not declare those checks passed.
