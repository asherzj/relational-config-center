# PR #109: follow the copied-from link without interrupting the new draft

Base: `73364a352caf5258d48f9391ed7a95d62bec6d53`. The only source change is `web/e2e/release-multitable.cjs`: after a successful copy, use the new draft's actual “基本信息 → 复制自” link to return to its source. The final source retains the original full suite, every business assertion, the zero-page-error assertion, all timeouts, and the earlier bounded click diagnostics.

## Formal failure and scope

Run `34666747432` passed 24 browser invocations, failed WebKit multitable, and did not execute the final nine invocations. Both observed Change Set clicks passed (563/585 ms), and all ten multitable business checks passed. The final assertion then found two page errors:

- `/api/v1/approval-notifications due to access control checks.`
- `/api/v1/release-orders/736b1ab66513215ac9e1c6dac2894f0e due to access control checks.`

The preserved fourth-run result maps `968f82f56722e08dfc642da1ff8cbbf6` to the original publish/rollback journey. Its later browser detail-request sequence places `736b1ab66513215ac9e1c6dac2894f0e` at version 1 immediately after the source-copy path `71b917277bf62868a2f3a29337483aaa` at versions 3/4, and before reprepare source `04d77133296cd5b07423f09c71f5f5ce` and new reprepare draft `1985c3eddd832e2eb4e8b7d48ef9621a`. Given the completed script's navigation sequence, this identifies the failing order read as the newly copied draft. The formal result does not timestamp those page errors, so it cannot alone establish when the notification error happened.

The earlier intermittent 20-second unstable-click failure remains separately unexplained. Passing clicks in this run are not evidence that it was repaired.

## Feedback loop and observed boundary

A passive local full-suite attempt added only page IDs, explicit copy/reprepare IDs, navigation boundaries and request/response/failure timestamps. It stopped before the relevant return navigation when Linux WebKit closed the target during a second copy-preview click. Its `page_errors` was empty. The server container remained running, with no captured OOM/crash explanation. This is retained as an independent incomplete attempt, not a reproduction of the fourth-run page errors.

The next bounded loop used real Admin/MySQL, the same two table policies and a rejected two-table source. It repeated only copy → read newly copied order through the fixture API → original `page.goto(source)` → real forward link → confirm copied-from association. Each copy was explicitly cancelled afterward to release its own targets. It ran exactly 20 iterations and then stopped. All 20 passed the original page-error assertion, but request timing showed that the immediate document navigation could cancel the newly copied draft's still-active reads; one first-iteration `/details` request failed with `Load request cancelled` immediately after the hard navigation.

To distinguish the navigation boundary from successful fast responses, a three-iteration comparison delayed only actual successful notification/new-order GET responses by 500 ms using `route.fetch()` followed by `route.fulfill({response})`. It did not abort requests, change response status/body/headers, alter credentials/roles, or extend any timeout. The recorded server responses were HTTP 200. This models slower reads in a controlled browser-routing fixture; it is not an assertion about actual CI latency.

In the old path's first iteration:

| Event | Relative time |
| --- | --- |
| New copied draft | `13edb0275d8ff5233825e6058802c977` |
| Notification/new-order browser GETs start | 5867 / 5868 ms |
| Fixture API read finishes and `goto(source)` starts | 5885 ms |
| Both GETs fail with `Load request cancelled` | 5891 ms |

There were three matching cancellations per iteration (two notification reads and one copied-order read), nine across three iterations. The replay checker in `probes/check-copy-navigation.cjs` fails on this captured trace. With only the copied-from UI navigation substituted, the same three-iteration delayed-response loop recorded zero matching cancellations and the same checker passes. Both before and after runs had `page_errors: []`; the **red/green signal here is the proven request-cancellation boundary**, not a claim to have reproduced the exact CI page-error message.

These observations confirm that the old fixture can tear down a newly entered document while its reads are active, despite successful same-origin server responses. The new path removes that demonstrated teardown/cancellation path and exercises the reverse association itself. They do **not** establish WebKit's low-level mechanism for reporting the fourth-run errors. The exact access-control page-error text was not reproduced locally, and other explicit navigations are not changed on speculation.

