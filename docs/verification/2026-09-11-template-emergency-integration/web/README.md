# #98 + #104 Web integration verification

## Source provenance

- Integration target before the merge: `e7e19b022fc606c137fdf8df1899caf9a5702bb1` (published #98 already integrated).
- Incoming delivered #104: `f545adf24aadd34ee39afb2643a4819aee54c986`.
- Actual Git merge base: `e2564842e72e96b6ffb1e86708978b11dd8a651b`.
- `00-pre-resolution-conflicts.log` records the untouched Web conflict markers and index entries before manual resolution.
- `08-source-sha256.txt` records every final Web path changed by the in-progress merge, including clean incoming changes, manual conflict resolutions, and browser-fixture adapters. `05-source-sha256-before-script-adapters.txt` preserves the earlier hash point.

The manual merge keeps the #98 compact title header, action priority, More menu, collapsible identifiers, person-ID disclosure, difference layout, and notification-read state. It adds #104 release types, required `emergency_reason`, `PENDING_PUBLICATION`, persisted emergency flows, neutral frozen-content wording, emergency submit/rebuild behavior, and hides approval summaries for emergency orders. `releaseHeaderSchema` retains the current user's `notification`; `releaseOrderSchema` deliberately omits it from historical/write results.

## Commands and results

1. Affected Web tests, first integration run:

   `pnpm exec vitest run src/api/release-orders.test.ts src/features/release-orders/ReleaseOrdersPage.test.tsx src/features/managed-data/ManagedDataMutationPage.test.tsx src/features/managed-data/ManagedDataPage.test.tsx src/features/notifications/NotificationsPage.test.tsx src/features/notifications/ApprovalNotifications.test.tsx`

   Vitest reported 2 failed files, 9 failed tests, and 145 passed tests. All nine failures came from the two notification test fixtures missing the newly required `emergency_reason` field. The command wrapper reached its 30-second capture limit after Vitest printed the final report, so its shell exit code was not captured. The complete unchanged output is `01-affected-tests-first.log`.

2. Same affected Web tests after adding the required field to the two consumers: exit `0`, 6 files and 154 tests passed. Output: `02-affected-tests-green.log`.

3. `pnpm typecheck`: exit `0`. Output: `02-typecheck-first.log`.

4. `pnpm build`: exit `0`; TypeScript and Vite production build passed. Output: `03-build.log`.

5. Initial `node --check` for `release-instances.cjs`, `release-emergency.cjs`, `notification-center.cjs`, and `release-notifications.cjs`: every command exited `0`. Output: `04-e2e-syntax.log`.

6. Static browser-fixture review found three #97 notification scripts still enabled newly created table policies with the obsolete empty body and relied on the default emergency association. Each fixture now enables with the created policy version, reads the current release associations, and explicitly binds `default_standard_v1` with the returned association version. The #98 layout adapters also replace ReleaseFlow's old “添加明细” selector with “添加变更” and locate the rejected reviewer's identity from persisted approval progress instead of the removed forward step list. Final `node --check` for all six touched browser scripts exited `0`. Output: `07-e2e-syntax-after-fixture-adapters.log`.

7. `rg -n '^(<<<<<<<|=======|>>>>>>>)' web` and `git diff --check -- web`: no output after resolution.

The first source-hash command failed before producing hashes because the loop used zsh's reserved `path` variable and thereby cleared command lookup inside that subprocess. No source file was modified by that failure. It was corrected with a non-reserved variable and absolute `/usr/bin/shasum`; the final hashes are in `08-source-sha256.txt`. Raw log hashes are in `06-log-sha256.txt`.

No database or browser-system test was run in this Web work window. The four scripts were only checked statically and syntactically; the parent integration task owns their isolated runtime execution and screenshot review.
