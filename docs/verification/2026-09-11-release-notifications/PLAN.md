# #97 implementation and acceptance plan

Base: 12fda54fc3c2f74e09e7dbde13a6e723d6a98e33. Issue 97 and parent 92 full snapshots are adjacent; no comments were present. The parent-approved seams are formal Web, authenticated public Admin HTTP with real MySQL, and necessary read-only persisted fact checks / boundary fault injection.

1. AC-019 publication participants: applicant and saved table-decision actors, deduplicated by permanent ID, excluding current actor; default ADMIN and later removed members retained. Start with failing real HTTP test.
2. AC-019 complete and original-order rollback: corresponding success changes and notifications in original transaction. Same-order aggregation; completion creates no new execution, command or downstream refresh. Preserve actor's past unread outcomes.
3. AC-019 reprepare: source cancellation notification, replacement draft, target transfer and original request result commit together. Draft has no review notification.
4. AC-020 real notification persistence faults and existing persistence/constraint matrices; exact original request replays / different-body conflicts; real COMMIT OK loss, HTTP response loss and terminal races. No success notification from failure history or GETs.
5. Formal browser desktop / 390 px / keyboard result access and original-action response-loss retry. No separate recovery flow.
6. Targeted affected regressions, independent Standards and Spec reviews, current source/evidence manifests, local commit. Root owns push, integration, closure and Notion.

Resources: only one task-owned MySQL fixture at a time across Go/browser/Compose. Existing user containers and other tasks' leftovers remain untouched. Business races use simultaneous real connections inside the single fixture. `web/node_modules` is an untracked symlink to preinstalled dependencies and excluded from staging/manifests. No migration, baseline, or legacy APPROVER cutover change expected.