## Final change

After reading the copied order, the suite waits for its “复制自” metadata, expands “基本信息”, clicks the link whose accessible name is the exact source order ID, and waits for that source URL. It then follows the existing forward link back to the new copy and retains the original copied-from, table and detail-ID assertions. The change strengthens the real bidirectional-link journey instead of substituting an address-bar navigation for the reverse link.

All temporary request interception, delays, timeline listeners and loop code are removed from the final E2E source. Their snapshots remain only in `probes/`, with original raw results beside them. No application authorization, fetch behavior, UI design, shared recovery helper or prior verification file changed. `web/DESIGN.md`, root `CONTEXT.md` and multitable ADR 0025 were read; no shared design/domain rule needs an amendment.

## Commands and environment

The same disposable real-backend runner provides the production Web proxy, account maintenance and MySQL 8.4. Chromium and Firefox run on macOS; WebKit 26.5 runs on Linux arm64 in the cached official `mcr.microsoft.com/playwright:v1.62.1-noble` image (`sha256:dcc5531e97840b9b5e794f2814476b21571c5124a3fca2267d73041f56e7580e`), connected by Playwright 1.62.1 with loopback forwarding. The browser server is bound only to a random host-loopback port with a read-only worktree mount. GitHub's all-Linux amd64 environment remains a distinct validation boundary.

```sh
node probes/check-copy-navigation.cjs delayed-hard-navigation/webkit/navigation-timeline.json # expected nonzero: 9 cancellations
node probes/check-copy-navigation.cjs delayed-link-navigation/webkit/navigation-timeline.json # exit 0: no cancellations
```

The browser probes were temporarily placed at the normal E2E script path; they are archived diagnostic snapshots, not extra CI tests. The final frozen full-suite command is:

```sh
NODE_OPTIONS='--require=/tmp/rcc-pr109-navigation-connect.cjs' RCC_WEBKIT_WS_ENDPOINT=ws://127.0.0.1:32772/ RCC_E2E_SUITE=release-multitable RCC_E2E_ENGINES=chromium,firefox,webkit RCC_E2E_ARTIFACTS=/tmp/rcc-pr109-navigation-final-shared bash scripts/browser-acceptance.sh
```

This final command runs the complete multitable suite in shared Chromium → Firefox → Linux WebKit order. It does not rerun all 34 formal browser invocations. A later green CI establishes its current acceptance result; it does not prove the low-level causes of either earlier intermittent WebKit failure.

## Frozen-source final verification and cleanup

`final-shared/run.txt` records the final source run from `2026-09-12T02:36:55Z` to `2026-09-12T02:39:40Z`: exit **0**, database post-check passed and cleanup verified. Chromium, Firefox and Linux WebKit each pass all ten original business checks with `page_errors: []`. Their successful bidirectional-copy checks follow the changed real link; their original independent approval, 25-item publication/rollback, 9 MiB same-key/same-order recovery, account isolation and zero-write storage-failure assertions remain intact. `final-shared/verified-summary.json` is derived directly from those three results.

The final WebKit 390px restoration screenshot was inspected and retains the actual restored value and scrollable comparison. It is existing end-to-end business evidence, not a screenshot of the earlier missing page-error timing. Both fixed-target clicks also pass, without claiming to resolve the previous intermittent unstable-click cause.

`node --check web/e2e/release-multitable.cjs` and `git diff --check` pass. The runner also passes production build/typecheck and its timeout-cleanup test. The final source SHA-256 stayed fixed throughout all three engines. Its only diff is five added lines and one removed line in the copy-return navigation; the earlier click diagnostic helper is unchanged.

All real runs used at most one owned MySQL container alongside the owned Linux WebKit server. Each runner verified its named database container/volume cleanup. The owned server `rcc-pr109-navigation-linux-webkit` was removed after final verification, and read-only Docker inventories found no matching task resources. Existing unrelated containers, volumes, the Colima VM and cached images were not changed.

`source-sha256.txt` binds the final candidate and `SHA256SUMS` covers the evidence files. Git status contains only this source modification and this new directory; earlier verification bytes remain unchanged. The diagnostic agent did not commit, push, or write externally.
