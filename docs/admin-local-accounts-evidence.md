# Local Account acceptance evidence (PM-009 / #34, delivery #40)

The 31 acceptance cases below refer to executable tests in this repository, including
all earlier slices. They are checked on the complete final code, not inferred from
owner-ticket status. No mail operations, roles, enterprise identity, audit, MFA,
public deployment or rolling-upgrade promise is included.

## Reproduce

Run from the repository root with Go, Node 24.19.0, pnpm 10.28.2 and Docker/MySQL 8.4.
Install Web dependencies with `pnpm --dir web install --frozen-lockfile`. Browser
checks use installed Chrome or `RCC_BROWSER_EXECUTABLE` pointing to Chromium.
On the recorded Colima host set both environment variables for integration commands:

```bash
export DOCKER_HOST=unix:///Users/asher/.colima/default/docker.sock
export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
# G: all Go modules and distributed binaries
make test && make build
# W: actual UI/API client contracts
pnpm --dir web test:run --maxWorkers=1 && pnpm --dir web typecheck && pnpm --dir web build
# I: complete real MySQL suite (JSON exposes all test-level skips/failures)
go test -json -count=1 -timeout=20m -tags=integration ./admin/...
# B: separate real browser system path, fails on missing prerequisites
make test-browser
# P: focused process/upgrade reproduction (also included in I)
go test -v -count=1 -timeout=5m -tags=integration ./admin/cmd/admin -run '^TestAccount(Process|Upgrade)'
```

A `skip` event with a `Test` name is not acceptance evidence. Package `[no test files]`
is different. G/W/I/B must all pass. The browser target starts a new MySQL container,
delivered Admin binary, Vite same-origin proxy and temporary Chrome profile, then cleans
only its own resources. Registration and login always use the public HTTP flow;
SQL in these tests sets explicit faults/capacity/time states or verifies persisted
contracts that safe HTTP responses intentionally cannot disclose.

## 31-case map

All Go names are under `admin/cmd/admin`; Web names are under `web/src`.
`account_integration_test.go` = AI, `business_auth_integration_test.go` = BI,
`account_delivery_integration_test.go` = DI, `account_maintenance_integration_test.go` = MI.
These abbreviations identify files, not substitute tests. Each named test runs via
I/P, each named Web suite via W, and the browser test via B.

