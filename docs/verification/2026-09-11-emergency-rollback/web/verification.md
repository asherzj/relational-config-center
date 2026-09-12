# Issue #105 Web verification

Scope: `web/**`; no database/container/browser process was started by the Web helper. Browser script execution and visual evidence are owned by the parent agent. All test mocks are HTTP/IndexedDB platform boundaries; no owned application collaborators are mocked.

## Vertical slices

- `01-preview-red.log`: the new public Web behavior fails before implementation because the preview has no durable idempotent request or displayed persisted per-table emergency nodes.
- `02-preview-green.log`: the same behavior passes after implementation; the original preview request is recorded before HTTP and rollback uses saved version 5, not pre-save version 4.
- `06-preview-contract-red.log` / `07-preview-contract-green.log`: mismatched successful preview for another order is rejected by the public API adapter.
- `04-preview-retry-regression.log` preserves an intermediate fixture-timing failure; `08-preview-retry-regression.log` is the corrected nine-test recovery/rollback run. The pending HTTP response now waits for IndexedDB persistence before the test can resolve it.
- `09-preview-display-permissions.log`: two supplementary scenarios passed immediately using the implemented shared behavior (real per-table node rendering and current-role revocation/restoration). No artificial red was introduced.
- `14-preview-conflict.log`: supplementary preview version-conflict scenario passed, preserving reason input and explicitly rebuilding only after reading the latest version.

## Final affected verification

`16-affected-regression-final.log`:

```sh
pnpm --dir web test:run src/features/release-orders src/api/release-orders.test.ts src/features/notifications src/features/managed-data/ChangeSetDialog.test.tsx src/features/managed-data/ManagedDataMutationPage.test.tsx
```

Result: 9 files, 178 tests passed. Includes publication, rollback, preview persistence/replay, rejection/rebuild, current authorization, notifications and affected draft/Change Set writers. The original `10-affected-regression.log` retains five failures caused by the notification detail fixture lacking required `rollback_table_flows`; `13-notification-contract-fixtures.log` proves the fixture update (13 notification tests), and the final full affected run passes.

- `11-typecheck.log` and `18-final-typecheck.log`: `pnpm --dir web typecheck` passed.
- `12-build.log`: `pnpm --dir web build` passed.
- `17-browser-script-syntax-final.log`: `node --check web/e2e/release-rollback-flows.cjs` passed. This is syntax verification only, not browser acceptance.
- `git diff --check -- web` passed.
- `source-sha256.json` records the implementation/tests/script snapshot supplied for parent verification.

The helper confirmed byte-identical `web/pnpm-lock.yaml` before creating its own `web/node_modules` symlink to the existing real directory `/private/tmp/rcc-issue-103-release-instances/web/node_modules`. The symlink is temporary and must not be committed; removing it must never remove its target.

## Browser script handoff

`web/e2e/release-rollback-flows.cjs` requires a fresh disposable template fixture with `policy_alpha` and `policy_beta` (`id`/`value`), the `template.browser.admin` account (or explicit `RCC_E2E_ADMIN_USERNAME` / `RCC_E2E_ADMIN_PASSWORD`), `RCC_E2E_ISOLATED=1`, `RCC_WEB_URL`, and a fresh existing `RCC_E2E_OUTPUT` directory. It uses public registration and the authenticated account-role API for temporary EDITOR/PUBLISHER accounts, so it does not require a maintenance CLI.

The script covers standard-origin rollback with distinct emergency templates; saved preview response loss, close/reload and keyboard original-key replay; source-template changes and fresh-key reopening without replacing instances; an empty-reason whole-order reversal that directly ends the original order; actual restoration node actors and stopped completion nodes; later reason editing; applicant result notification keyboard navigation; and historical preview replay after both COMPLETED and ROLLED_BACK emergency-origin orders. Each stage writes desktop/390px screenshots and layout JSON, plus publisher/applicant trace ZIP files and original request/snapshot evidence.

Existing browser call sites requiring parent coordination: `release-rollback-reason.cjs` executes using `published.version` after preview and must use `preview.expected_version`; `release-rollbacks.cjs` completes a competing order using its pre-preview version and must reread current version. Their request helpers already send `Idempotency-Key`. The helper did not modify or run those scripts.

## Limits

No commit/push, issue update, Notion update, backend execution, browser execution, or visual approval is claimed by this helper. Shared design rules were updated in `web/DESIGN.md`; no other design/architecture document was modified.


## Standards follow-up: shared recovery window

The initial review identified an actual mismatch: the generic “查看最新状态与配置” button could persist or replay a rollback preview. That write was removed. The generic conflict entry now only reads the actual current header/details and offers “打开恢复预览”. The shared QuickRollbackDialog owns preview save, original-key retry, explicit preview-conflict rebuild, and the final explicit “确认按最新状态快速回滚” execution rebuild. The original execution reason/body/key remains intact until that final confirmation. The dialog observes the normal current-header query so saved-version advancement and later permission/state changes remain current even when opened from conflict review.

`ReleaseRequestIntent.tsx` contains the unchanged request-intent renderer and reason reader extracted from ReleaseRequestReview to avoid a new Review/Dialog circular dependency. The old module re-exports both names for existing consumers.

- `20-conflict-shared-window-red.log`: initial new scenario fails because the shared-window entry is absent.
- `21-conflict-shared-window-red.log`: after waiting for the read action to finish, the same scenario independently fails because the read-named button sends one preview write (expected zero).
- `22-conflict-shared-window-green.log`: read-only generic review → shared dialog → lost preview response → remount → read-only review → explicit same-key/same-body/same-version retry → original execution request still retained → explicit new-key execution with saved version all pass.
- `23-conflict-typecheck.log` preserves an intermediate type mismatch: the edit aggregate omits notification in its public type. The fix carries the notification from the actual fetched header, with no schema relaxation or synthesized value.
- `24-conflict-affected-regression.log`: the complete previously affected nine-file scope passes after the correction (179 tests).
- `25-conflict-typecheck.log` and `26-conflict-build.log`: typecheck and production build pass.
- `27-conflict-source-sha256.json` records the exact four-file follow-up snapshot. The original source snapshot is retained as history.

The helper changed no backend/browser test script in this follow-up and did not run database or browser processes. Browser acceptance must use this corrected Web source and build; the parent owns execution.


A final boundary check found that a rejected execution must not prevent confirming an unresolved preview after the order later completes. `28-conflict-terminal-preview-red.log` fails because no original preview was sent; `29-conflict-terminal-preview-green.log` passes after excluding unresolved original-preview replay from the new-intent state gate. The current header still prevents rebuilding an execution for the completed order, and the original rejected execution remains available for inspection.

Final evidence for this one-line recovery-only change: `30-conflict-final-recovery-regression.log` passes all 14 matched recovery/rollback/instance scenarios (82 unrelated tests skipped); the other successful scenarios from `24-conflict-affected-regression.log` remain applicable. `31-conflict-final-typecheck.log`, `32-conflict-final-build.log`, and `git diff --check -- web` pass. `33-conflict-final-source-sha256.json` supersedes the four-file follow-up snapshot for the final corrected source. The helper has finished writing these four files and the corresponding verification records.