| AC | Observable promise | Executable evidence |
| --- | --- | --- |
| 001 | Empty accounts start healthy and show login/register; no default identity | DI `TestAccountProcessRequiresCompleteAuthenticationSchema`; BI `TestBusinessAPIsRequireSessionAndCSRF`; `features/accounts/WorkspaceAccess.test.tsx`; B |
| 002 | Public registration enables, signs in and returns real unverified identity | AI `TestLocalAccountRegistrationCreatesCurrentIdentity`; B |
| 003 | Concurrent normalized username/email collisions have one atomic winner | AI `TestLocalAccountNormalizedUniquenessAndAtomicRegistration` |
| 004 | Required fields and Unicode/password boundaries are enforced | AI `TestLocalAccountHTTPFieldContract`; `internal/domain/account_test.go` |
| 005 | Wrong password, unknown and disabled usernames have identical safe login failure | AI `TestLocalAccountFailuresAndCredentialBoundaries`; MI `TestAccountMaintenanceDisableAndEnableDoNotReviveSessions` |
| 006 | Login/registration return 429/Retry-After; forged forwarding cannot bypass limits | AI `TestLocalAccountRegistrationRateLimit`, `TestLocalAccountRateLimitsAcrossIPsAndWindows`, `TestLocalAccountSuccessClearsFailureCountAndIPLimit` |
| 007 | Login returns only to allowed local targets | `features/accounts/WorkspaceAccess.test.tsx` (safe-return cases) |
| 008 | Missing CSRF/wrong Origin reject writes; valid scripts work | AI `TestLocalAccountCSRFAndServiceFailure`; BI `TestBusinessAPIsRequireSessionAndCSRF`; DI restart test executes `scripts/account-session.py` |
| 009 | Unauthenticated business reads/writes are 401, health stays public | BI `TestBusinessAPIsRequireSessionAndCSRF` covers every business route; B |
| 010 | Independent sessions coexist; reload/reopen restores valid Cookie identity | AI `TestLocalAccountConcurrentSessionsActivityAndExpiry`; B reload and full Chrome close/relaunch steps |
| 011 | Background reads never renew; activity respects 30-minute/8-hour bounds | AI `TestLocalAccountConcurrentSessionsActivityAndExpiry`; `features/accounts/AccountPage.test.tsx` foreground activity |
| 012 | Current/browser-tab logout revokes only that session and destroys drafts | AI `TestLocalAccountCurrentAndAllSessionRevocation`; `features/accounts/WorkspaceAccess.test.tsx`, `sessionCoordination.test.ts`; B logout |
| 013 | Logout-all/change/reset revoke all old sessions, including same-password reset | AI `TestLocalAccountPasswordChangeRevokesAllSessions`, `TestLocalAccountCurrentAndAllSessionRevocation`; MI `TestAccountMaintenanceResetPasswordRevokesEverySession` |
| 014 | Security changes win over an already verified login snapshot | `account_security_race_integration_test.go`: `TestLocalAccountSecurityChangesWinAgainstVerifiedLogin` (seven mutations and successful same-barrier control) |
| 015 | Already authenticated writes finish under original account; new requests fail | BI `TestRevocationRejectsNewRequestsButAllowsAuthenticatedWriteToFinish` |
| 016 | Expiry masks workspace; same-account recovery rechecks target and requires confirmation | `WorkspaceAccess.test.tsx`, `ManagedDataMutationPage.test.tsx` and Query/Mutation/TablePoliciesPage tests |
| 017 | Logout/switch/refresh clear drafts; old responses cannot enter new identity | `WorkspaceAccess.test.tsx`, `AccountPage.test.tsx`, `sessionCoordination.test.ts`, `api/client.test.ts`; B refresh/reopen |
| 018 | Own profile/email changes retain stable identity and require current password for email | AI `TestLocalAccountProfileChangesAffectOnlyCurrentAccount`; Web `AccountPage.test.tsx` |
| 019 | No ordinary account-list/admin-other-account API | AI `TestLocalAccountPasswordChangeRevokesAllSessions`, `TestLocalAccountMaintenanceAuthorizesBeforeInputs`; Web own-account forms |
| 020 | CLI disable/enable/email changes preserve ownership and do not revive revoked sessions | MI `TestAccountMaintenanceDisableAndEnableDoNotReviveSessions`, `TestAccountMaintenanceCorrectEmailPreservesOwnershipAndSessions`, `TestAccountMaintenanceResetDisabledAccountPreservesDraftDestruction` |
| 021 | Two accounts' row and all three catalog writes get corresponding permanent Account IDs | BI `TestConcurrentAccountsOwnTheirBusinessChanges`; B independently checks MySQL creator/modifier |
| 022 | Historical Operator values stay literal; incompatible columns reject without mutation | BI `TestOperatorColumnsRejectIncompatibleWritesAndPreserveHistory` |
| 023 | Policy rejection stays 403 without login redirect; schema/Auto Fill still apply | `WorkspaceAccess.test.tsx`; I authenticated policy/query/mutation regressions in `mutation_integration_test.go` and `query_integration_test.go` |
| 024 | Graceful process restart preserves valid sessions, session CSRF and unfinished rate windows | DI `TestAccountProcessRestartPreservesSessionsAndRateWindows`; `TestAccountProcessCapacityNeverEvictsValidState` |
| 025 | Missing authentication structure fails startup; empty schema works; faults stay 503/504 | DI `TestAccountProcessRequiresCompleteAuthenticationSchema` (table/column/unique/collation/index/FK/lock row and additional session/expiry uniqueness, including invisible indexes), `TestAccountProcessDatabaseFailuresRemain503And504`; `process_test.go` unavailable DB process test |
| 026 | Lost registration/business/password responses do not trigger automatic replay | BI `TestCommittedWritesRemainSingleWhenHTTPResponsesAreLost`; Web account/data and three policy-page suites, including reopening drawers and failed 503/504 read checks preserving uncertainty |
| 027 | Actual legacy migration reaches fresh constraints; maintenance bypasses normal readiness | DI `TestAccountUpgradeFromLegacyMatchesFreshSchema`; MI `TestAccountMaintenanceIndependentConnectionAndAtomicFailures`; `TestAdminProcessRejectsRemovedAuthenticationConfiguration`; migration/HTTPS runbook |
| 028 | HTTPS/local Cookie attributes and sensitive-data protection | DI `TestAccountProcessHTTPSCookiesAndSensitiveMaterials` through actual TLS proxy; B local Cookie/storage/URL checks and Admin/Vite log checks; AI `TestLocalAccountStoredSecretsAndAbsoluteExpiry`; CLI secret tests |
| 029 | Browser → same-origin Vite → real Admin → MySQL registration/write/Operator/logout | `account_browser_integration_test.go`: `TestAccountBrowserSystemPath` and `web/e2e/accounts.mjs`; MySQL checks persisted row creator and modifier against current Account ID |
| 030 | Authentication control tables cannot be discovered, assigned or mutated generically | BI `TestAccountControlTablesCannotBeDiscoveredOrManaged` |
| 031 | Forged/tampered/expired/revoked/preauth credentials cannot read business; login rotates credentials | DI `TestAccountProcessRestartPreservesSessionsAndRateWindows`; AI `TestLocalAccountFailuresAndCredentialBoundaries`; B post-logout business 401 |

## Recorded delivery verification

The final implementation passed independent Standards and Spec review against
base `c98d5bdf1db42ce8d06da20d6e16d0d59ec659e0`. The 22-file implementation freeze manifest
has SHA-256 `ca181215a705644db1fb8823b83aeeaaf6307e426781545c5e13dc918e3a9c59`.
Only this evidence document was updated afterward to record completed results.

On 2026-09-07, all four Go modules tested and built successfully; the Web suite
passed all **158 tests in 16 files**, followed by typecheck and production build.
The complete real MySQL suite passed **330 test events, 0 failures and
0 test-level skips**, using `-json -count=1 -timeout=20m -tags=integration` and
the explicit Colima environment above. Its final run finished at
`2026-09-07T06:13:49.130005+00:00`. The real browser system target passed registration, full Chrome
close/relaunch, a MySQL configuration write attributed to the current Account ID,
Cookie/storage/URL protection and logout rejection.

| Final check | Result | Durable raw log and command/exit metadata |
| --- | --- | --- |
| go-test | exit 0 | `final-go-test.log`, `final-go-test.json` |
| go-build | exit 0 | `final-go-build.log`, `final-go-build.json` |
| web-test | exit 0 | `final-web-test.log`, `final-web-test.json` |
| web-typecheck | exit 0 | `final-web-typecheck.log`, `final-web-typecheck.json` |
| web-build | exit 0 | `final-web-build.log`, `final-web-build.json` |
| integration | exit 0 | `final-integration.log`, `final-integration.json` |
| browser | exit 0 | `final-browser.log`, `final-browser.json` |

The local delivery archive is
`.worktrees/account-delivery-records/issue-40/` under the original repository.
It retains every raw log, command, environment, timestamp and exit code listed
above, the runner `final-check.py`, and the reviewed source manifests. These local
artifacts are supplementary; the 31-case map and reproduction commands above are
part of the committed repository. Source was restored from persistent session history after the earlier OS temporary
directory was removed. Final checks were rerun to generate fresh logs in this
persistent location. The
interrupted temporary run was never counted as a completed acceptance run.

Additional checks in that archive:

- `nginx-validation-final.json/log`: the delivered HTTPS proxy configuration passed
  actual `nginx -t` in a disposable Docker build; test image/certificate resources
  were removed. No deployment was performed.
- `browser-no-docker.json/log`: an absent Docker socket made `make test-browser`
  fail with exit 2 and an explicit required-provider error, with no SKIP.
- All ten incomplete-schema process cases (including extra visible/invisible
  UNIQUE constraints), restart/session/rate persistence, actual legacy-to-fresh
  metadata comparison, maintenance independence, 503/504 faults, HTTPS Cookies
  and capacity cleanup passed in the final MySQL suite.

Initial final-check failures were diagnosed and fixed, rather than hidden by
retries. The SIGINT/SIGTERM harness incorrectly allowed 5 seconds for a configured
10-second shutdown. A real unfinished HTTP header reproduced both failures; the
harness now allows the production deadline plus 2 seconds of bounded process-exit
overhead, retaining exit/close assertions and the separate forced-shutdown bound.
Web test preparation now waits for visible Change Set focus before interruption
and flushes identity/activity effects before native events. Controlled focus
tracing showed jsdom redirecting username input into a hidden dialog; a real
Chrome control rejected that hidden focus. Two mounted test pages now generate
exactly two competing activity reports through one shared-document event.
Production authentication behavior and Web test timeouts were unchanged. The corrected
36-test Web pair passed six consecutive full-file runs before the final 158-test
suite. Raw failures, deterministic red/green probes and the refined explanation
are retained in `timeout-diagnosis.md` and its referenced logs; filtered diagnostic
runs and `superseded-integration.*` are explicitly excluded from final acceptance.

## Operational and architectural checks

The [account runbook](./admin-local-accounts.md) includes the stop-old-ingress order,
explicit SQL migration, Nginx HTTPS proxy and trusted client-IP settings, maintenance
CLI and executable memory-only Cookie/CSRF/activity script. Normal startup fails
safely without upgrading schemas; maintenance opens its own database connection.
Account domain vocabulary and ADR-0017/0018 remain accurate; no new domain concept
or dependency direction is introduced. This repository has no independent import
checker/generated OpenAPI parity check; Go internal boundaries, `TestHTTPDependsOnApplicationRatherThanDomainOrInfrastructure`,
`TestSharedBusinessServicesDoNotStoreRequestIdentity`, HTTP/MySQL contract tests,
Zod response validation and Web tests are the current automated protection.

TMP-01 is deleted: normal legacy Token/disabled-auth/fixed Operator settings and
Vite Token injection are rejected; `TestAdminProcessRejectsRemovedAuthenticationConfiguration`
and the complete business-route test prove the absence of a bypass. Historical
policy migration's explicit Operator is confined to maintenance. No deployment or
main-branch merge is performed by this acceptance work. Parent #34 and request #29
remain open; project-level completion is reported only after final integration and
Notion PM-009 synchronization.
